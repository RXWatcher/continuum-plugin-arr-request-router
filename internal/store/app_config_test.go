package store_test

import (
	"context"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arr-request-router/internal/store"
)

func TestGetAppConfigReturnsDefaultsAndGeneratedSecret(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetAppConfig(context.Background())
	if err != nil {
		t.Fatalf("get app config: %v", err)
	}

	if got.TMDBLanguage != store.DefaultTMDBLanguage {
		t.Fatalf("tmdb language = %q", got.TMDBLanguage)
	}
	if got.PollIntervalSeconds != store.DefaultPollIntervalSeconds {
		t.Fatalf("poll interval = %d", got.PollIntervalSeconds)
	}
	if got.StaleAfterHours != store.DefaultStaleAfterHours {
		t.Fatalf("stale after = %d", got.StaleAfterHours)
	}
	if got.SecretKey == "" {
		t.Fatal("secret key was not generated")
	}
}

func TestUpsertAppConfigPersistsValues(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	want := store.AppConfig{
		TMDBAPIKey:          "tmdb-key",
		TMDBLanguage:        "fr-FR",
		PollIntervalSeconds: 45,
		StaleAfterHours:     96,
		SecretKey:           "a-secret-key-that-is-long-enough",
	}

	if err := s.UpsertAppConfig(ctx, want); err != nil {
		t.Fatalf("upsert app config: %v", err)
	}
	got, err := s.GetAppConfig(ctx)
	if err != nil {
		t.Fatalf("get app config: %v", err)
	}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

func TestUpsertAppConfigNormalizesBounds(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertAppConfig(ctx, store.AppConfig{
		PollIntervalSeconds: 1,
		StaleAfterHours:     -1,
		SecretKey:           "a-secret-key-that-is-long-enough",
	}); err != nil {
		t.Fatalf("upsert app config: %v", err)
	}
	got, err := s.GetAppConfig(ctx)
	if err != nil {
		t.Fatalf("get app config: %v", err)
	}
	if got.PollIntervalSeconds != store.MinPollIntervalSeconds {
		t.Fatalf("poll interval = %d, want %d", got.PollIntervalSeconds, store.MinPollIntervalSeconds)
	}
	if got.StaleAfterHours != store.DefaultStaleAfterHours {
		t.Fatalf("stale after = %d, want %d", got.StaleAfterHours, store.DefaultStaleAfterHours)
	}
}

func TestImportLegacyAppConfigImportsOnlyIntoDefaultConfig(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	legacy := store.AppConfig{
		TMDBAPIKey:          "legacy-tmdb",
		TMDBLanguage:        "de-DE",
		PollIntervalSeconds: 60,
		StaleAfterHours:     120,
		SecretKey:           "legacy-secret-key-long-enough",
	}

	imported, err := s.ImportLegacyAppConfig(ctx, legacy)
	if err != nil {
		t.Fatalf("import legacy: %v", err)
	}
	if !imported {
		t.Fatal("expected import into default config")
	}
	got, err := s.GetAppConfig(ctx)
	if err != nil {
		t.Fatalf("get app config: %v", err)
	}
	if got != legacy {
		t.Fatalf("config = %+v, want legacy %+v", got, legacy)
	}
}

func TestImportLegacyAppConfigDoesNotOverwriteManagedConfig(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	managed := store.AppConfig{
		TMDBLanguage:        store.DefaultTMDBLanguage,
		PollIntervalSeconds: store.DefaultPollIntervalSeconds,
		StaleAfterHours:     store.DefaultStaleAfterHours,
		SecretKey:           "managed-secret-key-long-enough",
	}
	if err := s.UpsertAppConfig(ctx, managed); err != nil {
		t.Fatalf("upsert managed config: %v", err)
	}

	imported, err := s.ImportLegacyAppConfig(ctx, store.AppConfig{
		TMDBAPIKey:          "legacy-tmdb",
		TMDBLanguage:        "de-DE",
		PollIntervalSeconds: 60,
		StaleAfterHours:     120,
		SecretKey:           "legacy-secret-key-long-enough",
	})
	if err != nil {
		t.Fatalf("import legacy: %v", err)
	}
	if imported {
		t.Fatal("expected import to skip managed config")
	}
	got, err := s.GetAppConfig(ctx)
	if err != nil {
		t.Fatalf("get app config: %v", err)
	}
	if got != managed {
		t.Fatalf("config = %+v, want managed %+v", got, managed)
	}
}
