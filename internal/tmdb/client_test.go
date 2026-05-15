package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestClient(srv *httptest.Server) *Client {
	c := New(srv.URL, "test-api-key", "en-US")
	c.http = srv.Client()
	return c
}

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------------------
// Primary — movie
// ---------------------------------------------------------------------------

func TestPrimaryMovie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/550" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{
			"original_language": "en",
			"original_title":    "Fight Club",
			"genres":            []map[string]any{{"name": "Drama"}, {"name": "Thriller"}},
			"runtime":           139,
			"vote_average":      8.4,
			"vote_count":        25000,
			"popularity":        61.5,
			"adult":             false,
			"status":            "Released",
			"production_companies": []map[string]any{
				{"name": "Fox 2000 Pictures"}, {"name": "Regency Enterprises"},
			},
			"production_countries": []map[string]any{{"iso_3166_1": "US"}, {"iso_3166_1": "DE"}},
			"spoken_languages":     []map[string]any{{"iso_639_1": "en"}},
			"release_date":         "1999-10-15",
			"budget":               63000000,
			"revenue":              100853753,
			"belongs_to_collection": map[string]any{"name": "Fight Club Collection"},
			"imdb_id": "tt0137523",
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.Primary(context.Background(), "movie", 550)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertStr(t, "MediaType", got.MediaType, "movie")
	assertStr(t, "OriginalLanguage", got.OriginalLanguage, "en")
	assertStr(t, "OriginalTitle", got.OriginalTitle, "Fight Club")
	assertStrSlice(t, "Genres", got.Genres, []string{"Drama", "Thriller"})
	assertInt(t, "Runtime", got.Runtime, 139)
	assertFloat(t, "VoteAverage", got.VoteAverage, 8.4)
	assertInt(t, "VoteCount", got.VoteCount, 25000)
	assertFloat(t, "Popularity", got.Popularity, 61.5)
	if got.Adult {
		t.Error("Adult: want false")
	}
	assertStr(t, "Status", got.Status, "Released")
	assertStrSlice(t, "ProductionCompanies", got.ProductionCompanies, []string{"Fox 2000 Pictures", "Regency Enterprises"})
	assertStrSlice(t, "ProductionCountries", got.ProductionCountries, []string{"US", "DE"})
	assertStrSlice(t, "SpokenLanguages", got.SpokenLanguages, []string{"en"})
	assertStr(t, "ReleaseDate", got.ReleaseDate, "1999-10-15")
	assertInt(t, "Budget", got.Budget, 63000000)
	assertInt(t, "Revenue", got.Revenue, 100853753)
	assertStr(t, "BelongsToCollection", got.BelongsToCollection, "Fight Club Collection")
	assertStr(t, "IMDBID", got.IMDBID, "tt0137523")
}

// ---------------------------------------------------------------------------
// Primary — tv
// ---------------------------------------------------------------------------

