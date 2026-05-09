package store_test

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/migrate"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

// testDSN returns the DSN to use for integration tests. Honors
// TEST_DATABASE_URL if set, otherwise defaults to the local
// continuum-postgres container.
func testDSN() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://continuum:continuum@localhost:5432/continuum?search_path=arrouter_test&sslmode=disable"
}

// newTestStore opens a pool against the test DSN, runs migrations into the
// `arrouter_test` schema (creating it fresh each test run), and returns the
// store. Skips the test if Postgres is unreachable.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := testDSN()
	ctx := context.Background()

	// Open a no-search_path admin pool first to (re)create the schema.
	adminDSN := stripSearchPath(dsn)
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Skipf("postgres unreachable (%v); skipping integration test", err)
	}

	// Ping to verify the connection is actually usable before proceeding.
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unreachable (%v); skipping integration test", err)
	}
	defer admin.Close()

	if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS arrouter_test CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA arrouter_test"); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	if err := migrate.Run(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return store.New(pool)
}

// stripSearchPath removes the search_path query parameter from a DSN so the
// admin pool connects with the default search_path (public).
func stripSearchPath(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		// If we can't parse it, return as-is and let the caller handle it.
		return dsn
	}
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String()
}
