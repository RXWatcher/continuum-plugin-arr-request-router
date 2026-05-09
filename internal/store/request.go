package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Request represents a row in the request table.
type Request struct {
	ID               string
	TMDBID           int
	MediaType        string // "movie"|"tv"
	Title            string
	Year             int
	PosterURL        string
	RequesterUserID  string
	RequesterIsAdmin bool
	Status           string // queued|submitted|downloading|imported|failed|cancelled|unrouted
	RoutedArrID      *int64
	ExternalID       *int
	Error            string
	MatchTrace       []byte
	SubmittedAt      *time.Time
	LastPolledAt     *time.Time
	CompletedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// UpsertRequestQueued inserts a new Request with status='queued'. If a row
// with the same id already exists, the insert is a no-op (ON CONFLICT DO NOTHING).
func (s *Store) UpsertRequestQueued(ctx context.Context, r *Request) error {
	const q = `
INSERT INTO request
  (id, tmdb_id, media_type, title, year, poster_url,
   requester_user_id, requester_is_admin, status)
VALUES
  ($1, $2, $3, $4, $5, $6, $7, $8, 'queued')
ON CONFLICT (id) DO NOTHING`

	_, err := s.pool.Exec(ctx, q,
		r.ID, r.TMDBID, r.MediaType, r.Title, r.Year, r.PosterURL,
		r.RequesterUserID, r.RequesterIsAdmin,
	)
	if err != nil {
		return fmt.Errorf("store.UpsertRequestQueued: %w", err)
	}
	return nil
}

// GetRequest returns the Request with the given id, or (nil, nil) if no
// such row exists.
func (s *Store) GetRequest(ctx context.Context, id string) (*Request, error) {
	const q = `
SELECT id, tmdb_id, media_type, title, year, poster_url,
       requester_user_id, requester_is_admin, status,
       routed_arr_id, external_id, error, match_trace,
       submitted_at, last_polled_at, completed_at,
       created_at, updated_at
FROM request
WHERE id = $1`

	r := &Request{}
	var errMsg *string
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.TMDBID, &r.MediaType, &r.Title, &r.Year, &r.PosterURL,
		&r.RequesterUserID, &r.RequesterIsAdmin, &r.Status,
		&r.RoutedArrID, &r.ExternalID, &errMsg, &r.MatchTrace,
		&r.SubmittedAt, &r.LastPolledAt, &r.CompletedAt,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store.GetRequest: %w", err)
	}
	if errMsg != nil {
		r.Error = *errMsg
	}
	return r, nil
}

// MarkSubmitted transitions the request to status='submitted', sets
// external_id and submitted_at. Idempotent: also matches rows already in
// 'submitted' state.
func (s *Store) MarkSubmitted(ctx context.Context, id string, externalID int) error {
	const q = `
UPDATE request
SET status      = 'submitted',
    external_id = $2,
    submitted_at = now(),
    updated_at  = now()
WHERE id = $1
  AND status IN ('queued','submitted')`

	_, err := s.pool.Exec(ctx, q, id, externalID)
	if err != nil {
		return fmt.Errorf("store.MarkSubmitted: %w", err)
	}
	return nil
}

// MarkDownloading transitions the request to status='downloading'. It returns
// (true, nil) when a row was actually transitioned (from queued or submitted),
// and (false, nil) when the row was already downloading or in a terminal state.
func (s *Store) MarkDownloading(ctx context.Context, id string) (transitioned bool, err error) {
	const q = `
UPDATE request
SET status     = 'downloading',
    updated_at = now()
WHERE id = $1
  AND status IN ('queued','submitted')`

	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return false, fmt.Errorf("store.MarkDownloading: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// MarkImported transitions the request to status='imported' and sets
// completed_at. It is a no-op when the row is already in a terminal state.
func (s *Store) MarkImported(ctx context.Context, id string) error {
	const q = `
UPDATE request
SET status       = 'imported',
    completed_at = now(),
    updated_at   = now()
WHERE id = $1
  AND status IN ('submitted','downloading')`

	_, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store.MarkImported: %w", err)
	}
	return nil
}

// MarkFailed transitions the request to status='failed', records the error
// message, and sets completed_at. It is a no-op when the row is already in a
// terminal state.
func (s *Store) MarkFailed(ctx context.Context, id string, msg string) error {
	const q = `
UPDATE request
SET status       = 'failed',
    error        = $2,
    completed_at = now(),
    updated_at   = now()
WHERE id = $1
  AND status IN ('queued','submitted','downloading')`

	_, err := s.pool.Exec(ctx, q, id, msg)
	if err != nil {
		return fmt.Errorf("store.MarkFailed: %w", err)
	}
	return nil
}

// MarkCancelled transitions the request to status='cancelled' and sets
// completed_at. It is a no-op when the row is already in a terminal state.
func (s *Store) MarkCancelled(ctx context.Context, id string) error {
	const q = `
UPDATE request
SET status       = 'cancelled',
    completed_at = now(),
    updated_at   = now()
WHERE id = $1
  AND status IN ('queued','submitted','downloading')`

	_, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store.MarkCancelled: %w", err)
	}
	return nil
}

