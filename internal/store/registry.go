package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RegisteredArr represents a row in the registered_arr table.
type RegisteredArr struct {
	ID                int64
	Name              string
	Kind              string // "radarr"|"sonarr"
	URL               string
	APIKey            string // encrypted at rest (Task 2.1 wraps; this layer is opaque about it)
	RootFolderPath    string
	QualityProfileID  *int
	LanguageProfileID *int
	Priority          int
	Enabled           bool
	RulesJSON         []byte
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreateArr inserts a new RegisteredArr row and returns its assigned ID.
func (s *Store) CreateArr(ctx context.Context, a *RegisteredArr) (int64, error) {
	const q = `
INSERT INTO registered_arr
  (name, kind, url, api_key, root_folder_path,
   quality_profile_id, language_profile_id,
   priority, enabled, rules_json)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id`

	var id int64
	err := s.pool.QueryRow(ctx, q,
		a.Name, a.Kind, a.URL, a.APIKey, a.RootFolderPath,
		a.QualityProfileID, a.LanguageProfileID,
		a.Priority, a.Enabled, a.RulesJSON,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store.CreateArr: %w", err)
	}
	return id, nil
}

// GetArr returns the RegisteredArr with the given ID, or (nil, nil) if no
// such row exists.
func (s *Store) GetArr(ctx context.Context, id int64) (*RegisteredArr, error) {
	const q = `
SELECT id, name, kind, url, api_key, root_folder_path,
       quality_profile_id, language_profile_id,
       priority, enabled, rules_json,
       created_at, updated_at
FROM registered_arr
WHERE id = $1`

	a := &RegisteredArr{}
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&a.ID, &a.Name, &a.Kind, &a.URL, &a.APIKey, &a.RootFolderPath,
		&a.QualityProfileID, &a.LanguageProfileID,
		&a.Priority, &a.Enabled, &a.RulesJSON,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store.GetArr: %w", err)
	}
	return a, nil
}

// ListArrs returns all registered arr instances ordered by (kind, priority, id).
func (s *Store) ListArrs(ctx context.Context) ([]*RegisteredArr, error) {
	const q = `
SELECT id, name, kind, url, api_key, root_folder_path,
       quality_profile_id, language_profile_id,
       priority, enabled, rules_json,
       created_at, updated_at
FROM registered_arr
ORDER BY kind, priority, id`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store.ListArrs: %w", err)
	}
	defer rows.Close()

	return scanArrs(rows)
}

// ListEnabledArrsByKind returns enabled registered arr instances of the given
// kind, ordered by (priority, id).
func (s *Store) ListEnabledArrsByKind(ctx context.Context, kind string) ([]*RegisteredArr, error) {
	const q = `
SELECT id, name, kind, url, api_key, root_folder_path,
       quality_profile_id, language_profile_id,
       priority, enabled, rules_json,
       created_at, updated_at
FROM registered_arr
WHERE kind = $1 AND enabled
ORDER BY priority, id`

	rows, err := s.pool.Query(ctx, q, kind)
	if err != nil {
		return nil, fmt.Errorf("store.ListEnabledArrsByKind: %w", err)
	}
	defer rows.Close()

	return scanArrs(rows)
}

// UpdateArr updates all mutable fields of the given RegisteredArr row,
// setting updated_at = now() via SQL. Returns nil even when no row matched.
func (s *Store) UpdateArr(ctx context.Context, a *RegisteredArr) error {
	const q = `
UPDATE registered_arr
SET name               = $2,
    kind               = $3,
    url                = $4,
    api_key            = $5,
    root_folder_path   = $6,
    quality_profile_id = $7,
    language_profile_id = $8,
    priority           = $9,
    enabled            = $10,
    rules_json         = $11,
    updated_at         = now()
WHERE id = $1`

	_, err := s.pool.Exec(ctx, q,
		a.ID, a.Name, a.Kind, a.URL, a.APIKey, a.RootFolderPath,
		a.QualityProfileID, a.LanguageProfileID,
		a.Priority, a.Enabled, a.RulesJSON,
	)
	if err != nil {
		return fmt.Errorf("store.UpdateArr: %w", err)
	}
	return nil
}

// DeleteArr removes the RegisteredArr row with the given ID.
func (s *Store) DeleteArr(ctx context.Context, id int64) error {
	const q = `DELETE FROM registered_arr WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store.DeleteArr: %w", err)
	}
	return nil
}

// scanArrs drains a pgx.Rows cursor into a slice of *RegisteredArr.
func scanArrs(rows pgx.Rows) ([]*RegisteredArr, error) {
	var result []*RegisteredArr
	for rows.Next() {
		a := &RegisteredArr{}
		if err := rows.Scan(
			&a.ID, &a.Name, &a.Kind, &a.URL, &a.APIKey, &a.RootFolderPath,
			&a.QualityProfileID, &a.LanguageProfileID,
			&a.Priority, &a.Enabled, &a.RulesJSON,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store.scanArrs: %w", err)
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store.scanArrs: %w", err)
	}
	return result, nil
}
