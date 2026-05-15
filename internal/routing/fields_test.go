package routing_test

import (
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/routing"
)

// ── Group A ──────────────────────────────────────────────────────────────────

func TestGroupAFields(t *testing.T) {
	ctx := routing.Context{
		Event: routing.RequestEvent{
			RequestID:        "req-001",
			MediaType:        "movie",
			LibraryID:        "lib-42",
			Year:             1999,
			RequesterUserID:  "user-7",
			RequesterIsAdmin: true,
			Title:            "The Matrix",
			TMDBID:           603,
			PosterURL:        "https://example.com/poster.jpg",
		},
	}

	tests := []struct {
		field string
		want  any
	}{
		{"mediaType", "movie"},
		{"libraryId", "lib-42"},
		{"year", 1999},
		{"decade", 1990},
		{"requesterUserId", "user-7"},
		{"requesterIsAdmin", true},
		{"title", "The Matrix"},
		{"tmdbId", 603},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, ok := routing.GetField(ctx, tt.field)
			if !ok {
				t.Fatalf("GetField(%q) ok=false, want true", tt.field)
			}
			if got != tt.want {
				t.Fatalf("GetField(%q) = %v (%T), want %v (%T)", tt.field, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestGroupADecadeIsDerived(t *testing.T) {
	ctx := routing.Context{Event: routing.RequestEvent{Year: 2003}}
	got, ok := routing.GetField(ctx, "decade")
	if !ok {
		t.Fatal("GetField(decade) ok=false")
	}
	if got != 2000 {
		t.Fatalf("decade: got %v want 2000", got)
	}
}

func TestGroupAUnknownFieldReturnsNotOK(t *testing.T) {
	ctx := routing.Context{Event: routing.RequestEvent{Year: 2020}}
	_, ok := routing.GetField(ctx, "banana")
	if ok {
		t.Fatal("GetField(banana) ok=true, want false")
	}
}

// ── Helpers for shared test data ──────────────────────────────────────────────

var allKnownFieldNames = []string{
	// Group A
	"mediaType", "libraryId", "year", "decade",
	"requesterUserId", "requesterIsAdmin", "title", "tmdbId",
	// Group B common
	"original_language", "original_title", "genres", "runtime",
	"vote_average", "vote_count", "popularity", "adult", "status",
	"production_companies", "production_countries", "spoken_languages",
	// Group B movie-only
	"release_date", "budget", "revenue", "belongs_to_collection", "imdb_id",
	// Group B tv-only
	"networks", "origin_country", "first_air_date", "last_air_date",
	"type", "in_production", "number_of_seasons", "number_of_episodes", "created_by",
	// Group C
	"keywords", "content_rating",
}

func fullyPopulatedTMDBPrimary() *routing.TMDBPrimary {
	return &routing.TMDBPrimary{
		MediaType:           "movie",
		OriginalLanguage:    "en",
		OriginalTitle:       "Test Title",
		Genres:              []string{"Drama"},
		Runtime:             120,
		VoteAverage:         8.0,
		VoteCount:           5000,
		Popularity:          50.0,
		Adult:               false,
		Status:              "Released",
		ProductionCompanies: []string{"Test Studio"},
		ProductionCountries: []string{"US"},
		SpokenLanguages:     []string{"English"},
		ReleaseDate:         "2001-01-01",
		Budget:              100000000,
		Revenue:             500000000,
		BelongsToCollection: "Test Collection",
		IMDBID:              "tt0000000",
		Networks:            []string{"Test Network"},
		OriginCountry:       []string{"US"},
		FirstAirDate:        "2001-01-01",
		LastAirDate:         "2010-12-31",
		Type:                "Scripted",
		InProduction:        false,
		NumberOfSeasons:     5,
		NumberOfEpisodes:    50,
		CreatedBy:           []string{"Test Creator"},
	}
}

// ── Group B ───────────────────────────────────────────────────────────────────

func makeMoviePrimary() *routing.TMDBPrimary {
	return &routing.TMDBPrimary{
		MediaType:           "movie",
		OriginalLanguage:    "en",
		OriginalTitle:       "The Matrix",
		Genres:              []string{"Action", "Sci-Fi"},
		Runtime:             136,
		VoteAverage:         8.7,
		VoteCount:           22000,
		Popularity:          85.3,
		Adult:               false,
		Status:              "Released",
		ProductionCompanies: []string{"Warner Bros."},
		ProductionCountries: []string{"US"},
		SpokenLanguages:     []string{"English"},
		ReleaseDate:         "1999-03-31",
		Budget:              63000000,
		Revenue:             463517383,
		BelongsToCollection: "The Matrix Collection",
		IMDBID:              "tt0133093",
	}
}

func makeTVPrimary() *routing.TMDBPrimary {
	return &routing.TMDBPrimary{
		MediaType:        "tv",
		OriginalLanguage: "en",
		OriginalTitle:    "Breaking Bad",
		Genres:           []string{"Drama", "Crime"},
		Runtime:          47,
		VoteAverage:      9.5,
		VoteCount:        12000,
		Popularity:       200.0,
		Adult:            false,
		Status:           "Ended",
		Networks:         []string{"AMC"},
		OriginCountry:    []string{"US"},
		FirstAirDate:     "2008-01-20",
		LastAirDate:      "2013-09-29",
		Type:             "Scripted",
		InProduction:     false,
		NumberOfSeasons:  5,
		NumberOfEpisodes: 62,
		CreatedBy:        []string{"Vince Gilligan"},
	}
}

func TestGroupBLoadedReturnsValues(t *testing.T) {
	ctx := routing.Context{
		Event:   routing.RequestEvent{MediaType: "movie"},
		Primary: makeMoviePrimary(),
	}

	tests := []struct {
		field string
		want  any
	}{
		{"release_date", "1999-03-31"},
		{"original_language", "en"},
		{"runtime", 136},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, ok := routing.GetField(ctx, tt.field)
			if !ok {
				t.Fatalf("GetField(%q) ok=false, want true", tt.field)
			}
			if got != tt.want {
				t.Fatalf("GetField(%q) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}

	// genres is a slice — check separately
	t.Run("genres", func(t *testing.T) {
		got, ok := routing.GetField(ctx, "genres")
		if !ok {
			t.Fatal("GetField(genres) ok=false")
		}
		genres, ok2 := got.([]string)
		if !ok2 {
			t.Fatalf("genres: got %T, want []string", got)
		}
		if len(genres) != 2 || genres[0] != "Action" {
			t.Fatalf("genres: got %v", genres)
		}
	})
}

func TestGroupBUnloadedReturnsNotOK(t *testing.T) {
	ctx := routing.Context{
		Event:   routing.RequestEvent{MediaType: "movie"},
		Primary: nil,
	}
	_, ok := routing.GetField(ctx, "original_language")
	if ok {
		t.Fatal("GetField(original_language) with Primary=nil: ok=true, want false")
	}
}

func TestGroupBMovieOnlyAbsentOnTVKind(t *testing.T) {
	ctx := routing.Context{
		Event:   routing.RequestEvent{MediaType: "tv"},
		Primary: makeMoviePrimary(), // Primary is populated but MediaType says tv
	}

	movieOnlyFields := []string{"release_date", "budget", "imdb_id"}
	for _, f := range movieOnlyFields {
		t.Run(f, func(t *testing.T) {
			_, ok := routing.GetField(ctx, f)
			if ok {
				t.Fatalf("GetField(%q) on tv context: ok=true, want false", f)
			}
		})
	}
}

func TestGroupBTVOnlyAbsentOnMovieKind(t *testing.T) {
	ctx := routing.Context{
		Event:   routing.RequestEvent{MediaType: "movie"},
		Primary: makeTVPrimary(), // Primary is populated but MediaType says movie
	}

	tvOnlyFields := []string{"networks", "origin_country", "type", "first_air_date", "in_production", "number_of_seasons"}
	for _, f := range tvOnlyFields {
		t.Run(f, func(t *testing.T) {
			_, ok := routing.GetField(ctx, f)
			if ok {
				t.Fatalf("GetField(%q) on movie context: ok=true, want false", f)
			}
		})
	}
}

// ── Group C keywords ──────────────────────────────────────────────────────────

func TestGroupCKeywordsLoaded(t *testing.T) {
	ctx := routing.Context{
		Keywords: []string{"anime", "time travel"},
	}
	got, ok := routing.GetField(ctx, "keywords")
	if !ok {
		t.Fatal("GetField(keywords) ok=false, want true")
	}
	kws, ok2 := got.([]string)
	if !ok2 {
		t.Fatalf("keywords: got %T, want []string", got)
	}
	if len(kws) != 2 || kws[0] != "anime" {
		t.Fatalf("keywords: got %v", kws)
	}
}

func TestGroupCKeywordsUnloaded(t *testing.T) {
	ctx := routing.Context{Keywords: nil}
	_, ok := routing.GetField(ctx, "keywords")
	if ok {
		t.Fatal("GetField(keywords) with nil slice: ok=true, want false")
	}
}

func TestGroupCKeywordsEmptyStillLoaded(t *testing.T) {
	ctx := routing.Context{Keywords: []string{}}
	got, ok := routing.GetField(ctx, "keywords")
	if !ok {
		t.Fatal("GetField(keywords) with empty slice: ok=false, want true")
	}
	kws, ok2 := got.([]string)
	if !ok2 {
		t.Fatalf("keywords: got %T, want []string", got)
	}
	if len(kws) != 0 {
		t.Fatalf("keywords: expected empty slice, got %v", kws)
	}
}

// ── Group C content_rating ────────────────────────────────────────────────────

func TestGroupCContentRating(t *testing.T) {
	t.Run("loaded", func(t *testing.T) {
		ctx := routing.Context{ContentRating: "TV-MA"}
		got, ok := routing.GetField(ctx, "content_rating")
		if !ok {
			t.Fatal("GetField(content_rating) ok=false, want true")
		}
		if got != "TV-MA" {
			t.Fatalf("content_rating: got %v, want TV-MA", got)
		}
	})
	t.Run("empty string is unloaded", func(t *testing.T) {
		ctx := routing.Context{ContentRating: ""}
		_, ok := routing.GetField(ctx, "content_rating")
		if ok {
			t.Fatal("GetField(content_rating) with empty string: ok=true, want false")
		}
	})
}

// ── FieldGroupOf ──────────────────────────────────────────────────────────────

func TestFieldGroupOfKnownAndUnknown(t *testing.T) {
	tests := []struct {
		name      string
		wantGroup routing.FieldGroup
		wantOK    bool
	}{
		{"year", routing.GroupA, true},
		{"genres", routing.GroupB, true},
		{"keywords", routing.GroupCKeywords, true},
		{"content_rating", routing.GroupCContentRating, true},
		{"banana", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, ok := routing.FieldGroupOf(tt.name)
			if ok != tt.wantOK {
				t.Fatalf("FieldGroupOf(%q) ok=%v, want %v", tt.name, ok, tt.wantOK)
			}
			if ok && g != tt.wantGroup {
				t.Fatalf("FieldGroupOf(%q) group=%v, want %v", tt.name, g, tt.wantGroup)
			}
		})
	}
}

// ── KnownField via registry ───────────────────────────────────────────────────

func TestKnownFieldAcceptsAllRegistered(t *testing.T) {
	if len(allKnownFieldNames) != 36 {
		t.Fatalf("allKnownFieldNames has %d fields, want exactly 36; if you added a field to the registry, update this list", len(allKnownFieldNames))
	}
	for _, name := range allKnownFieldNames {
		t.Run(name, func(t *testing.T) {
			if !routing.KnownField(name) {
				t.Fatalf("KnownField(%q) = false, want true", name)
			}
		})
	}
}

func TestKnownFieldRejectsOpNames(t *testing.T) {
	opNames := []string{"between", "regex", "eq", "contains"}
	for _, op := range opNames {
		t.Run(op, func(t *testing.T) {
			if routing.KnownField(op) {
				t.Fatalf("KnownField(%q) = true, want false (op name should not be a field)", op)
			}
		})
	}
}

// ── Integration: ValidateRules accepts real field names ───────────────────────

func TestValidateRulesAcceptsRealFieldNames(t *testing.T) {
	r := routing.Rules{
		Match: "all",
		Groups: []routing.Group{{
			Match: "all",
			Rules: []routing.Rule{
				{Field: "keywords", Op: "contains", Value: []byte(`"anime"`)},
			},
		}},
	}
	if err := routing.ValidateRules(r); err != nil {
		t.Fatalf("ValidateRules with field=keywords: unexpected error: %v", err)
	}
}

// ── Consistency: registry drift detection ─────────────────────────────────────

// TestFieldRegistryConsistency catches drift if a future change adds a
// field to fieldRegistry but forgets to add it to movieOnlyFields /
// tvOnlyFields, or vice versa. We can't reach the unexported maps
// directly, so we rely on GetField behavior:
//   - For each registered Group B field, exactly one of these must hold:
//       a) it is movie+tv (loaded under both kinds when Primary is set), or
//       b) it is movie-only (loaded only on movie kind), or
//       c) it is tv-only (loaded only on tv kind).
// We don't have to know which list it's in — we just have to confirm
// the runtime behavior is one of those three categories. Anything else
// (e.g., a field that returns ok=false on both kinds even with Primary set)
// is a registry/kind-map drift bug.
func TestFieldRegistryConsistency(t *testing.T) {
	primary := fullyPopulatedTMDBPrimary()
	movieCtx := routing.Context{Event: routing.RequestEvent{MediaType: "movie"}, Primary: primary}
	tvCtx := routing.Context{Event: routing.RequestEvent{MediaType: "tv"}, Primary: primary}

	for _, name := range allKnownFieldNames {
		g, ok := routing.FieldGroupOf(name)
		if !ok {
			t.Errorf("FieldGroupOf(%q) = false but name is in test list", name)
			continue
		}
		if g != routing.GroupB {
			continue // only Group B has kind gating
		}

		_, okMovie := routing.GetField(movieCtx, name)
		_, okTV := routing.GetField(tvCtx, name)
		if !okMovie && !okTV {
			t.Errorf("field %q is GroupB but resolves to ok=false on both movie and tv kinds — registry/kind-map drift", name)
		}
	}
}
