package arr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RXWatcher/continuum-plugin-arr-request-router/internal/arr"
)

func TestRadarrAddMoviePostsTMDBID(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/movie" {
			t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id":     42,
			"tmdbId": 603,
			"title":  "X",
			"year":   1999,
		})
	}))
	defer srv.Close()

	c := arr.NewRadarr(srv.URL, "testkey")
	movie, err := c.AddMovie(context.Background(), arr.AddMovieRequest{
		Title:            "X",
		TMDBID:           603,
		Year:             1999,
		QualityProfileID: 7,
		RootFolderPath:   "/movies",
		Monitored:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if movie.ID != 42 {
		t.Fatalf("movie.ID = %d, want 42", movie.ID)
	}
	if v, _ := got["tmdbId"].(float64); int(v) != 603 {
		t.Fatalf("tmdbId = %v, want 603", got["tmdbId"])
	}
	if got["title"] != "X" {
		t.Fatalf("title = %v, want X", got["title"])
	}
}

func TestIsConflictDetects409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`[{"errorMessage":"already exists"}]`))
	}))
	defer srv.Close()

	c := arr.NewRadarr(srv.URL, "testkey")
	_, err := c.AddMovie(context.Background(), arr.AddMovieRequest{
		Title:            "Duplicate",
		TMDBID:           100,
		QualityProfileID: 1,
		RootFolderPath:   "/movies",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !arr.IsConflict(err) {
		t.Fatalf("IsConflict = false, want true; err = %v", err)
	}
}