func TestPrimaryTV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/1396" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{
			"original_language":    "en",
			"original_name":        "Breaking Bad",
			"genres":               []map[string]any{{"name": "Crime"}, {"name": "Drama"}},
			"episode_run_time":     []int{47},
			"vote_average":         9.5,
			"vote_count":           12000,
			"popularity":           200.3,
			"adult":                false,
			"status":               "Ended",
			"production_companies": []map[string]any{{"name": "Gran Via Productions"}},
			"production_countries": []map[string]any{{"iso_3166_1": "US"}},
			"spoken_languages":     []map[string]any{{"iso_639_1": "en"}},
			"networks":             []map[string]any{{"name": "AMC"}},
			"origin_country":       []string{"US"},
			"first_air_date":       "2008-01-20",
			"last_air_date":        "2013-09-29",
			"type":                 "Scripted",
			"in_production":        false,
			"number_of_seasons":    5,
			"number_of_episodes":   62,
			"created_by":           []map[string]any{{"name": "Vince Gilligan"}},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.Primary(context.Background(), "tv", 1396)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertStr(t, "MediaType", got.MediaType, "tv")
	assertStr(t, "OriginalLanguage", got.OriginalLanguage, "en")
	assertStr(t, "OriginalTitle", got.OriginalTitle, "Breaking Bad") // original_name → OriginalTitle
	assertStrSlice(t, "Genres", got.Genres, []string{"Crime", "Drama"})
	assertInt(t, "Runtime", got.Runtime, 47) // episode_run_time[0]
	assertFloat(t, "VoteAverage", got.VoteAverage, 9.5)
	assertInt(t, "VoteCount", got.VoteCount, 12000)
	assertFloat(t, "Popularity", got.Popularity, 200.3)
	assertStr(t, "Status", got.Status, "Ended")
	assertStrSlice(t, "ProductionCompanies", got.ProductionCompanies, []string{"Gran Via Productions"})
	assertStrSlice(t, "ProductionCountries", got.ProductionCountries, []string{"US"})
	assertStrSlice(t, "SpokenLanguages", got.SpokenLanguages, []string{"en"})
	assertStrSlice(t, "Networks", got.Networks, []string{"AMC"})
	assertStrSlice(t, "OriginCountry", got.OriginCountry, []string{"US"})
	assertStr(t, "FirstAirDate", got.FirstAirDate, "2008-01-20")
	assertStr(t, "LastAirDate", got.LastAirDate, "2013-09-29")
	assertStr(t, "Type", got.Type, "Scripted")
	if got.InProduction {
		t.Error("InProduction: want false")
	}
	assertInt(t, "NumberOfSeasons", got.NumberOfSeasons, 5)
	assertInt(t, "NumberOfEpisodes", got.NumberOfEpisodes, 62)
	assertStrSlice(t, "CreatedBy", got.CreatedBy, []string{"Vince Gilligan"})
}

// ---------------------------------------------------------------------------
// Primary — edge cases
// ---------------------------------------------------------------------------

func TestPrimaryEpisodeRunTimeEmptyArrayYieldsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"original_language": "ja",
			"original_name":     "Some Anime",
			"episode_run_time":  []int{},
			"genres":            []map[string]any{},
			"production_companies": []map[string]any{},
			"production_countries": []map[string]any{},
			"spoken_languages":     []map[string]any{},
			"networks":             []map[string]any{},
			"origin_country":       []string{},
			"created_by":           []map[string]any{},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.Primary(context.Background(), "tv", 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInt(t, "Runtime", got.Runtime, 0)
}

func TestPrimaryBelongsToCollectionNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"original_language":    "en",
			"original_title":       "Solo Film",
			"genres":               []map[string]any{},
			"production_companies": []map[string]any{},
			"production_countries": []map[string]any{},
			"spoken_languages":     []map[string]any{},
			"belongs_to_collection": nil,
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.Primary(context.Background(), "movie", 111)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertStr(t, "BelongsToCollection", got.BelongsToCollection, "")
}

