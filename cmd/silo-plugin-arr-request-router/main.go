package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/jackc/pgx/v5/pgxpool"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtime"

	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/arr"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/consumer"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/event"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/httproutes"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/migrate"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/poll"
	pluginrt "github.com/RXWatcher/silo-plugin-arr-request-router/internal/runtime"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/server"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/store"
	"github.com/RXWatcher/silo-plugin-arr-request-router/internal/tmdb"
	"github.com/RXWatcher/silo-plugin-arr-request-router/web"
)

//go:embed manifest.json
var manifestRaw []byte

func main() {
	logger := hclog.New(&hclog.LoggerOptions{Name: "silo-plugin-arr-request-router"})

	manifest, err := loadManifest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}

	httpSrv := httproutes.NewServer()

	var (
		poolPtr   atomic.Pointer[pgxpool.Pool]
		storePtr  atomic.Pointer[store.Store]
		eventsPtr atomic.Pointer[event.Publisher]
		submitPtr atomic.Pointer[consumer.SubmitHandler]
		cancelPtr atomic.Pointer[consumer.CancelHandler]
	)

	// pollLoopMu guards the cancel func for the background poll goroutine.
	var pollLoopMu sync.Mutex
	var pollLoopCancel context.CancelFunc

	// radarrFactory and sonarrFactory create per-arr clients. They are
	// deterministic (no shared state) so they do not need atomic pointers.
	radarrFactory := func(u, key string) *arr.Radarr { return arr.NewRadarr(u, key) }
	sonarrFactory := func(u, key string) *arr.Sonarr { return arr.NewSonarr(u, key) }

	// pollCfgSnapshot holds per-Configure values for the poller closure.
	type pollCfgSnapshot struct {
		staleAfterHours int
		secretKey       string
	}
	var pollCfgPtr atomic.Pointer[pollCfgSnapshot]

	// pollerDeps is the lazy-deps closure for the poller. Returns nil until the
	// first Configure call populates the pointers.
	pollerDeps := func() *poll.Deps {
		s := storePtr.Load()
		ev := eventsPtr.Load()
		c := pollCfgPtr.Load()
		if s == nil || c == nil {
			return nil
		}
		return &poll.Deps{
			Store:           s,
			Radarr:          radarrFactory,
			Sonarr:          sonarrFactory,
			Events:          ev,
			StaleAfterHours: c.staleAfterHours,
			SecretKey:       c.secretKey,
		}
	}

	poller := poll.New(pollerDeps, logger.Named("poll"))

	// startPollLoop (re)starts the background poll goroutine at the given interval.
	// rootCtx is cancelled on SIGTERM/SIGINT. Background loops (poll ticker)
	// derive from it so a graceful shutdown stops in-flight database queries
	// instead of letting the host's drain timeout kill them mid-statement.
	// signal.NotifyContext fanning to multiple subscribers is documented and
	// safe; the SDK runtime's own signal handler continues running.
	rootCtx, rootCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer rootCancel()

	startPollLoop := func(interval time.Duration) {
		pollLoopMu.Lock()
		defer pollLoopMu.Unlock()
		if pollLoopCancel != nil {
			pollLoopCancel()
		}
		if interval <= 0 {
			pollLoopCancel = nil
			return
		}
		ctx, cancel := context.WithCancel(rootCtx)
		pollLoopCancel = cancel
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if err := poller.Run(ctx); err != nil && ctx.Err() == nil {
						logger.Warn("poll run error", "err", err)
					}
				}
			}
		}()
	}

	rt := pluginrt.New(manifest, func(cfg pluginrt.Config) error {
		ctx := context.Background()

		// Explicit MaxConns cap. The pgx default scales with GOMAXPROCS and
		// can be as low as 4; the poll loop + admin SPA + consumer mix can
		// starve under that. 16 is generous without saturating a shared
		// Postgres. Operators override via DSN (?pool_max_conns=N).
		pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("parse db: %w", err)
		}
		if pcfg.MaxConns < 16 {
			pcfg.MaxConns = 16
		}
		p, err := pgxpool.NewWithConfig(ctx, pcfg)
		if err != nil {
			return fmt.Errorf("pgxpool: %w", err)
		}
		if err := migrate.Run(ctx, cfg.DatabaseURL); err != nil {
			p.Close()
			return fmt.Errorf("migrate: %w", err)
		}

		s := store.New(p)
		ev := event.New(sdkruntime.Host(), logger.Named("event"))
		storePtr.Store(s)
		eventsPtr.Store(ev)

		if _, err := s.ImportLegacyAppConfig(ctx, appConfigFromRuntime(cfg)); err != nil {
			p.Close()
			return fmt.Errorf("import legacy app config: %w", err)
		}
		appCfg, err := s.GetAppConfig(ctx)
		if err != nil {
			p.Close()
			return fmt.Errorf("get app config: %w", err)
		}

		deps := &server.Deps{
			Store:  s,
			Events: ev,
			Poll:   poller,
			Radarr: radarrFactory,
			Sonarr: sonarrFactory,
			WebFS:  web.FS(),
		}

		applyAppConfig := func(_ context.Context, appCfg store.AppConfig) error {
			appCfg = store.NormalizeAppConfig(appCfg)
			tmdbClient := tmdb.New("https://api.themoviedb.org/3", appCfg.TMDBAPIKey, appCfg.TMDBLanguage)
			enricher := tmdb.NewCache(tmdbClient, 24*time.Hour)

			submitH := &consumer.SubmitHandler{
				Store:     s,
				Enricher:  enricher,
				Radarr:    radarrFactory,
				Sonarr:    sonarrFactory,
				Events:    ev,
				SecretKey: appCfg.SecretKey,
				Log:       logger.Named("submit"),
			}
			cancelH := &consumer.CancelHandler{
				Store:     s,
				Radarr:    radarrFactory,
				Sonarr:    sonarrFactory,
				Events:    ev,
				SecretKey: appCfg.SecretKey,
				Log:       logger.Named("cancel"),
			}

			deps.Enricher = enricher
			deps.Submit = submitH
			deps.SecretKey = appCfg.SecretKey

			submitPtr.Store(submitH)
			cancelPtr.Store(cancelH)

			snap := &pollCfgSnapshot{
				staleAfterHours: appCfg.StaleAfterHours,
				secretKey:       appCfg.SecretKey,
			}
			pollCfgPtr.Store(snap)
			startPollLoop(time.Duration(appCfg.PollIntervalSeconds) * time.Second)
			return nil
		}

		deps.OnConfig = applyAppConfig
		if err := applyAppConfig(ctx, appCfg); err != nil {
			p.Close()
			return err
		}

		mux := server.New(deps)
		httpSrv.SetHandler(mux.Handler())

		if old := poolPtr.Swap(p); old != nil {
			old.Close()
		}
		return nil
	})

	// lazySubmitter and lazyCanceller are thin wrappers that check the
	// atomic pointers at call time, returning a no-op when not yet configured.
	lazySubmit := &lazySubmitter{ptr: &submitPtr}
	lazyCancel := &lazyCanceller{ptr: &cancelPtr}

	dispatcher := consumer.New(lazySubmit, lazyCancel, logger.Named("consumer"))
	eventSrv := consumer.NewEventServer(dispatcher)

	scheduled := &poll.ScheduledServer{Poller: poller}

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Logger: logger,
		Servers: sdkruntime.CapabilityServers{
			Runtime:       rt,
			HttpRoutes:    httpSrv,
			EventConsumer: eventSrv,
			ScheduledTask: scheduled,
		},
	})
}

