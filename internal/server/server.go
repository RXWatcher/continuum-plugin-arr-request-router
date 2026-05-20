package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/consumer"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/poll"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/store"
)

// Deps is the wiring this package needs from main.go.
type Deps struct {
	Store     *store.Store
	Enricher  routing.Enricher
	Events    *event.Publisher
	Poll      *poll.Poller            // exposed for future "poll now" admin button
	Submit    *consumer.SubmitHandler // for retry / re-route in Task 9.5
	OnConfig  func(context.Context, store.AppConfig) error
	SecretKey string
	WebFS     http.FileSystem // dist/ embedded SPA — populated by Task 11.3
}

// Server holds the wired dependencies and exposes an http.Handler.
type Server struct{ deps *Deps }

// New constructs a Server from the provided Deps.
func New(d *Deps) *Server { return &Server{deps: d} }

// Handler returns the http.Handler the plugin SDK serves under the
// http_routes.v1 capability. Routes:
//
//	/api/admin/registry/*   — registry CRUD + test-connection
//	/api/admin/route-test   — Task 9.4
//	/api/admin/requests/*   — Task 9.5
//	/admin/*                — SPA (Task 10.2 fills in)
//	/assets/*               — static assets
//
// All /api/admin/* and /admin/* require admin role via the requireAdmin
// middleware; /assets/* is public to allow the browser to fetch JS/CSS.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Route("/api/admin", func(r chi.Router) {
		r.Use(requireAdmin)
		r.Get("/config", s.handleGetConfig)
		r.Put("/config", s.handlePutConfig)
		r.Route("/registry", s.registryRoutes)
		r.Post("/route-test", s.handleRouteTest)
		r.Route("/requests", s.requestsRoutes)
		r.Get("/targets/health", s.handleTargetsHealth)
	})

	r.Get("/admin", s.handleSPA)
	r.Get("/admin/*", s.handleSPA)
	if s.deps.WebFS != nil {
		r.Get("/assets/*", http.FileServer(s.deps.WebFS).ServeHTTP)
	}
	return r
}

// requireAdmin is a chi middleware that enforces the admin role. Requests
// without the X-Continuum-User-Role: admin header are rejected with 403.
// Requests without any identity headers at all are also rejected with 403
// (plugin host always stamps the headers; absence means unauthenticated).
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromRequest(r)
		if !ok || !id.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.deps.Store.GetAppConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var cfg store.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	cfg = store.NormalizeAppConfig(cfg)
	if err := s.deps.Store.UpsertAppConfig(r.Context(), cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.deps.OnConfig != nil {
		if err := s.deps.OnConfig(r.Context(), cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, cfg)
}