func TestPrimary404ReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status_message":"The resource you requested could not be found."}`, 404)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Primary(context.Background(), "movie", 999)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestPrimary500ReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status_message":"Internal server error"}`, 500)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Primary(context.Background(), "tv", 123)
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestPrimaryUnknownMediaTypeReturnsErr(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, 200, map[string]any{})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Primary(context.Background(), "podcast", 1)
	if err == nil {
		t.Fatal("expected error for unknown mediaType, got nil")
	}
	if called {
		t.Error("no HTTP call expected for unknown mediaType")
	}
}

func TestPrimaryAttachesAPIKeyAndLanguage(t *testing.T) {
	var gotAPIKey, gotLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.URL.Query().Get("api_key")
		gotLang = r.URL.Query().Get("language")
		writeJSON(w, 200, map[string]any{
			"original_language":    "en",
			"original_title":       "Test",
			"genres":               []map[string]any{},
			"production_companies": []map[string]any{},
			"production_countries": []map[string]any{},
			"spoken_languages":     []map[string]any{},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "my-secret-key", "fr-FR")
	c.http = srv.Client()

	_, err := c.Primary(context.Background(), "movie", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAPIKey != "my-secret-key" {
		t.Errorf("api_key: want %q, got %q", "my-secret-key", gotAPIKey)
	}
	if gotLang != "fr-FR" {
		t.Errorf("language: want %q, got %q", "fr-FR", gotLang)
	}
}

// ---------------------------------------------------------------------------
// Keywords
// ---------------------------------------------------------------------------

func TestKeywordsMovie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/550/keywords" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{
			"keywords": []map[string]any{
				{"id": 1, "name": "based on novel"},
				{"id": 2, "name": "fight"},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	kw, err := c.Keywords(context.Background(), "movie", 550)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertStrSlice(t, "Keywords", kw, []string{"based on novel", "fight"})
}

func TestKeywordsTV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/1396/keywords" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{"id": 10, "name": "drugs"},
				{"id": 11, "name": "chemistry teacher"},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	kw, err := c.Keywords(context.Background(), "tv", 1396)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertStrSlice(t, "Keywords", kw, []string{"drugs", "chemistry teacher"})
}

func TestKeywordsUnknownMediaTypeReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Keywords(context.Background(), "anime", 1)
	if err == nil {
		t.Fatal("expected error for unknown mediaType, got nil")
	}
}

func TestKeywords404ReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status_message":"not found"}`, 404)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Keywords(context.Background(), "movie", 9999)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// ---------------------------------------------------------------------------
// ContentRating
// ---------------------------------------------------------------------------

func TestContentRatingMovieUS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/550/release_dates" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{
					"iso_3166_1": "GB",
					"release_dates": []map[string]any{
						{"certification": "18"},
					},
				},
				{
					"iso_3166_1": "US",
					"release_dates": []map[string]any{
						{"certification": "R"},
						{"certification": "PG-13"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rating, err := c.ContentRating(context.Background(), "movie", 550)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertStr(t, "ContentRating", rating, "R")
}

func TestContentRatingMovieEmptyCertSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{
					"iso_3166_1": "US",
					"release_dates": []map[string]any{
						{"certification": ""},
						{"certification": "PG"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rating, err := c.ContentRating(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertStr(t, "ContentRating", rating, "PG")
}

func TestContentRatingMovieNoUSReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{
					"iso_3166_1": "DE",
					"release_dates": []map[string]any{
						{"certification": "FSK 18"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rating, err := c.ContentRating(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rating != "" {
		t.Errorf("want empty rating, got %q", rating)
	}
}

func TestContentRatingTVUS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/1396/content_ratings" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{"iso_3166_1": "GB", "rating": "15"},
				{"iso_3166_1": "US", "rating": "TV-MA"},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rating, err := c.ContentRating(context.Background(), "tv", 1396)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertStr(t, "ContentRating", rating, "TV-MA")
}

func TestContentRatingTVNoUSReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{"iso_3166_1": "AU", "rating": "MA15+"},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rating, err := c.ContentRating(context.Background(), "tv", 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rating != "" {
		t.Errorf("want empty rating, got %q", rating)
	}
}

func TestContentRatingUnknownMediaTypeReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ContentRating(context.Background(), "documentary", 1)
	if err == nil {
		t.Fatal("expected error for unknown mediaType, got nil")
	}
}

// ---------------------------------------------------------------------------
// assert helpers
// ---------------------------------------------------------------------------

func assertStr(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %q, got %q", name, want, got)
	}
}

func assertInt(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %d, got %d", name, want, got)
	}
}

func assertFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	// allow small float comparison tolerance
	diff := got - want
	if diff < -0.001 || diff > 0.001 {
		t.Errorf("%s: want %f, got %f", name, want, got)
	}
}

func assertStrSlice(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: want %v, got %v", name, want, got)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: want %q, got %q", name, i, want[i], got[i])
		}
	}
}

// Ensure the routing package import is actually used (it's used via *routing.TMDBPrimary).
var _ *routing.TMDBPrimary = (*routing.TMDBPrimary)(nil)

// Ensure strings package import is used.
var _ = strings.Contains
