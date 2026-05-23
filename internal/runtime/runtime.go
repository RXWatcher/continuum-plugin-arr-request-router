// Package runtime implements the plugin's Runtime gRPC server. Its Configure
// handler parses the global config payload into Config and invokes a callback
// supplied by main.go so the plugin can (re)wire its pool/store/clients.
package runtime

import (
	"context"
	"fmt"
	"sync"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginsdk/runtimedefault"
)

const (
	defaultPollIntervalSeconds = 30
	minPollIntervalSeconds     = 10
	maxPollIntervalSeconds     = 600
	defaultStaleAfterHours     = 72
	defaultTMDBLanguage        = "en-US"
	minSecretKeyLength         = 16
)

// Config is the parsed plugin global config.
type Config struct {
	DatabaseURL         string
	TMDBAPIKey          string
	TMDBLanguage        string
	PollIntervalSeconds int
	StaleAfterHours     int
	SecretKey           string
}

// Server implements the plugin's Runtime service.
type Server struct {
	runtimedefault.Server
	manifest *pluginv1.PluginManifest
	onCfg    func(Config) error

	mu  sync.RWMutex
	cfg Config
}

func New(manifest *pluginv1.PluginManifest, onConfig func(Config) error) *Server {
	return &Server{manifest: manifest, onCfg: onConfig}
}

func (s *Server) GetManifest(_ context.Context, _ *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *Server) Configure(_ context.Context, req *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	cfg, err := loadConfig(req.GetConfig())
	if err != nil {
		return nil, err
	}

	if s.onCfg != nil {
		if err := s.onCfg(cfg); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return &pluginv1.ConfigureResponse{}, nil
}

func (s *Server) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// loadConfig parses a slice of ConfigEntry protos into a Config. It is
// exported as a standalone function so tests can exercise it without a gRPC
// server.
func loadConfig(entries []*pluginv1.ConfigEntry) (Config, error) {
	cfg := Config{
		TMDBLanguage:        defaultTMDBLanguage,
		PollIntervalSeconds: defaultPollIntervalSeconds,
		StaleAfterHours:     defaultStaleAfterHours,
	}

	for _, e := range entries {
		v := e.GetValue()
		if v == nil {
			continue
		}
		m := v.AsMap()
		switch e.GetKey() {
		case "database_url":
			cfg.DatabaseURL = stringFromValue(m["value"], firstString(m))
		case "tmdb.api_key":
			cfg.TMDBAPIKey = stringFromValue(m["value"], firstString(m))
		case "tmdb.language":
			if s := stringFromValue(m["value"], firstString(m)); s != "" {
				cfg.TMDBLanguage = s
			}
		case "poll_interval_seconds":
			if n, ok := intFromValue(m["value"]); ok && n > 0 {
				cfg.PollIntervalSeconds = n
			}
		case "stale_after_hours":
			if n, ok := intFromValue(m["value"]); ok && n > 0 {
				cfg.StaleAfterHours = n
			}
		case "secret_key":
			cfg.SecretKey = stringFromValue(m["value"], firstString(m))
		}
	}

	// Required field validation.
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("database_url is required")
	}
	if cfg.SecretKey != "" && len(cfg.SecretKey) < minSecretKeyLength {
		return Config{}, fmt.Errorf("secret_key must be at least %d characters", minSecretKeyLength)
	}

	// Clamp poll interval to allowed range.
	if cfg.PollIntervalSeconds < minPollIntervalSeconds {
		cfg.PollIntervalSeconds = minPollIntervalSeconds
	}
	if cfg.PollIntervalSeconds > maxPollIntervalSeconds {
		cfg.PollIntervalSeconds = maxPollIntervalSeconds
	}

	return cfg, nil
}

func stringFromValue(candidates ...any) string {
	for _, c := range candidates {
		if s, ok := c.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func firstString(m map[string]any) any {
	for _, v := range m {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return nil
}

// intFromValue extracts an int from proto numeric types (float64 from AsMap,
// or direct int/int64/float64 variants).
func intFromValue(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case int32:
		return int(n), true
	}
	return 0, false
}
