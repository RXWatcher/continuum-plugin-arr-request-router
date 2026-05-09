package arr_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/arr"
)

func TestSystemStatusOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/system/status" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "k" {
			t.Fatalf("missing api key")
		}
		w.Write([]byte(`{"version":"5.4.0","instanceName":"Radarr","branch":"main"}`))
	}))
	defer srv.Close()
	got, err := arr.SystemStatus(context.Background(), srv.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "5.4.0" || got.InstanceName != "Radarr" {
		t.Fatalf("got %+v", got)
	}
}

func TestSystemStatusUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := arr.SystemStatus(context.Background(), srv.URL, "k"); err == nil {
		t.Fatal("expected error from 401")
	}
}

func TestSystemStatusTrimsTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/system/status" {
			t.Fatalf("path: %s (caller appended slash before /api/...)", r.URL.Path)
		}
		w.Write([]byte(`{"version":"5.0.0"}`))
	}))
	defer srv.Close()
	if _, err := arr.SystemStatus(context.Background(), srv.URL+"/", "k"); err != nil {
		t.Fatal(err)
	}
}
