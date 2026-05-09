// Package auth extracts the authenticated user identity continuum's plugin
// host injects into HTTP requests forwarded to the plugin. The convention
// (subject to confirmation against continuum's pluginhost) is HTTP request
// headers X-Continuum-User-Id and X-Continuum-User-Role.
package auth

import "net/http"

const (
	HeaderUserID = "X-Continuum-User-Id"
	HeaderRole   = "X-Continuum-User-Role"
)

type Identity struct {
	UserID  string
	IsAdmin bool
}

// FromRequest returns the Identity stamped on the request by continuum's
// plugin host, plus an ok flag (false if no user_id header was present).
func FromRequest(r *http.Request) (Identity, bool) {
	uid := r.Header.Get(HeaderUserID)
	if uid == "" {
		return Identity{}, false
	}
	return Identity{
		UserID:  uid,
		IsAdmin: r.Header.Get(HeaderRole) == "admin",
	}, true
}