// MarkUnrouted transitions the request to status='unrouted', stores the
// match_trace JSONB, sets error to the reason string, and sets completed_at.
// It is a no-op when the row is not in the 'queued' state (unrouted only
// happens at routing time, before any *arr is contacted).
func (s *Store) MarkUnrouted(ctx context.Context, id string, trace []byte, reason string) error {
	const q = `
UPDATE request
SET status       = 'unrouted',
    match_trace  = $2,
    error        = $3,
    completed_at = now(),
    updated_at   = now()
WHERE id = $1
  AND status IN ('queued')`

	_, err := s.pool.Exec(ctx, q, id, trace, reason)
	if err != nil {
		return fmt.Errorf("store.MarkUnrouted: %w", err)
	}
	return nil
}

// SetRoutedArr stores the routed_arr_id and match_trace on the request row.
func (s *Store) SetRoutedArr(ctx context.Context, id string, arrID int64, trace []byte) error {
	const q = `
UPDATE request
SET routed_arr_id = $2,
    match_trace   = $3,
    updated_at    = now()
WHERE id = $1`

	_, err := s.pool.Exec(ctx, q, id, arrID, trace)
	if err != nil {
		return fmt.Errorf("store.SetRoutedArr: %w", err)
	}
	return nil
}

// ListPollable returns all requests whose status is 'submitted' or
// 'downloading', ordered by id.
func (s *Store) ListPollable(ctx context.Context) ([]*Request, error) {
	const q = `
SELECT id, tmdb_id, media_type, title, year, poster_url,
       requester_user_id, requester_is_admin, status,
       routed_arr_id, external_id, error, match_trace,
       submitted_at, last_polled_at, completed_at,
       created_at, updated_at
FROM request
WHERE status IN ('submitted','downloading')
ORDER BY id`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store.ListPollable: %w", err)
	}
	defer rows.Close()

	return scanRequests(rows)
}

// UpdateLastPolled sets last_polled_at on the request row. It deliberately does
// NOT touch updated_at to avoid noise in change-tracking queries.
func (s *Store) UpdateLastPolled(ctx context.Context, id string, t time.Time) error {
	const q = `UPDATE request SET last_polled_at = $2 WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id, t)
	if err != nil {
		return fmt.Errorf("store.UpdateLastPolled: %w", err)
	}
	return nil
}

// ListRequestsForAdmin returns a paginated list of requests and the total count.
// When status is empty, all rows are returned; otherwise only rows matching the
// given status are included. Results are ordered by (created_at DESC, id DESC).
// A single query with a window function is used to avoid the COUNT/SELECT race.
func (s *Store) ListRequestsForAdmin(ctx context.Context, status string, limit, offset int) ([]*Request, int, error) {
	const allQ = `
SELECT id, tmdb_id, media_type, title, year, poster_url,
       requester_user_id, requester_is_admin, status,
       routed_arr_id, external_id, error, match_trace,
       submitted_at, last_polled_at, completed_at,
       created_at, updated_at,
       COUNT(*) OVER() AS total
FROM request
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2`

	const filteredQ = `
SELECT id, tmdb_id, media_type, title, year, poster_url,
       requester_user_id, requester_is_admin, status,
       routed_arr_id, external_id, error, match_trace,
       submitted_at, last_polled_at, completed_at,
       created_at, updated_at,
       COUNT(*) OVER() AS total
FROM request
WHERE status = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3`

	var (
		pgxRows pgx.Rows
		err     error
	)
	if status == "" {
		pgxRows, err = s.pool.Query(ctx, allQ, limit, offset)
	} else {
		pgxRows, err = s.pool.Query(ctx, filteredQ, status, limit, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("store.ListRequestsForAdmin query: %w", err)
	}
	defer pgxRows.Close()

	return scanRequestsWithTotal(pgxRows)
}

// scanRequests drains a pgx.Rows cursor into a slice of *Request.
func scanRequests(rows pgx.Rows) ([]*Request, error) {
	var result []*Request
	for rows.Next() {
		r := &Request{}
		var errMsg *string
		if err := rows.Scan(
			&r.ID, &r.TMDBID, &r.MediaType, &r.Title, &r.Year, &r.PosterURL,
			&r.RequesterUserID, &r.RequesterIsAdmin, &r.Status,
			&r.RoutedArrID, &r.ExternalID, &errMsg, &r.MatchTrace,
			&r.SubmittedAt, &r.LastPolledAt, &r.CompletedAt,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("store.scanRequests: %w", err)
		}
		if errMsg != nil {
			r.Error = *errMsg
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store.scanRequests: %w", err)
	}
	return result, nil
}

// scanRequestsWithTotal drains a pgx.Rows cursor that includes a trailing
// COUNT(*) OVER() column into a slice of *Request plus the total row count.
// When there are no rows the function returns (nil, 0, nil).
func scanRequestsWithTotal(rows pgx.Rows) ([]*Request, int, error) {
	var (
		result []*Request
		total  int
	)
	for rows.Next() {
		r := &Request{}
		var errMsg *string
		if err := rows.Scan(
			&r.ID, &r.TMDBID, &r.MediaType, &r.Title, &r.Year, &r.PosterURL,
			&r.RequesterUserID, &r.RequesterIsAdmin, &r.Status,
			&r.RoutedArrID, &r.ExternalID, &errMsg, &r.MatchTrace,
			&r.SubmittedAt, &r.LastPolledAt, &r.CompletedAt,
			&r.CreatedAt, &r.UpdatedAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("store.scanRequestsWithTotal: %w", err)
		}
		if errMsg != nil {
			r.Error = *errMsg
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store.scanRequestsWithTotal: %w", err)
	}
	return result, total, nil
}
