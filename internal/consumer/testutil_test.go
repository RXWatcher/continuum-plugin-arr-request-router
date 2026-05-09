package consumer_test

// Source of truth for this helper is internal/store/testutil_test.go.
// Duplicated here (option b) to avoid rearranging shared code mid-implementation.

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/migrate"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

func testDSN() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://continuum:continuum@localhost:5432/continuum?search_path=arrouter_test&sslmode=disable"
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := testDSN()
	ctx := context.Background()

	adminDSN := stripSearchPath(dsn)
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Skipf("postgres unreachable (%v); skipping integration test", err)
	}

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

func stripSearchPath(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String()
}
