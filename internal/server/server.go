package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/consumer"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/poll"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

// Deps is the wiring this package needs from main.go.
type Deps struct {
	Store     *store.Store
	Enricher  routing.Enricher
	Events    *event.Publisher
	Poll      *poll.Poller            // exposed for future "poll now" admin button
	Submit    *consumer.SubmitHandler // for retry / re-route in Task 9.5
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
		r.Route("/registry", s.registryRoutes)
		r.Post("/route-test", s.handleRouteTest)
		r.Route("/requests", s.requestsRoutes)
	})

	// Task 10.2 will replace this with a theme-injecting prerender.
	r.Get("/admin/*", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "spa not yet wired", http.StatusNotImplemented)
	})
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
