package store_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/migrate"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

var (
	testSchemaOnce sync.Once
	testSchema     string
)

// schemaName returns a per-process schema name so that parallel test binaries
// (one per package) each get their own schema and never collide.
func schemaName() string {
	testSchemaOnce.Do(func() {
		testSchema = fmt.Sprintf("arrouter_test_%d", os.Getpid())
	})
	return testSchema
}

// testDSN returns the DSN to use for integration tests. Honors
// TEST_DATABASE_URL if set (substituting the schema name into search_path),
// otherwise defaults to the local continuum-postgres container.
func testDSN() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		u, err := url.Parse(v)
		if err == nil {
			q := u.Query()
			q.Set("search_path", schemaName())
			u.RawQuery = q.Encode()
			return u.String()
		}
		return v
	}
	return fmt.Sprintf(
		"postgres://continuum:continuum@localhost:5432/continuum?search_path=%s&sslmode=disable",
		schemaName(),
	)
}

// newTestStore opens a pool against the test DSN, runs migrations into a
// per-process schema (creating it fresh each test run), and returns the store.
// Skips the test if Postgres is unreachable.
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

	dropStmt := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName())
	createStmt := fmt.Sprintf("CREATE SCHEMA %s", schemaName())

	if _, err := admin.Exec(ctx, dropStmt); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := admin.Exec(ctx, createStmt); err != nil {
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

// cleanupSchema drops the per-process schema as a best-effort post-run cleanup
// so stale schemas don't accumulate in the database.
func cleanupSchema() {
	ctx := context.Background()
	adminDSN := stripSearchPath(testDSN())
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		return
	}
	defer admin.Close()
	_, _ = admin.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName()))
}

// TestMain ensures the per-process schema is removed after all tests in this
// binary finish, regardless of pass/fail.
func TestMain(m *testing.M) {
	code := m.Run()
	cleanupSchema()
	os.Exit(code)
}
