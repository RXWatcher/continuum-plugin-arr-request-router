package runtime

import (
	"strings"
	"testing"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// makeEntry builds a ConfigEntry whose Value is a structpb.Struct containing the
// supplied fields — mirroring the {"value": ...} envelope used in the manifest
// json_schema and the existing {"value": ...} envelope convention.
func makeEntry(key string, fields map[string]any) *pluginv1.ConfigEntry {
	s, err := structpb.NewStruct(fields)
	if err != nil {
		panic("makeEntry: " + err.Error())
	}
	return &pluginv1.ConfigEntry{Key: key, Value: s}
}

// requiredEntries returns the three required entries with valid values.
func requiredEntries() []*pluginv1.ConfigEntry {
	return []*pluginv1.ConfigEntry{
		makeEntry("database_url", map[string]any{"value": "postgres://user:pass@host:5432/db"}),
		makeEntry("tmdb.api_key", map[string]any{"value": "apikey123"}),
		makeEntry("secret_key", map[string]any{"value": "exactly16charkey"}),
	}
}

func TestLoadConfigFullValid(t *testing.T) {
	entries := []*pluginv1.ConfigEntry{
		makeEntry("database_url", map[string]any{"value": "postgres://user:pass@host/db"}),
		makeEntry("tmdb.api_key", map[string]any{"value": "tmdb-key-xyz"}),
		makeEntry("tmdb.language", map[string]any{"value": "fr-FR"}),
		makeEntry("poll_interval_seconds", map[string]any{"value": float64(60)}),
		makeEntry("stale_after_hours", map[string]any{"value": float64(48)}),
		makeEntry("secret_key", map[string]any{"value": "a-secret-key-that-is-long-enough"}),
	}
	cfg, err := loadConfig(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://user:pass@host/db" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.TMDBAPIKey != "tmdb-key-xyz" {
		t.Errorf("TMDBAPIKey = %q", cfg.TMDBAPIKey)
	}
	if cfg.TMDBLanguage != "fr-FR" {
		t.Errorf("TMDBLanguage = %q, want fr-FR", cfg.TMDBLanguage)
	}
	if cfg.PollIntervalSeconds != 60 {
		t.Errorf("PollIntervalSeconds = %d, want 60", cfg.PollIntervalSeconds)
	}
	if cfg.StaleAfterHours != 48 {
		t.Errorf("StaleAfterHours = %d, want 48", cfg.StaleAfterHours)
	}
	if cfg.SecretKey != "a-secret-key-that-is-long-enough" {
		t.Errorf("SecretKey = %q", cfg.SecretKey)
	}
}

func TestLoadConfigDefaultsApply(t *testing.T) {
	cfg, err := loadConfig(requiredEntries())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TMDBLanguage != defaultTMDBLanguage {
		t.Errorf("TMDBLanguage = %q, want %q", cfg.TMDBLanguage, defaultTMDBLanguage)
	}
	if cfg.PollIntervalSeconds != defaultPollIntervalSeconds {
		t.Errorf("PollIntervalSeconds = %d, want %d", cfg.PollIntervalSeconds, defaultPollIntervalSeconds)
	}
	if cfg.StaleAfterHours != defaultStaleAfterHours {
		t.Errorf("StaleAfterHours = %d, want %d", cfg.StaleAfterHours, defaultStaleAfterHours)
	}
}

func TestLoadConfigMissingDatabaseURLErrors(t *testing.T) {
	entries := []*pluginv1.ConfigEntry{
		makeEntry("tmdb.api_key", map[string]any{"value": "apikey"}),
		makeEntry("secret_key", map[string]any{"value": "exactly16charkey"}),
	}
	_, err := loadConfig(entries)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "database_url") {
		t.Errorf("error %q does not mention database_url", err.Error())
	}
}

func TestLoadConfigMissingTMDBAPIKeyAllowed(t *testing.T) {
	entries := []*pluginv1.ConfigEntry{
		makeEntry("database_url", map[string]any{"value": "postgres://host/db"}),
		makeEntry("secret_key", map[string]any{"value": "exactly16charkey"}),
	}
	cfg, err := loadConfig(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TMDBAPIKey != "" {
		t.Errorf("TMDBAPIKey = %q, want empty", cfg.TMDBAPIKey)
	}
}

func TestLoadConfigMissingSecretKeyAllowed(t *testing.T) {
	entries := []*pluginv1.ConfigEntry{
		makeEntry("database_url", map[string]any{"value": "postgres://host/db"}),
		makeEntry("tmdb.api_key", map[string]any{"value": "apikey"}),
	}
	cfg, err := loadConfig(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SecretKey != "" {
		t.Errorf("SecretKey = %q, want empty", cfg.SecretKey)
	}
}

func TestLoadConfigSecretKeyTooShortErrors(t *testing.T) {
	entries := []*pluginv1.ConfigEntry{
		makeEntry("database_url", map[string]any{"value": "postgres://host/db"}),
		makeEntry("tmdb.api_key", map[string]any{"value": "apikey"}),
		makeEntry("secret_key", map[string]any{"value": "tooshort"}), // 8 chars < 16
	}
	_, err := loadConfig(entries)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "secret_key") {
		t.Errorf("error %q does not mention secret_key", err.Error())
	}
}

func TestLoadConfigEmptyTMDBLanguageDefaults(t *testing.T) {
	entries := append(requiredEntries(),
		makeEntry("tmdb.language", map[string]any{"value": ""}),
	)
	cfg, err := loadConfig(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TMDBLanguage != defaultTMDBLanguage {
		t.Errorf("TMDBLanguage = %q, want %q (should default on empty)", cfg.TMDBLanguage, defaultTMDBLanguage)
	}
}

func TestLoadConfigZeroPollIntervalDefaults(t *testing.T) {
	entries := append(requiredEntries(),
		makeEntry("poll_interval_seconds", map[string]any{"value": float64(0)}),
	)
	cfg, err := loadConfig(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0 is treated as absent; default applies but then clamped to min (10) if default < min.
	// Default is 30 which is >= min(10), so should get 30.
	if cfg.PollIntervalSeconds != defaultPollIntervalSeconds {
		t.Errorf("PollIntervalSeconds = %d, want %d (should default when 0)", cfg.PollIntervalSeconds, defaultPollIntervalSeconds)
	}
}