func appConfigFromRuntime(cfg pluginrt.Config) store.AppConfig {
	return store.AppConfig{
		TMDBAPIKey:          cfg.TMDBAPIKey,
		TMDBLanguage:        cfg.TMDBLanguage,
		PollIntervalSeconds: cfg.PollIntervalSeconds,
		StaleAfterHours:     cfg.StaleAfterHours,
		SecretKey:           cfg.SecretKey,
	}
}

// lazySubmitter implements consumer.Submitter by checking the atomic pointer
// at call time. Returns nil (no-op) if the pointer is not yet populated.
type lazySubmitter struct {
	ptr *atomic.Pointer[consumer.SubmitHandler]
}

func (l *lazySubmitter) HandleSubmitted(ctx context.Context, payload map[string]any) error {
	h := l.ptr.Load()
	if h == nil {
		return nil
	}
	return h.HandleSubmitted(ctx, payload)
}

// lazyCanceller implements consumer.Canceller by checking the atomic pointer
// at call time. Returns nil (no-op) if the pointer is not yet populated.
type lazyCanceller struct {
	ptr *atomic.Pointer[consumer.CancelHandler]
}

func (l *lazyCanceller) HandleCancelled(ctx context.Context, payload map[string]any) error {
	h := l.ptr.Load()
	if h == nil {
		return nil
	}
	return h.HandleCancelled(ctx, payload)
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestRaw)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}
	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])
	if len(manifest.GetSupportedPlatforms()) == 0 {
		manifest.SupportedPlatforms = []*pluginv1.SupportedPlatform{
			{Os: goruntime.GOOS, Arch: goruntime.GOARCH},
		}
	}
	return manifest, nil
}
