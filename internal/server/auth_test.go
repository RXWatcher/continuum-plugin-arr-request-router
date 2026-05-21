package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/auth"
)

// TestRequireAdminMissingRoleHeader verifies that a request with no
// X-Continuum-User-Role header is rejected with 403. The middleware treats
// missing identity headers as unauthenticated and falls through to the same
// forbidden response as a non-admin role — the plugin host always stamps the
// headers, so absence implies the request bypassed the host.
func TestRequireAdminMissingRoleHeader(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "/", nil)
	// Neither HeaderUserID nor HeaderRole set.
	w := httptest.NewRecorder()
	requireAdmin(next).ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("downstream handler was called for unauthenticated request")
	}
}

// TestRequireAdminNonAdminRole verifies that a request with a valid user_id
// but a non-admin role is rejected with 403 and never reaches the downstream
// handler.
func TestRequireAdminNonAdminRole(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "user")
	w := httptest.NewRecorder()
	requireAdmin(next).ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("downstream handler was called for non-admin role")
	}
}

// TestRequireAdminAdminRoleReachable verifies that a request with the admin
// role passes through to the downstream handler.
func TestRequireAdminAdminRoleReachable(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(auth.HeaderUserID, "user-1")
	r.Header.Set(auth.HeaderRole, "admin")
	w := httptest.NewRecorder()
	requireAdmin(next).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("downstream handler was not called for admin role")
	}
}
