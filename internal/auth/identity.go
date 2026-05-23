// Package auth extracts the authenticated user identity silo's plugin
// host injects into HTTP requests forwarded to the plugin. The convention
// (subject to confirmation against silo's pluginhost) is HTTP request
// headers X-Silo-User-Id and X-Silo-User-Role.
package auth

import "net/http"

const (
	HeaderUserID = "X-Silo-User-Id"
	HeaderRole   = "X-Silo-User-Role"
)

type Identity struct {
	UserID  string
	IsAdmin bool
}

// FromRequest returns the Identity stamped on the request by silo's
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
