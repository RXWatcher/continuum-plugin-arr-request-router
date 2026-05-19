package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const (
	DefaultTMDBLanguage        = "en-US"
	DefaultPollIntervalSeconds = 30
	MinPollIntervalSeconds     = 10
	MaxPollIntervalSeconds     = 600
	DefaultStaleAfterHours     = 72
)

// AppConfig is the plugin-owned operational configuration stored in
// app_config. database_url remains host-managed bootstrap config.
type AppConfig struct {
	TMDBAPIKey          string `json:"tmdb_api_key"`
	TMDBLanguage        string `json:"tmdb_language"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	StaleAfterHours     int    `json:"stale_after_hours"`
	SecretKey           string `json:"secret_key"`
}

type appConfigRecord struct {
	AppConfig
	Managed bool `json:"managed,omitempty"`
}

func DefaultAppConfig() AppConfig {
	return AppConfig{
		TMDBLanguage:        DefaultTMDBLanguage,
		PollIntervalSeconds: DefaultPollIntervalSeconds,
		StaleAfterHours:     DefaultStaleAfterHours,
	}
}

func NormalizeAppConfig(cfg AppConfig) AppConfig {
	if cfg.TMDBLanguage == "" {
		cfg.TMDBLanguage = DefaultTMDBLanguage
	}
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = DefaultPollIntervalSeconds
	}
	if cfg.PollIntervalSeconds < MinPollIntervalSeconds {
		cfg.PollIntervalSeconds = MinPollIntervalSeconds
	}
	if cfg.PollIntervalSeconds > MaxPollIntervalSeconds {
		cfg.PollIntervalSeconds = MaxPollIntervalSeconds
	}
	if cfg.StaleAfterHours <= 0 {
		cfg.StaleAfterHours = DefaultStaleAfterHours
	}
	return cfg
}

func (s *Store) GetAppConfig(ctx context.Context) (AppConfig, error) {
	if err := s.ensureAppConfig(ctx); err != nil {
		return AppConfig{}, err
	}
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT data FROM app_config WHERE id = 1`).Scan(&raw); err != nil {
		return AppConfig{}, err
	}
	cfg, err := decodeAppConfig(raw)
	if err != nil {
		return AppConfig{}, err
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey, err = generateSecretKey()
		if err != nil {
			return AppConfig{}, err
		}
		if err := s.upsertAppConfigRecord(ctx, cfg, true); err != nil {
			return AppConfig{}, err
		}
	}
	return cfg, nil
}

func (s *Store) UpsertAppConfig(ctx context.Context, cfg AppConfig) error {
	return s.upsertAppConfigRecord(ctx, cfg, true)
}

func (s *Store) ImportLegacyAppConfig(ctx context.Context, cfg AppConfig) (bool, error) {
	legacy := NormalizeAppConfig(cfg)
	if legacy == DefaultAppConfig() {
		return false, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO app_config (id, data)
		 VALUES (1, '{}'::jsonb)
		 ON CONFLICT (id) DO NOTHING`,
	); err != nil {
		return false, err
	}

	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT data FROM app_config WHERE id = 1 FOR UPDATE`).Scan(&raw); err != nil {
		return false, err
	}
	current, err := decodeAppConfig(raw)
	if err != nil {
		return false, err
	}
	if appConfigManaged(raw) || current != DefaultAppConfig() {
		return false, tx.Commit(ctx)
	}
	if legacy.SecretKey == "" {
		legacy.SecretKey, err = generateSecretKey()
		if err != nil {
			return false, err
		}
	}
	next, err := json.Marshal(appConfigRecord{
		AppConfig: NormalizeAppConfig(legacy),
		Managed:   true,
	})
	if err != nil {
		return false, fmt.Errorf("marshal legacy app config: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE app_config SET data = $1, updated_at = now() WHERE id = 1`, next); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Store) ensureAppConfig(ctx context.Context) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO app_config (id, data)
		 VALUES (1, '{}'::jsonb)
		 ON CONFLICT (id) DO NOTHING`,
	)
	if err != nil {
		return fmt.Errorf("ensure app config: %w", err)
	}
	return nil
}

func (s *Store) upsertAppConfigRecord(ctx context.Context, cfg AppConfig, managed bool) error {
	raw, err := json.Marshal(appConfigRecord{
		AppConfig: NormalizeAppConfig(cfg),
		Managed:   managed,
	})
	if err != nil {
		return fmt.Errorf("marshal app config: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO app_config (id, data, updated_at)
		 VALUES (1, $1, now())
		 ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`,
		raw,
	)
	if err != nil {
		return fmt.Errorf("upsert app config: %w", err)
	}
	return nil
}

func decodeAppConfig(raw []byte) (AppConfig, error) {
	cfg := DefaultAppConfig()
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("decode app config: %w", err)
	}
	return NormalizeAppConfig(cfg), nil
}

func appConfigManaged(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var record appConfigRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return false
	}
	return record.Managed
}

func generateSecretKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate secret key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b[:]), nil
}
