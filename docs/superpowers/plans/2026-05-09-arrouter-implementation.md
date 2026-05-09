# continuum.arrouter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `continuum.arrouter` plugin from scratch — a rule-based multi-*arr router that consumes `continuum.requests` events, picks the highest-priority Radarr/Sonarr instance whose admin-curated rules match (with on-demand TMDB enrichment), forwards the request, polls for status, and publishes status events. Plus a small coordinated update to `continuum-plugin-requests` so it consumes arrouter status events.

**Architecture:** Mirrors `continuum-plugin-arrproxy` deliberately (same store/consumer/poll/event/runtime/server packages). New surface area is a `routing/` package containing the rule engine, a `tmdb/` package with three independent TTL caches, a `crypto/` package for encrypting registered-*arr API keys at rest, and a `registered_arr` table feeding the routing decision. Frontend is a Vite/React/Tailwind SPA with continuum's `CollectionBuilder` ported over.

**Tech Stack:** Go 1.26, pgx/v5, golang-migrate (embedded SQL), hashicorp/go-plugin via continuum-plugin-sdk, chi router, hclog. Frontend: Vite + React + TypeScript + Tailwind, mirroring `continuum-plugin-requests/web/`.

**Spec:** `/opt/continuum-plugin-arrouter/SPEC.md`. Read it before starting — every task here references it.

**Reference repo:** `/opt/continuum-plugin-arrproxy` is the architectural twin. Many files are near-verbatim copies; the plan calls these out explicitly.

---

## Repo boundary

- **Repo A:** `/opt/continuum-plugin-arrouter` — Phases 0–11 (the plugin itself).
- **Repo B:** `/opt/continuum-plugin-requests` — Phase 12 only (extend subscriptions, rename handlers).

Each phase ends with a clean working state. Don't merge phases. Keep commits inside the phase they belong to.

---

## File structure (Repo A)

```
continuum-plugin-arrouter/
├── Makefile
├── README.md
├── go.mod
├── cmd/continuum-plugin-arrouter/
│   ├── main.go
│   └── manifest.json                  # //go:embed-ed
├── internal/
│   ├── arr/                           # Radarr/Sonarr v3 clients (mostly ported from arrproxy)
│   │   ├── common.go
│   │   ├── profiles.go
│   │   ├── radarr.go
│   │   └── sonarr.go
│   ├── auth/identity.go               # admin role guard (ported)
│   ├── consumer/
│   │   ├── consumer.go                # routes events → handlers
│   │   ├── submit.go                  # submitted handler
│   │   ├── cancel.go                  # cancelled handler
│   │   └── consumer_test.go
│   ├── crypto/
│   │   ├── secret.go                  # AES-GCM round-trip for api_key
│   │   └── secret_test.go
│   ├── event/publisher.go             # plugin.continuum.arrouter.* events
│   ├── httproutes/server.go           # http_routes.v1 capability glue
│   ├── migrate/
│   │   ├── runner.go                  # ported
│   │   └── files/0001_init.{up,down}.sql
│   ├── poll/
│   │   ├── poll.go                    # per-row poll
│   │   ├── scheduled.go               # scheduled_task.v1 binding
│   │   └── poll_test.go
│   ├── routing/
│   │   ├── rules.go                   # JSON shape, validation
│   │   ├── rules_test.go
│   │   ├── operators.go               # eq, ne, gt, …, regex
│   │   ├── operators_test.go
│   │   ├── fields.go                  # Group A/B/C accessors
│   │   ├── fields_test.go
│   │   ├── evaluator.go               # group/rule evaluator + match_trace
│   │   ├── evaluator_test.go
│   │   ├── router.go                  # picks first matching arr
│   │   ├── router_test.go
│   │   └── trace.go                   # match_trace types/marshalling
│   ├── runtime/runtime.go             # Config struct + bootstrap
│   ├── server/
│   │   ├── server.go
│   │   ├── prerender_handler.go       # theme injection
│   │   ├── registry_handlers.go
│   │   ├── requests_handlers.go
│   │   ├── route_test_handler.go
│   │   └── handlers_test.go
│   ├── store/
│   │   ├── store.go
│   │   ├── registry.go
│   │   ├── registry_test.go
│   │   ├── request.go
│   │   └── request_test.go
│   └── tmdb/
│       ├── client.go                  # primary + keywords + ratings
│       ├── cache.go                   # 3 sync.Maps, TTL
│       └── client_test.go
└── web/                               # Vite SPA, mirrors continuum-plugin-requests/web/
    ├── embed.go                       # //go:embed dist/* + handler
    ├── package.json
    ├── tsconfig.json
    ├── vite.config.ts
    ├── tailwind.config.ts
    ├── postcss.config.js
    ├── index.html
    └── src/
        ├── main.tsx
        ├── App.tsx
        ├── index.css
        ├── api/{client.ts,types.ts}
        ├── components/
        │   ├── CollectionBuilder.tsx        # ported from continuum
        │   ├── CollectionBuilder.test.tsx
        │   ├── RegistryTable.tsx
        │   ├── RegistryEditor.tsx
        │   ├── ConnectionPanel.tsx
        │   ├── RuleTestPanel.tsx
        │   ├── RequestsQueue.tsx
        │   └── StatusPill.tsx
        └── pages/
            ├── RegistryListPage.tsx
            ├── RegistryEditorPage.tsx
            └── RequestsQueuePage.tsx
```

---

## Phase 0 — Scaffold

### Task 0.1: Bootstrap repo

**Files:**
- Create: `Makefile`, `README.md`, `go.mod`, `.gitignore`
- Create: `cmd/continuum-plugin-arrouter/main.go` (stub)
- Create: `cmd/continuum-plugin-arrouter/manifest.json`

- [ ] **Step 1: Initialize go.mod**

```bash
cd /opt/continuum-plugin-arrouter
go mod init github.com/ContinuumApp/continuum-plugin-arrouter
```

- [ ] **Step 2: Add the SDK replace directive**

Edit `go.mod`, append the same replace line `arrproxy` uses (verify the worktree path exists first):

```
replace github.com/ContinuumApp/continuum-plugin-sdk => /opt/worktrees/continuum-plugin-sdk-rh
```

Add the SDK as a require:

```bash
go get github.com/ContinuumApp/continuum-plugin-sdk@v0.0.0-00010101000000-000000000000
```

- [ ] **Step 3: Create Makefile**

```makefile
BINARY := continuum-plugin-arrouter
GO ?= go

.PHONY: build test clean

build:
	$(GO) build -o $(BINARY) ./cmd/continuum-plugin-arrouter

test:
	$(GO) test ./...

clean:
	rm -f $(BINARY)
```

- [ ] **Step 4: Create stub main.go**

```go
// cmd/continuum-plugin-arrouter/main.go
package main

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed manifest.json
var manifestRaw []byte

func main() {
	fmt.Fprintln(os.Stderr, "continuum-plugin-arrouter: stub")
	os.Exit(0)
}
```

- [ ] **Step 5: Create manifest.json**

```json
{
  "plugin_id": "continuum.arrouter",
  "version": "0.1.0",
  "checksum": "__CHECKSUM__",
  "continuum_api_version": "v1",
  "capabilities": [
    {
      "type": "event_consumer.v1",
      "id": "router",
      "display_name": "Router",
      "description": "Routes request events to a registered Radarr/Sonarr based on rules.",
      "subscriptions": [
        "plugin.continuum.requests.submitted",
        "plugin.continuum.requests.cancelled"
      ]
    },
    {
      "type": "scheduled_task.v1",
      "id": "poll",
      "display_name": "Poll *arrs",
      "description": "Polls registered *arrs for download/import progress."
    },
    {
      "type": "http_routes.v1",
      "id": "admin",
      "display_name": "Arrouter Admin",
      "description": "Admin SPA for the arrouter plugin."
    }
  ],
  "http_routes": [
    {"id": "api",    "method": "*",   "path": "/api/admin/*", "access": "authenticated"},
    {"id": "assets", "method": "GET", "path": "/assets/*",    "access": "public"},
    {"id": "spa",    "method": "GET", "path": "/admin/*",     "access": "authenticated", "navigable": true, "navigation_label": "Arrouter", "navigation_kind": "admin"}
  ],
  "global_config_schema": []
}
```

(Global config schema entries are appended in Phase 11. Stubbed empty for now.)

- [ ] **Step 6: Build to verify scaffold compiles**

Run: `make build`
Expected: produces `./continuum-plugin-arrouter` binary, exits 0.

- [ ] **Step 7: Commit**

```bash
git add Makefile go.mod go.sum cmd/ README.md .gitignore
git commit -m "chore: scaffold continuum-plugin-arrouter"
```

---

## Phase 1 — Storage layer

### Task 1.1: Migration runner (port from arrproxy)

**Files:**
- Create: `internal/migrate/runner.go` (verbatim copy from arrproxy with package path swap)
- Create: `internal/migrate/files/0001_init.up.sql`
- Create: `internal/migrate/files/0001_init.down.sql`

- [ ] **Step 1: Copy migrate runner**

```bash
cp /opt/continuum-plugin-arrproxy/internal/migrate/runner.go internal/migrate/runner.go
```

Search/replace `arrproxy` → `arrouter` in the comment header and any error strings. Leave the code intact — it's already generic.

- [ ] **Step 2: Write `0001_init.up.sql`**

Match the spec's "DB" section verbatim:

```sql
CREATE TABLE registered_arr (
  id                  BIGSERIAL PRIMARY KEY,
  name                TEXT NOT NULL,
  kind                TEXT NOT NULL CHECK (kind IN ('radarr','sonarr')),
  url                 TEXT NOT NULL,
  api_key             TEXT NOT NULL,
  root_folder_path    TEXT NOT NULL,
  quality_profile_id  INTEGER,
  language_profile_id INTEGER,
  priority            INTEGER NOT NULL DEFAULT 100,
  enabled             BOOLEAN NOT NULL DEFAULT true,
  rules_json          JSONB NOT NULL DEFAULT '{"match":"all","groups":[]}'::jsonb,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX registered_arr_kind_priority_idx
  ON registered_arr (kind, priority) WHERE enabled;

CREATE TABLE request (
  id                  TEXT PRIMARY KEY,
  tmdb_id             INTEGER NOT NULL,
  media_type          TEXT NOT NULL CHECK (media_type IN ('movie','tv')),
  title               TEXT NOT NULL,
  year                INTEGER NOT NULL DEFAULT 0,
  poster_url          TEXT,
  requester_user_id   TEXT NOT NULL,
  requester_is_admin  BOOLEAN NOT NULL DEFAULT false,
  status              TEXT NOT NULL CHECK (status IN
    ('queued','submitted','downloading','imported','failed','cancelled','unrouted')),
  routed_arr_id       BIGINT REFERENCES registered_arr(id) ON DELETE SET NULL,
  external_id         INTEGER,
  error               TEXT,
  match_trace         JSONB,
  submitted_at        TIMESTAMPTZ,
  last_polled_at      TIMESTAMPTZ,
  completed_at        TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX request_status_idx ON request (status)
  WHERE status IN ('submitted','downloading');
CREATE INDEX request_tmdb_idx ON request (tmdb_id, media_type);
CREATE INDEX request_routed_arr_idx ON request (routed_arr_id)
  WHERE status IN ('submitted','downloading');
```

(Schema-qualified names omitted — `search_path=arrouter` is part of the DSN.)

- [ ] **Step 3: Write `0001_init.down.sql`**

```sql
DROP TABLE IF EXISTS request;
DROP TABLE IF EXISTS registered_arr;
```

- [ ] **Step 4: Build to verify embedding works**

Run: `go build ./internal/migrate/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/migrate
git commit -m "feat(migrate): add 0001_init for registered_arr + request"
```

### Task 1.2: Store skeleton

**Files:**
- Create: `internal/store/store.go`

- [ ] **Step 1: Write store skeleton**

```go
package store

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }
```

- [ ] **Step 2: Build**

Run: `go build ./internal/store/...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store.go
git commit -m "feat(store): add Store skeleton"
```

### Task 1.3: Registry repository — write failing tests

**Files:**
- Create: `internal/store/registry.go` (empty stubs)
- Create: `internal/store/registry_test.go`

- [ ] **Step 1: Write empty stubs in `registry.go`**

```go
package store

import (
	"context"
	"time"
)

type RegisteredArr struct {
	ID                int64
	Name              string
	Kind              string // "radarr"|"sonarr"
	URL               string
	APIKey            string // encrypted
	RootFolderPath    string
	QualityProfileID  *int
	LanguageProfileID *int
	Priority          int
	Enabled           bool
	RulesJSON         []byte
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (s *Store) CreateArr(ctx context.Context, a *RegisteredArr) (int64, error)            { return 0, nil }
func (s *Store) GetArr(ctx context.Context, id int64) (*RegisteredArr, error)              { return nil, nil }
func (s *Store) ListArrs(ctx context.Context) ([]*RegisteredArr, error)                    { return nil, nil }
func (s *Store) ListEnabledArrsByKind(ctx context.Context, kind string) ([]*RegisteredArr, error) { return nil, nil }
func (s *Store) UpdateArr(ctx context.Context, a *RegisteredArr) error                     { return nil }
func (s *Store) DeleteArr(ctx context.Context, id int64) error                             { return nil }
```

- [ ] **Step 2: Write integration tests against a real Postgres**

`internal/store/registry_test.go` uses a test helper `newTestStore(t)` that opens a connection to the URL in env `TEST_DATABASE_URL`, runs migrations, and `t.Cleanup`s a `TRUNCATE`. (See arrproxy `internal/store/request_test.go` for the helper pattern — port it.)

```go
package store_test

// minimum coverage:
// - Create then Get returns same row
// - List returns rows in (kind, priority, id) order
// - ListEnabledArrsByKind respects enabled and kind filters; orders by priority ASC, id ASC
// - Update changes fields; updated_at advances
// - Delete removes the row; subsequent Get returns nil, nil (not an error)
// - rules_json round-trips as JSONB
```

Write one test function per bullet.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `TEST_DATABASE_URL=postgres://… go test ./internal/store/ -run Registry -v`
Expected: every test fails (stubs return zero values).

- [ ] **Step 4: Implement the methods**

Each method is a single `s.pool.QueryRow` / `Exec` / `Query` against `registered_arr`. Use `pgx`'s `pgx.RowToStructByName` or manual `Scan`. Match the pattern in `/opt/continuum-plugin-arrproxy/internal/store/request.go`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: same as Step 3.
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/registry.go internal/store/registry_test.go
git commit -m "feat(store): registry CRUD"
```

### Task 1.4: Request repository

**Files:**
- Create: `internal/store/request.go`
- Create: `internal/store/request_test.go`

Mirror `arrproxy/internal/store/request.go` but with these additions:
- `Status` enum gains `"unrouted"`.
- New columns: `routed_arr_id *int64`, `match_trace []byte`.

- [ ] **Step 1: Write stub `request.go`**

```go
package store

import (
	"context"
	"time"
)

type Request struct {
	ID                string
	TMDBID            int
	MediaType         string
	Title             string
	Year              int
	PosterURL         string
	RequesterUserID   string
	RequesterIsAdmin  bool
	Status            string
	RoutedArrID       *int64
	ExternalID        *int
	Error             string
	MatchTrace        []byte
	SubmittedAt       *time.Time
	LastPolledAt      *time.Time
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (s *Store) UpsertRequestQueued(ctx context.Context, r *Request) error                                      { return nil }
func (s *Store) GetRequest(ctx context.Context, id string) (*Request, error)                                    { return nil, nil }
func (s *Store) MarkSubmitted(ctx context.Context, id string, externalID int) error                             { return nil }
func (s *Store) MarkDownloading(ctx context.Context, id string) (transitioned bool, err error)                  { return false, nil }
func (s *Store) MarkImported(ctx context.Context, id string) error                                              { return nil }
func (s *Store) MarkFailed(ctx context.Context, id string, msg string) error                                    { return nil }
func (s *Store) MarkCancelled(ctx context.Context, id string) error                                             { return nil }
func (s *Store) MarkUnrouted(ctx context.Context, id string, trace []byte, reason string) error                 { return nil }
func (s *Store) SetRoutedArr(ctx context.Context, id string, arrID int64, trace []byte) error                   { return nil }
func (s *Store) ListPollable(ctx context.Context) ([]*Request, error)                                           { return nil, nil }
func (s *Store) UpdateLastPolled(ctx context.Context, id string, t time.Time) error                             { return nil }
func (s *Store) ListForAdmin(ctx context.Context, status string, limit, offset int) ([]*Request, int, error)    { return nil, 0, nil }
```

- [ ] **Step 2: Write the test file**

```go
package store_test

// coverage:
// - UpsertRequestQueued inserts; second call with same id is a no-op (do not overwrite later state)
// - SetRoutedArr stores routed_arr_id + match_trace
// - MarkSubmitted: status submitted, external_id set, submitted_at = now
// - MarkDownloading transitions submitted → downloading and returns true once; subsequent calls return false
// - MarkImported / MarkFailed / MarkCancelled / MarkUnrouted set terminal status + completed_at
// - ListPollable returns rows IN ('submitted','downloading') and skips others
// - UpdateLastPolled sets last_polled_at without changing status
// - ListForAdmin paginates and filters by status
```

- [ ] **Step 3: Run tests to verify failure**

Run: `go test ./internal/store/ -run Request -v`
Expected: all fail.

- [ ] **Step 4: Implement methods**

Each method maps to one SQL statement. Use `INSERT ... ON CONFLICT (id) DO NOTHING` for `UpsertRequestQueued`.

- [ ] **Step 5: Run tests to verify pass**

- [ ] **Step 6: Commit**

```bash
git add internal/store/request.go internal/store/request_test.go
git commit -m "feat(store): request CRUD with unrouted + routed_arr_id"
```

---

## Phase 2 — Secret encryption

### Task 2.1: AES-GCM round-trip

**Files:**
- Create: `internal/crypto/secret.go`
- Create: `internal/crypto/secret_test.go`

- [ ] **Step 1: Write the failing test**

```go
package crypto_test

import (
	"strings"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/crypto"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key := strings.Repeat("k", 32)
	plain := "super-secret-api-key"
	sealed, err := crypto.Seal(key, plain)
	if err != nil { t.Fatal(err) }
	if sealed == plain { t.Fatal("seal returned plaintext") }
	got, err := crypto.Open(key, sealed)
	if err != nil { t.Fatal(err) }
	if got != plain { t.Fatalf("got %q want %q", got, plain) }
}

func TestSealUsesFreshNonce(t *testing.T) {
	key := strings.Repeat("k", 32)
	a, _ := crypto.Seal(key, "x")
	b, _ := crypto.Seal(key, "x")
	if a == b { t.Fatal("seal must use fresh nonce per call") }
}

func TestOpenWrongKey(t *testing.T) {
	a, _ := crypto.Seal(strings.Repeat("k", 32), "x")
	_, err := crypto.Open(strings.Repeat("z", 32), a)
	if err == nil { t.Fatal("expected error from wrong key") }
}

func TestOpenTamperedCiphertextFails(t *testing.T) {
	key := strings.Repeat("k", 32)
	sealed, _ := crypto.Seal(key, "x")
	tampered := sealed[:len(sealed)-2] + "AA"
	_, err := crypto.Open(key, tampered)
	if err == nil { t.Fatal("expected error from tampered ciphertext") }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/crypto/ -v`
Expected: build error (package doesn't exist yet).

- [ ] **Step 3: Implement `secret.go`**

```go
// Package crypto provides authenticated symmetric encryption for plugin
// secrets persisted in the database (currently registered_arr.api_key).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// Seal returns base64(nonce || ciphertext || tag) using AES-256-GCM.
// `key` is any UTF-8 string; it is hashed with SHA-256 to produce the
// 32-byte AES key.
func Seal(key, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil { return "", err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Open is the inverse of Seal. Returns an error if the key is wrong, the
// ciphertext is malformed, or the auth tag fails.
func Open(key, sealed string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil { return "", err }
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil { return "", err }
	if len(raw) < gcm.NonceSize() { return "", errors.New("crypto: short ciphertext") }
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil { return "", err }
	return string(pt), nil
}

func newGCM(key string) (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil { return nil, err }
	return cipher.NewGCM(block)
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/crypto/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/crypto
git commit -m "feat(crypto): AES-GCM seal/open for stored secrets"
```

---

## Phase 3 — Rule engine

The rule engine is pure logic with no I/O — easy to TDD comprehensively. We build it bottom-up: rule types → operators → field accessors → evaluator → router.

### Task 3.1: Rules JSON shape and validation

**Files:**
- Create: `internal/routing/rules.go`
- Create: `internal/routing/rules_test.go`

- [ ] **Step 1: Write the failing test**

```go
package routing_test

import (
	"encoding/json"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

func TestParseRulesValid(t *testing.T) {
	raw := []byte(`{"match":"all","groups":[{"match":"any","rules":[{"field":"year","op":"gte","value":2000}]}]}`)
	r, err := routing.ParseRules(raw)
	if err != nil { t.Fatal(err) }
	if r.Match != "all" || len(r.Groups) != 1 { t.Fatalf("unexpected: %+v", r) }
}

func TestParseRulesEmptyDefaultsToMatchAll(t *testing.T) {
	r, err := routing.ParseRules([]byte(`{}`))
	if err != nil { t.Fatal(err) }
	if r.Match != "all" || len(r.Groups) != 0 { t.Fatalf("unexpected: %+v", r) }
}

func TestValidateRulesRejectsUnknownField(t *testing.T) {
	r := routing.Rules{Match:"all", Groups:[]routing.Group{{Match:"all", Rules:[]routing.Rule{{Field:"banana", Op:"eq", Value: json.RawMessage(`"x"`)}}}}}
	if err := routing.ValidateRules(r); err == nil { t.Fatal("expected error") }
}

func TestValidateRulesRejectsUnknownOp(t *testing.T) { /* … */ }
func TestValidateRulesRejectsBadCombinator(t *testing.T) { /* … */ }
func TestValidateRulesAcceptsEmptyGroups(t *testing.T) { /* the catch-all case */ }
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/routing/ -run Rules -v`
Expected: build error (package missing).

- [ ] **Step 3: Implement `rules.go`**

```go
package routing

import (
	"encoding/json"
	"fmt"
)

type Combinator string // "all" | "any"

type Rules struct {
	Match  Combinator `json:"match"`
	Groups []Group    `json:"groups"`
}

type Group struct {
	Match Combinator `json:"match"`
	Rules []Rule     `json:"rules"`
}

type Rule struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

func ParseRules(raw []byte) (Rules, error) {
	var r Rules
	if len(raw) == 0 { return Rules{Match: "all"}, nil }
	if err := json.Unmarshal(raw, &r); err != nil { return Rules{}, err }
	if r.Match == "" { r.Match = "all" }
	for i := range r.Groups {
		if r.Groups[i].Match == "" { r.Groups[i].Match = "all" }
	}
	return r, nil
}

func ValidateRules(r Rules) error {
	if r.Match != "all" && r.Match != "any" {
		return fmt.Errorf("invalid top-level match: %q", r.Match)
	}
	for gi, g := range r.Groups {
		if g.Match != "all" && g.Match != "any" {
			return fmt.Errorf("group %d: invalid match: %q", gi, g.Match)
		}
		for ri, ru := range g.Rules {
			if !KnownField(ru.Field) {
				return fmt.Errorf("group %d rule %d: unknown field %q", gi, ri, ru.Field)
			}
			if !KnownOp(ru.Op) {
				return fmt.Errorf("group %d rule %d: unknown op %q", gi, ri, ru.Op)
			}
		}
	}
	return nil
}
```

`KnownField` and `KnownOp` are stubs returning `true` for now — they get filled in by Tasks 3.2 and 3.4/3.5. Add them as `var KnownField = func(string) bool { return true }` so later tasks can replace.

- [ ] **Step 4: Run tests to verify pass** (or skip the unknown-field/op tests until 3.2 if needed)

- [ ] **Step 5: Commit**

```bash
git add internal/routing/rules.go internal/routing/rules_test.go
git commit -m "feat(routing): rules types + validation skeleton"
```

### Task 3.2: Operators

**Files:**
- Create: `internal/routing/operators.go`
- Create: `internal/routing/operators_test.go`

- [ ] **Step 1: Write a table-driven failing test**

```go
package routing_test

import (
	"encoding/json"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

func TestOperators(t *testing.T) {
	cases := []struct {
		name     string
		op       string
		actual   any
		value    string // JSON
		want     bool
	}{
		{"eq string ci", "eq", "Animation", `"animation"`, true},
		{"eq mismatch",   "eq", "Animation", `"comedy"`,    false},
		{"ne true",       "ne", "Animation", `"comedy"`,    true},
		{"in match",      "in", "ja", `["en","ja","ko"]`, true},
		{"in miss",       "in", "fr", `["en","ja","ko"]`, false},
		{"not_in match",  "not_in", "fr", `["en","ja","ko"]`, true},
		{"gt int",        "gt", 2005, `2000`, true},
		{"gt fail",       "gt", 1999, `2000`, false},
		{"between",       "between", 2005, `[2000,2010]`, true},
		{"between out",   "between", 1999, `[2000,2010]`, false},
		{"contains substr", "contains", "Sci-Fi & Fantasy", `"fantasy"`, true},
		{"contains array", "contains", []string{"Animation","Action"}, `"animation"`, true},
		{"contains miss",  "contains", []string{"Drama"}, `"animation"`, false},
		{"starts_with",    "starts_with", "The Matrix", `"the "`, true},
		{"regex",          "regex", "abc-123", `"^abc-\\d+$"`, true},
		{"regex bad",      "regex", "abc-XYZ", `"^abc-\\d+$"`, false},
		{"regex invalid",  "regex", "abc", `"["`, false}, // invalid regex → false
		{"type mismatch",  "gt", "string", `5`, false},
		{"missing actual", "eq", nil, `"x"`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := routing.Apply(c.op, c.actual, json.RawMessage(c.value))
			if got != c.want { t.Fatalf("got %v want %v", got, c.want) }
		})
	}
}

func TestKnownOp(t *testing.T) {
	for _, op := range []string{"eq","ne","in","not_in","gt","gte","lt","lte","between","contains","starts_with","regex"} {
		if !routing.KnownOp(op) { t.Errorf("KnownOp(%q) = false", op) }
	}
	if routing.KnownOp("bogus") { t.Error("KnownOp(bogus) = true") }
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `operators.go`**

```go
package routing

import (
	"encoding/json"
	"regexp"
	"strings"
)

func KnownOp(op string) bool {
	switch op {
	case "eq","ne","in","not_in","gt","gte","lt","lte","between","contains","starts_with","regex":
		return true
	}
	return false
}

// Apply returns (matched, traceNote). traceNote is non-empty when the rule
// fell back to false because of a type or input issue (e.g. invalid regex,
// type mismatch) — the evaluator surfaces it in match_trace.
func Apply(op string, actual any, raw json.RawMessage) (bool, string) {
	switch op {
	case "eq":          return cmpEq(actual, raw)
	case "ne":          b, n := cmpEq(actual, raw); return !b && n == "", n
	case "in":          return cmpIn(actual, raw, false)
	case "not_in":      b, n := cmpIn(actual, raw, false); return !b && n == "", n
	case "gt","gte","lt","lte":
		return cmpNumeric(op, actual, raw)
	case "between":     return cmpBetween(actual, raw)
	case "contains":    return cmpContains(actual, raw)
	case "starts_with": return cmpStartsWith(actual, raw)
	case "regex":       return cmpRegex(actual, raw)
	}
	return false, "unknown op"
}

// cmpEq, cmpIn, cmpNumeric, cmpBetween, cmpContains, cmpStartsWith, cmpRegex
// each handle the type-coercion details. String compare is case-insensitive
// (use strings.EqualFold and strings.ToLower for substring/prefix). Numeric
// compare accepts int, int64, float64, json.Number; any non-numeric actual
// returns (false, "type mismatch").
//
// cmpRegex compiles RE2 fresh per call; an invalid pattern returns
// (false, "invalid regex: <err>") so it surfaces in the trace.
```

Implement each helper. Keep them small and readable.

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Wire `KnownOp` into `rules.go`** — replace the stub `KnownOp` reference (Task 3.1) with the real one. (If you used `var KnownOp = func(...) bool {...}` in Task 3.1, swap it for a direct call.)

- [ ] **Step 6: Commit**

```bash
git add internal/routing
git commit -m "feat(routing): operators with case-insensitive + safe regex"
```

### Task 3.3: Group A field accessors

**Files:**
- Create: `internal/routing/fields.go` (initial — Group A only)
- Create: `internal/routing/fields_test.go`

The "context" the evaluator queries is a struct holding the request event payload + lazily-resolved enrichment payloads. Define it here.

- [ ] **Step 1: Write the failing test**

```go
package routing_test

import (
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

func TestGroupAFields(t *testing.T) {
	ctx := routing.Context{
		Event: routing.RequestEvent{
			MediaType: "movie", LibraryID: "lib1",
			Year: 2003, RequesterUserID: "u1", RequesterIsAdmin: false,
			Title: "Lost in Translation", TMDBID: 153,
		},
	}
	cases := map[string]any{
		"mediaType":        "movie",
		"libraryId":        "lib1",
		"year":             2003,
		"decade":           2000,
		"requesterUserId":  "u1",
		"requesterIsAdmin": false,
		"title":            "Lost in Translation",
		"tmdbId":           153,
	}
	for f, want := range cases {
		got, ok := routing.GetField(ctx, f)
		if !ok { t.Errorf("%s: missing", f); continue }
		if got != want { t.Errorf("%s: got %v want %v", f, got, want) }
	}
}

func TestGroupAUnknownField(t *testing.T) {
	_, ok := routing.GetField(routing.Context{}, "banana")
	if ok { t.Error("expected unknown field to return ok=false") }
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement Group A in `fields.go`**

```go
package routing

type RequestEvent struct {
	RequestID        string
	MediaType        string // "movie"|"tv"
	LibraryID        string
	Year             int
	RequesterUserID  string
	RequesterIsAdmin bool
	Title            string
	TMDBID           int
	PosterURL        string
}

type Context struct {
	Event       RequestEvent
	Primary     *TMDBPrimary  // populated lazily
	Keywords    []string      // populated lazily
	ContentRating string      // populated lazily; empty if absent
}

// TMDBPrimary placeholder — concrete shape filled in Task 3.4.
type TMDBPrimary struct{}

// FieldGroup classifies a field by where its data comes from.
type FieldGroup int
const (
	GroupA FieldGroup = iota // request event
	GroupB                   // primary TMDB call
	GroupCKeywords
	GroupCContentRating
)

func KnownField(name string) bool {
	_, ok := fieldGroups[name]
	return ok
}

func FieldGroupOf(name string) (FieldGroup, bool) {
	g, ok := fieldGroups[name]
	return g, ok
}

var fieldGroups = map[string]FieldGroup{
	// Group A
	"mediaType":        GroupA,
	"libraryId":        GroupA,
	"year":             GroupA,
	"decade":           GroupA,
	"requesterUserId":  GroupA,
	"requesterIsAdmin": GroupA,
	"title":            GroupA,
	"tmdbId":           GroupA,
}

// GetField returns (value, true) if the field is known and the data for its
// group is loaded. Returns (nil, false) if the field is missing — the
// evaluator treats missing as "rule false".
func GetField(ctx Context, name string) (any, bool) {
	switch name {
	case "mediaType":        return ctx.Event.MediaType, true
	case "libraryId":        return ctx.Event.LibraryID, true
	case "year":             return ctx.Event.Year, true
	case "decade":           return ctx.Event.Year - (ctx.Event.Year % 10), true
	case "requesterUserId":  return ctx.Event.RequesterUserID, true
	case "requesterIsAdmin": return ctx.Event.RequesterIsAdmin, true
	case "title":            return ctx.Event.Title, true
	case "tmdbId":           return ctx.Event.TMDBID, true
	}
	return nil, false
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/routing/fields.go internal/routing/fields_test.go
git commit -m "feat(routing): group A field accessors"
```

### Task 3.4: Group B field accessors (primary TMDB)

**Files:**
- Modify: `internal/routing/fields.go`
- Modify: `internal/routing/fields_test.go`

- [ ] **Step 1: Write the failing test (extend existing test file)**

```go
func TestGroupBFields(t *testing.T) {
	primary := &routing.TMDBPrimary{
		MediaType:           "movie",
		OriginalLanguage:    "ja",
		OriginalTitle:       "千と千尋の神隠し",
		Genres:              []string{"Animation","Family"},
		Runtime:             125,
		VoteAverage:         8.5,
		VoteCount:           14000,
		Popularity:          88.4,
		Adult:               false,
		Status:              "Released",
		ProductionCompanies: []string{"Studio Ghibli"},
		ProductionCountries: []string{"JP"},
		SpokenLanguages:     []string{"ja"},
		ReleaseDate:         "2001-07-20",
		Budget:              19000000,
		Revenue:             395580000,
		BelongsToCollection: "",
		IMDBID:              "tt0245429",
	}
	ctx := routing.Context{
		Event: routing.RequestEvent{MediaType:"movie", Year: 2001},
		Primary: primary,
	}
	expect := map[string]any{
		"original_language":    "ja",
		"genres":               []string{"Animation","Family"},
		"runtime":              125,
		"vote_average":         8.5,
		"production_countries": []string{"JP"},
		"release_date":         "2001-07-20",
		"belongs_to_collection": "", // present, just empty
	}
	for f, want := range expect {
		got, ok := routing.GetField(ctx, f)
		if !ok { t.Errorf("%s: missing", f); continue }
		// deep-equal slices
		_ = got; _ = want // sketch — use reflect.DeepEqual
	}
}

func TestGroupBMissingWhenPrimaryUnloaded(t *testing.T) {
	ctx := routing.Context{Event: routing.RequestEvent{MediaType:"movie"}}
	if _, ok := routing.GetField(ctx, "original_language"); ok {
		t.Error("Group B field should be missing when Primary is nil")
	}
}

func TestTVOnlyOnMovieKindMissing(t *testing.T) {
	ctx := routing.Context{
		Event: routing.RequestEvent{MediaType:"movie"},
		Primary: &routing.TMDBPrimary{MediaType:"movie", Networks: nil},
	}
	if _, ok := routing.GetField(ctx, "networks"); ok {
		t.Error("networks must be missing on movie kind")
	}
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement**

Replace the placeholder `TMDBPrimary` struct with the full shape:

```go
type TMDBPrimary struct {
	MediaType            string // "movie"|"tv"; redundant with Event but lets fields.go decide kind-only fields
	OriginalLanguage     string
	OriginalTitle        string
	Genres               []string
	Runtime              int
	VoteAverage          float64
	VoteCount            int
	Popularity           float64
	Adult                bool
	Status               string
	ProductionCompanies  []string
	ProductionCountries  []string
	SpokenLanguages      []string

	// Movie-only:
	ReleaseDate         string
	Budget              int
	Revenue             int
	BelongsToCollection string
	IMDBID              string

	// TV-only:
	Networks         []string
	OriginCountry    []string
	FirstAirDate     string
	LastAirDate      string
	Type             string
	InProduction     bool
	NumberOfSeasons  int
	NumberOfEpisodes int
	CreatedBy        []string
}
```

Extend `fieldGroups`:

```go
"original_language": GroupB,
"original_title": GroupB,
"genres": GroupB,
"runtime": GroupB,
"vote_average": GroupB,
"vote_count": GroupB,
"popularity": GroupB,
"adult": GroupB,
"status": GroupB,
"production_companies": GroupB,
"production_countries": GroupB,
"spoken_languages": GroupB,
// movie-only
"release_date": GroupB,
"budget": GroupB,
"revenue": GroupB,
"belongs_to_collection": GroupB,
"imdb_id": GroupB,
// tv-only
"networks": GroupB,
"origin_country": GroupB,
"first_air_date": GroupB,
"last_air_date": GroupB,
"type": GroupB,
"in_production": GroupB,
"number_of_seasons": GroupB,
"number_of_episodes": GroupB,
"created_by": GroupB,
```

Extend `GetField` with one switch arm per field. Return `(nil, false)` if `ctx.Primary == nil`. For kind-mismatched fields (e.g. `networks` on a movie context), return `(nil, false)` rather than the empty slice — the spec calls this "treated as missing".

```go
func GetField(ctx Context, name string) (any, bool) {
	if g, ok := fieldGroups[name]; ok {
		switch g {
		case GroupA:
			return getGroupA(ctx, name)
		case GroupB:
			if ctx.Primary == nil { return nil, false }
			return getGroupB(ctx, name)
		case GroupCKeywords:
			if ctx.Keywords == nil { return nil, false }
			return ctx.Keywords, true
		case GroupCContentRating:
			if ctx.ContentRating == "" { return nil, false }
			return ctx.ContentRating, true
		}
	}
	return nil, false
}

func getGroupB(ctx Context, name string) (any, bool) {
	p := ctx.Primary
	movieOnly := func(v any) (any, bool) {
		if ctx.Event.MediaType != "movie" { return nil, false }
		return v, true
	}
	tvOnly := func(v any) (any, bool) {
		if ctx.Event.MediaType != "tv" { return nil, false }
		return v, true
	}
	switch name {
	case "original_language": return p.OriginalLanguage, true
	case "original_title":    return p.OriginalTitle, true
	case "genres":            return p.Genres, true
	case "runtime":           return p.Runtime, true
	// … etc, including movie/tv kind gating
	case "release_date":      return movieOnly(p.ReleaseDate)
	case "networks":          return tvOnly(p.Networks)
	}
	return nil, false
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/routing
git commit -m "feat(routing): group B field accessors"
```

### Task 3.5: Group C field accessors

**Files:**
- Modify: `internal/routing/fields.go`
- Modify: `internal/routing/fields_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestGroupCKeywordsLoaded(t *testing.T) {
	ctx := routing.Context{Keywords: []string{"anime","time travel"}}
	got, ok := routing.GetField(ctx, "keywords")
	if !ok { t.Fatal("missing") }
	// reflect.DeepEqual got, []string{"anime","time travel"}
}

func TestGroupCKeywordsUnloaded(t *testing.T) {
	if _, ok := routing.GetField(routing.Context{}, "keywords"); ok {
		t.Error("keywords must be missing when not loaded")
	}
}

func TestGroupCContentRating(t *testing.T) {
	ctx := routing.Context{ContentRating: "TV-MA"}
	got, ok := routing.GetField(ctx, "content_rating")
	if !ok || got != "TV-MA" { t.Fatalf("got %v ok %v", got, ok) }
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement**

Add to `fieldGroups`:

```go
"keywords":       GroupCKeywords,
"content_rating": GroupCContentRating,
```

The `GetField` Group C branches were already added in Task 3.4 — this task just registers the names.

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/routing
git commit -m "feat(routing): group C field accessors"
```

### Task 3.6: Evaluator + match_trace

**Files:**
- Create: `internal/routing/trace.go`
- Create: `internal/routing/evaluator.go`
- Create: `internal/routing/evaluator_test.go`

- [ ] **Step 1: Write `trace.go`**

```go
package routing

type RuleResult struct {
	Field   string `json:"field"`
	Op      string `json:"op"`
	Matched bool   `json:"matched"`
	Note    string `json:"note,omitempty"` // type mismatch / invalid regex / missing field
}

type GroupResult struct {
	Match   string       `json:"match"`
	Matched bool         `json:"matched"`
	Rules   []RuleResult `json:"rules"`
}

type ArrTrace struct {
	ArrID   int64         `json:"arr_id"`
	ArrName string        `json:"arr_name"`
	Match   string        `json:"match"`
	Matched bool          `json:"matched"`
	Groups  []GroupResult `json:"groups"`
}

type Trace struct {
	TMDBPrimaryError  string     `json:"tmdb_primary_error,omitempty"`
	KeywordsError     string     `json:"keywords_error,omitempty"`
	ContentRatingErr  string     `json:"content_rating_error,omitempty"`
	Candidates        []ArrTrace `json:"candidates"`
	ChosenArrID       *int64     `json:"chosen_arr_id,omitempty"`
}
```

- [ ] **Step 2: Write the failing test**

```go
package routing_test

import (
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

func TestEvaluateAllMatchAllRules(t *testing.T) { /* match=all, all rules true → true */ }
func TestEvaluateAllMatchOneFalse(t *testing.T) { /* match=all, one rule false → false */ }
func TestEvaluateAnyMatchOneTrue(t *testing.T)  { /* match=any, one rule true  → true */ }
func TestEvaluateEmptyGroupsMatchesEverything(t *testing.T) { /* catch-all */ }
func TestEvaluateMissingFieldIsRuleFalse(t *testing.T) { /* trace.Note = "missing" */ }
func TestEvaluateInvalidRegexRuleFalse(t *testing.T)  { /* trace.Note contains "invalid regex" */ }
```

- [ ] **Step 3: Run to verify failure**

- [ ] **Step 4: Implement `evaluator.go`**

```go
package routing

import "encoding/json"

// Evaluate returns whether the rules match the context, plus a per-group
// trace.
func Evaluate(rules Rules, ctx Context) (bool, []GroupResult) {
	if len(rules.Groups) == 0 {
		return true, nil // empty rules = match everything
	}
	results := make([]GroupResult, 0, len(rules.Groups))
	matched := rules.Match == "all"
	for _, g := range rules.Groups {
		gr := evalGroup(g, ctx)
		results = append(results, gr)
		switch rules.Match {
		case "all":
			matched = matched && gr.Matched
		case "any":
			matched = matched || gr.Matched
		}
	}
	return matched, results
}

func evalGroup(g Group, ctx Context) GroupResult {
	out := GroupResult{Match: string(g.Match), Rules: make([]RuleResult, 0, len(g.Rules))}
	matched := g.Match == "all"
	for _, r := range g.Rules {
		rr := evalRule(r, ctx)
		out.Rules = append(out.Rules, rr)
		switch g.Match {
		case "all":
			matched = matched && rr.Matched
		case "any":
			matched = matched || rr.Matched
		}
	}
	out.Matched = matched
	return out
}

func evalRule(r Rule, ctx Context) RuleResult {
	actual, ok := GetField(ctx, r.Field)
	if !ok {
		return RuleResult{Field: r.Field, Op: r.Op, Matched: false, Note: "missing"}
	}
	matched, note := Apply(r.Op, actual, json.RawMessage(r.Value))
	return RuleResult{Field: r.Field, Op: r.Op, Matched: matched, Note: note}
}
```

- [ ] **Step 5: Run tests to verify pass**

- [ ] **Step 6: Commit**

```bash
git add internal/routing/trace.go internal/routing/evaluator.go internal/routing/evaluator_test.go
git commit -m "feat(routing): evaluator with match_trace"
```

### Task 3.7: Router (orchestrator)

**Files:**
- Create: `internal/routing/router.go`
- Create: `internal/routing/router_test.go`

The router is the public entry point: takes the registry + an event + an enrichment fetcher (interface), returns the chosen *arr (or nil) plus the trace.

- [ ] **Step 1: Write the failing test**

```go
package routing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

type fakeEnricher struct {
	primary    *routing.TMDBPrimary
	keywords   []string
	rating     string
	primaryErr error
}

func (f *fakeEnricher) Primary(ctx context.Context, mt string, id int)   (*routing.TMDBPrimary, error) { return f.primary, f.primaryErr }
func (f *fakeEnricher) Keywords(ctx context.Context, mt string, id int)   ([]string, error)             { return f.keywords, nil }
func (f *fakeEnricher) ContentRating(ctx context.Context, mt string, id int) (string, error)            { return f.rating, nil }

func TestRouterFirstMatchWins(t *testing.T) { /* two enabled radarrs; lower-priority unmatched → second chosen */ }
func TestRouterKindFilter(t *testing.T)     { /* sonarr in registry skipped on movie event */ }
func TestRouterDisabledSkipped(t *testing.T) { /* enabled=false skipped */ }
func TestRouterNoMatchUnrouted(t *testing.T) { /* returns chosen=nil */ }
func TestRouterLazyFetchOnlyWhenNeeded(t *testing.T) {
	// rules use only Group A → enricher.Primary not called
}
func TestRouterTMDBErrorIsRecorded(t *testing.T) {
	// enricher.Primary returns error → routing proceeds, Group B rules false,
	// trace.TMDBPrimaryError set
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `router.go`**

```go
package routing

import (
	"context"
	"errors"
)

type Enricher interface {
	Primary(ctx context.Context, mediaType string, tmdbID int) (*TMDBPrimary, error)
	Keywords(ctx context.Context, mediaType string, tmdbID int) ([]string, error)
	ContentRating(ctx context.Context, mediaType string, tmdbID int) (string, error)
}

type Candidate struct {
	ID    int64
	Name  string
	Kind  string
	Rules Rules
}

// Decide picks the first enabled, kind-matching candidate whose rules match.
// Candidates must already be sorted (priority ASC, id ASC) by the caller.
// Returns the chosen ID (nil if no match) plus the full trace.
func Decide(ctx context.Context, candidates []Candidate, ev RequestEvent, enr Enricher) (*int64, Trace) {
	relevant := filterByKind(candidates, ev.MediaType)
	needPrimary, needKeywords, needRating := analyzeNeeds(relevant)

	rctx := Context{Event: ev}
	trace := Trace{}

	if needPrimary {
		p, err := enr.Primary(ctx, ev.MediaType, ev.TMDBID)
		if err != nil {
			trace.TMDBPrimaryError = err.Error()
		} else {
			rctx.Primary = p
		}
	}
	if needKeywords {
		k, err := enr.Keywords(ctx, ev.MediaType, ev.TMDBID)
		if err != nil {
			trace.KeywordsError = err.Error()
		} else {
			rctx.Keywords = k
		}
	}
	if needRating {
		r, err := enr.ContentRating(ctx, ev.MediaType, ev.TMDBID)
		if err != nil {
			trace.ContentRatingErr = err.Error()
		} else {
			rctx.ContentRating = r
		}
	}

	for _, c := range relevant {
		matched, groups := Evaluate(c.Rules, rctx)
		trace.Candidates = append(trace.Candidates, ArrTrace{
			ArrID: c.ID, ArrName: c.Name,
			Match: string(c.Rules.Match), Matched: matched,
			Groups: groups,
		})
		if matched {
			id := c.ID
			trace.ChosenArrID = &id
			return &id, trace
		}
	}
	return nil, trace
}

func filterByKind(in []Candidate, mediaType string) []Candidate {
	var kind string
	switch mediaType {
	case "movie": kind = "radarr"
	case "tv":    kind = "sonarr"
	default:      return nil
	}
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if c.Kind == kind { out = append(out, c) }
	}
	return out
}

// analyzeNeeds inspects the rules to decide which enrichment groups must be
// fetched. Touching only Group A → no fetches.
func analyzeNeeds(cs []Candidate) (primary, keywords, rating bool) {
	for _, c := range cs {
		for _, g := range c.Rules.Groups {
			for _, r := range g.Rules {
				grp, ok := FieldGroupOf(r.Field)
				if !ok { continue }
				switch grp {
				case GroupB:           primary = true
				case GroupCKeywords:   keywords = true
				case GroupCContentRating: rating = true
				}
			}
		}
	}
	return
}

var ErrNoCandidates = errors.New("no candidates")
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/routing/router.go internal/routing/router_test.go
git commit -m "feat(routing): router with lazy enrichment + kind filter"
```

---

## Phase 4 — TMDB client + 3-group cache

### Task 4.1: TMDB client — primary lookup

**Files:**
- Create: `internal/tmdb/client.go`
- Create: `internal/tmdb/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
package tmdb_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/tmdb"
)

func TestPrimaryMovie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/603" { t.Fatalf("path: %s", r.URL.Path) }
		if r.URL.Query().Get("api_key") != "k" { t.Fatal("missing api_key") }
		w.Write([]byte(`{
			"original_language":"en","original_title":"The Matrix",
			"genres":[{"name":"Action"},{"name":"Sci-Fi"}],
			"runtime":136,"vote_average":8.2,"vote_count":24000,"popularity":110.5,
			"adult":false,"status":"Released",
			"production_companies":[{"name":"Warner Bros."}],
			"production_countries":[{"iso_3166_1":"US"}],
			"spoken_languages":[{"iso_639_1":"en"}],
			"release_date":"1999-03-30","budget":63000000,"revenue":463517383,
			"belongs_to_collection":{"name":"The Matrix Collection"},
			"imdb_id":"tt0133093"
		}`))
	}))
	defer srv.Close()
	c := tmdb.New(srv.URL, "k", "en-US")
	got, err := c.Primary(context.Background(), "movie", 603)
	if err != nil { t.Fatal(err) }
	if got.OriginalTitle != "The Matrix" { t.Errorf("title: %s", got.OriginalTitle) }
	if got.BelongsToCollection != "The Matrix Collection" { t.Errorf("collection: %s", got.BelongsToCollection) }
	// … assert remaining fields
}

func TestPrimaryTV(t *testing.T) { /* /tv/{id}, original_name, episode_run_time[0], networks, origin_country, etc. */ }

func TestPrimaryNotFoundReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := tmdb.New(srv.URL, "k", "en-US")
	_, err := c.Primary(context.Background(), "movie", 603)
	if err == nil { t.Fatal("expected error") }
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tmdb/ -run Primary -v`
Expected: build error.

- [ ] **Step 3: Implement `client.go`**

```go
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

type Client struct {
	baseURL  string
	apiKey   string
	language string
	http     *http.Client
}

func New(baseURL, apiKey, language string) *Client {
	if language == "" { language = "en-US" }
	return &Client{baseURL: baseURL, apiKey: apiKey, language: language, http: &http.Client{Timeout: 10 * time.Second}}
}

// raw JSON shapes — kept private; we map them to routing.TMDBPrimary.
type primaryMovie struct {
	OriginalLanguage    string  `json:"original_language"`
	OriginalTitle       string  `json:"original_title"`
	Genres              []named `json:"genres"`
	Runtime             int     `json:"runtime"`
	VoteAverage         float64 `json:"vote_average"`
	VoteCount           int     `json:"vote_count"`
	Popularity          float64 `json:"popularity"`
	Adult               bool    `json:"adult"`
	Status              string  `json:"status"`
	ProductionCompanies []named `json:"production_companies"`
	ProductionCountries []iso31661 `json:"production_countries"`
	SpokenLanguages     []iso6391  `json:"spoken_languages"`
	ReleaseDate         string `json:"release_date"`
	Budget              int    `json:"budget"`
	Revenue             int    `json:"revenue"`
	BelongsToCollection *named `json:"belongs_to_collection"`
	IMDBID              string `json:"imdb_id"`
}
type primaryTV struct {
	OriginalLanguage    string  `json:"original_language"`
	OriginalName        string  `json:"original_name"`
	Genres              []named `json:"genres"`
	EpisodeRunTime      []int   `json:"episode_run_time"`
	VoteAverage         float64 `json:"vote_average"`
	VoteCount           int     `json:"vote_count"`
	Popularity          float64 `json:"popularity"`
	Adult               bool    `json:"adult"`
	Status              string  `json:"status"`
	ProductionCompanies []named    `json:"production_companies"`
	ProductionCountries []iso31661 `json:"production_countries"`
	SpokenLanguages     []iso6391  `json:"spoken_languages"`
	Networks            []named  `json:"networks"`
	OriginCountry       []string `json:"origin_country"`
	FirstAirDate        string   `json:"first_air_date"`
	LastAirDate         string   `json:"last_air_date"`
	Type                string   `json:"type"`
	InProduction        bool     `json:"in_production"`
	NumberOfSeasons     int      `json:"number_of_seasons"`
	NumberOfEpisodes    int      `json:"number_of_episodes"`
	CreatedBy           []named  `json:"created_by"`
}
type named struct{ Name string `json:"name"` }
type iso31661 struct{ Code string `json:"iso_3166_1"` }
type iso6391 struct{ Code string `json:"iso_639_1"` }

func (c *Client) Primary(ctx context.Context, mediaType string, tmdbID int) (*routing.TMDBPrimary, error) {
	switch mediaType {
	case "movie":
		var m primaryMovie
		if err := c.getJSON(ctx, fmt.Sprintf("/movie/%d", tmdbID), &m); err != nil { return nil, err }
		return movieToPrimary(m), nil
	case "tv":
		var t primaryTV
		if err := c.getJSON(ctx, fmt.Sprintf("/tv/%d", tmdbID), &t); err != nil { return nil, err }
		return tvToPrimary(t), nil
	}
	return nil, fmt.Errorf("unknown mediaType %q", mediaType)
}

func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	u, _ := url.Parse(c.baseURL + path)
	q := u.Query()
	q.Set("api_key", c.apiKey)
	q.Set("language", c.language)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil { return err }
	resp, err := c.http.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("tmdb %s: %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func movieToPrimary(m primaryMovie) *routing.TMDBPrimary { /* map fields, set MediaType="movie" */ return nil }
func tvToPrimary(t primaryTV) *routing.TMDBPrimary       { /* map fields, runtime = first of EpisodeRunTime */ return nil }
```

Fill the two mapping helpers with one assignment per field.

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/tmdb/client.go internal/tmdb/client_test.go
git commit -m "feat(tmdb): primary lookup for movies + tv"
```

### Task 4.2: TMDB client — keywords + content rating

**Files:**
- Modify: `internal/tmdb/client.go`
- Modify: `internal/tmdb/client_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestKeywordsMovie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/603/keywords" { t.Fatal(r.URL.Path) }
		w.Write([]byte(`{"keywords":[{"name":"matrix"},{"name":"hacker"}]}`))
	}))
	defer srv.Close()
	c := tmdb.New(srv.URL, "k", "")
	got, err := c.Keywords(context.Background(), "movie", 603)
	if err != nil { t.Fatal(err) }
	if len(got) != 2 || got[0] != "matrix" { t.Fatalf("got %v", got) }
}

func TestKeywordsTV(t *testing.T) {
	// /tv/{id}/keywords returns {"results":[{"name":"…"}]}
}

func TestContentRatingMovieUS(t *testing.T) {
	// /movie/{id}/release_dates → first US release_dates[].certification non-empty
}

func TestContentRatingTVUS(t *testing.T) {
	// /tv/{id}/content_ratings → results where iso_3166_1=="US" → rating
}

func TestContentRatingMissingReturnsEmpty(t *testing.T) {
	// 200 response with no US entry → empty string, no error
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement**

```go
type movieKeywords struct{ Keywords []named `json:"keywords"` }
type tvKeywords    struct{ Results  []named `json:"results"` }

func (c *Client) Keywords(ctx context.Context, mediaType string, tmdbID int) ([]string, error) {
	switch mediaType {
	case "movie":
		var k movieKeywords
		if err := c.getJSON(ctx, fmt.Sprintf("/movie/%d/keywords", tmdbID), &k); err != nil { return nil, err }
		return namesOf(k.Keywords), nil
	case "tv":
		var k tvKeywords
		if err := c.getJSON(ctx, fmt.Sprintf("/tv/%d/keywords", tmdbID), &k); err != nil { return nil, err }
		return namesOf(k.Results), nil
	}
	return nil, fmt.Errorf("unknown mediaType %q", mediaType)
}

type movieReleaseDates struct {
	Results []struct {
		Country      string `json:"iso_3166_1"`
		ReleaseDates []struct{ Certification string `json:"certification"` } `json:"release_dates"`
	} `json:"results"`
}
type tvContentRatings struct {
	Results []struct {
		Country string `json:"iso_3166_1"`
		Rating  string `json:"rating"`
	} `json:"results"`
}

func (c *Client) ContentRating(ctx context.Context, mediaType string, tmdbID int) (string, error) {
	switch mediaType {
	case "movie":
		var r movieReleaseDates
		if err := c.getJSON(ctx, fmt.Sprintf("/movie/%d/release_dates", tmdbID), &r); err != nil { return "", err }
		for _, c := range r.Results {
			if c.Country == "US" {
				for _, d := range c.ReleaseDates {
					if d.Certification != "" { return d.Certification, nil }
				}
			}
		}
	case "tv":
		var r tvContentRatings
		if err := c.getJSON(ctx, fmt.Sprintf("/tv/%d/content_ratings", tmdbID), &r); err != nil { return "", err }
		for _, c := range r.Results {
			if c.Country == "US" && c.Rating != "" { return c.Rating, nil }
		}
	}
	return "", nil
}

func namesOf(in []named) []string {
	out := make([]string, len(in))
	for i, n := range in { out[i] = n.Name }
	return out
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/tmdb
git commit -m "feat(tmdb): keywords + US content rating"
```

### Task 4.3: 3-group TTL cache

**Files:**
- Create: `internal/tmdb/cache.go`
- Modify: `internal/tmdb/client_test.go` (add cache tests)

The cache wraps a `Client` and implements `routing.Enricher`. Three independent `sync.Map`s, one TTL.

- [ ] **Step 1: Write the failing test**

```go
func TestCacheHitAvoidsSecondCall(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++; w.Write([]byte(`{"original_language":"en"}`))
	}))
	defer srv.Close()
	c := tmdb.NewCache(tmdb.New(srv.URL, "k", ""), 24*time.Hour)
	_, _ = c.Primary(context.Background(), "movie", 1)
	_, _ = c.Primary(context.Background(), "movie", 1)
	if calls != 1 { t.Fatalf("calls=%d, want 1", calls) }
}

func TestCacheTTLExpires(t *testing.T) {
	// nowFunc injection: advance the clock past TTL → second call hits the upstream again
}

func TestCacheSeparateGroups(t *testing.T) {
	// hitting Primary populates only the primary cache, not keywords
}

func TestCacheUpstreamErrorIsNotCached(t *testing.T) {
	// upstream 500 → second call also hits upstream
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `cache.go`**

```go
package tmdb

import (
	"context"
	"sync"
	"time"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
)

type Cache struct {
	upstream *Client
	ttl      time.Duration
	now      func() time.Time
	primary  sync.Map // key: cacheKey, val: primaryEntry
	keywords sync.Map // key: cacheKey, val: keywordsEntry
	rating   sync.Map // key: cacheKey, val: ratingEntry
}

type cacheKey struct{ MediaType string; ID int }
type primaryEntry  struct{ V *routing.TMDBPrimary; At time.Time }
type keywordsEntry struct{ V []string;             At time.Time }
type ratingEntry   struct{ V string;               At time.Time }

func NewCache(upstream *Client, ttl time.Duration) *Cache {
	return &Cache{upstream: upstream, ttl: ttl, now: time.Now}
}

func (c *Cache) Primary(ctx context.Context, mt string, id int) (*routing.TMDBPrimary, error) {
	k := cacheKey{mt, id}
	if v, ok := c.primary.Load(k); ok {
		e := v.(primaryEntry)
		if c.now().Sub(e.At) < c.ttl { return e.V, nil }
	}
	v, err := c.upstream.Primary(ctx, mt, id)
	if err != nil { return nil, err }
	c.primary.Store(k, primaryEntry{V: v, At: c.now()})
	return v, nil
}

// Keywords and ContentRating mirror Primary — same shape.
func (c *Cache) Keywords(ctx context.Context, mt string, id int) ([]string, error) { /* … */ return nil, nil }
func (c *Cache) ContentRating(ctx context.Context, mt string, id int) (string, error) { /* … */ return "", nil }
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/tmdb
git commit -m "feat(tmdb): 3-group TTL cache implementing routing.Enricher"
```

---

## Phase 5 — *arr clients

### Task 5.1: Port arr package from arrproxy

**Files:**
- Create: `internal/arr/{common,profiles,radarr,sonarr}.go`

The Radarr/Sonarr v3 clients are functionally identical to arrproxy's. Port verbatim, then adjust any imports.

- [ ] **Step 1: Copy files**

```bash
cp /opt/continuum-plugin-arrproxy/internal/arr/*.go internal/arr/
```

- [ ] **Step 2: Update import paths**

`grep -l ContinuumApp/continuum-plugin-arrproxy internal/arr/*.go | xargs sed -i 's|continuum-plugin-arrproxy|continuum-plugin-arrouter|g'` — but preview the change first.

- [ ] **Step 3: Add a minimal smoke test**

`internal/arr/radarr_test.go`:

```go
package arr_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/arr"
)

func TestRadarrAddSendsTMDBID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v3/movie" { t.Fatalf("%s %s", r.Method, r.URL.Path) }
		w.Write([]byte(`{"id":42}`))
	}))
	defer srv.Close()
	c := arr.NewRadarr(srv.URL, "k", &http.Client{})
	id, err := c.Add(context.Background(), arr.MovieAdd{Title:"X", TMDBID:603, Year:1999, RootFolderPath:"/movies"})
	if err != nil { t.Fatal(err) }
	if id != 42 { t.Fatalf("id=%d", id) }
}
```

- [ ] **Step 4: Run `go test ./internal/arr/...`**

Expected: PASS (or whatever pre-existing tests in arrproxy already cover — port them too if present).

- [ ] **Step 5: Commit**

```bash
git add internal/arr
git commit -m "feat(arr): port Radarr/Sonarr v3 clients from arrproxy"
```

Note: when arrproxy gets bug fixes to these files, port them. Diverging is allowed but should be a deliberate decision.

Sanity-check after the copy that the package exports the helpers later phases depend on:
- `arr.IsAlreadyExists(err) bool` — used by `consumer.SubmitHandler` (Task 7.2) to detect Radarr/Sonarr's "409 already exists" error. If arrproxy's package doesn't already export this, add it as a one-line predicate over the typed error the clients return.
- `arr.Radarr.HasFile(ctx, externalID) (bool, error)` and `arr.Radarr.InQueue(ctx, externalID) (bool, error)` — used by the poll loop (Task 8.1).
- `arr.Sonarr.PercentImported(ctx, externalID) (int, error)` and `arr.Sonarr.InQueue(ctx, externalID) (bool, error)` — used by Task 8.2.
- `arr.Radarr.Delete(ctx, externalID) error` and `arr.Sonarr.Delete(ctx, externalID) error` — used by the cancel handler (Task 7.4).
- `arr.SystemStatus(ctx, url, apiKey) (...)` — used by the test-connection endpoint (Task 9.3).

If any of these names differ in the arrproxy source, harmonize them before continuing — later tasks reference these exact signatures.

---

## Phase 6 — Event publisher

### Task 6.1: Publisher

**Files:**
- Create: `internal/event/publisher.go`

The publisher mirrors arrproxy's, only the event names differ. The host's `RuntimeHost.v1` SDK service is the underlying transport — wrap it in a typed helper so consumer/poll code doesn't have to format event names.

- [ ] **Step 1: Copy and rename**

```bash
cp /opt/continuum-plugin-arrproxy/internal/event/publisher.go internal/event/publisher.go
sed -i 's|arrproxy|arrouter|g' internal/event/publisher.go
```

- [ ] **Step 2: Add the new event helpers**

Open `internal/event/publisher.go` and add an `Unrouted` helper:

```go
func (p *Publisher) Unrouted(ctx context.Context, requestID, reason string) error {
	return p.publish(ctx, "unrouted", map[string]any{
		"requestId": requestID,
		"reason":    reason,
		"at":        time.Now().UTC().Format(time.RFC3339),
	})
}
```

The arrproxy file already defines `Submitted`, `Downloading`, `Imported`, `Failed`, `Cancelled` — those become arrouter's event names automatically by virtue of the `arrouter` rename in Step 1.

- [ ] **Step 3: Smoke-test the rename**

Run: `go vet ./internal/event/...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/event
git commit -m "feat(event): publisher with arrouter event names + unrouted"
```

---

## Phase 7 — Consumer (event handlers)

The consumer subscribes to `plugin.continuum.requests.{submitted,cancelled}`. We split the two handlers across files for readability.

### Task 7.1: Consumer skeleton + dispatch

**Files:**
- Create: `internal/consumer/consumer.go`
- Create: `internal/consumer/consumer_test.go`

- [ ] **Step 1: Write the failing test**

```go
package consumer_test

import (
	"context"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/consumer"
)

type fakeSubmitter struct{ called bool }
func (f *fakeSubmitter) HandleSubmitted(ctx context.Context, payload map[string]any) error { f.called = true; return nil }

type fakeCanceller struct{ called bool }
func (f *fakeCanceller) HandleCancelled(ctx context.Context, payload map[string]any) error { f.called = true; return nil }

func TestDispatchSubmitted(t *testing.T) {
	s := &fakeSubmitter{}; c := &fakeCanceller{}
	d := consumer.New(s, c, nil)
	if err := d.Handle(context.Background(), "plugin.continuum.requests.submitted", map[string]any{"requestId":"r1"}); err != nil { t.Fatal(err) }
	if !s.called || c.called { t.Fatalf("submit=%v cancel=%v", s.called, c.called) }
}

func TestDispatchCancelled(t *testing.T) { /* mirror */ }

func TestDispatchUnknownEventIgnored(t *testing.T) {
	d := consumer.New(&fakeSubmitter{}, &fakeCanceller{}, nil)
	if err := d.Handle(context.Background(), "plugin.continuum.unrelated", map[string]any{}); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `consumer.go`**

```go
package consumer

import (
	"context"

	"github.com/hashicorp/go-hclog"
)

type Submitter interface { HandleSubmitted(context.Context, map[string]any) error }
type Canceller interface { HandleCancelled(context.Context, map[string]any) error }

type Dispatcher struct {
	submit Submitter
	cancel Canceller
	log    hclog.Logger
}

func New(s Submitter, c Canceller, log hclog.Logger) *Dispatcher {
	if log == nil { log = hclog.NewNullLogger() }
	return &Dispatcher{submit: s, cancel: c, log: log}
}

func (d *Dispatcher) Handle(ctx context.Context, eventName string, payload map[string]any) error {
	switch eventName {
	case "plugin.continuum.requests.submitted":
		return d.submit.HandleSubmitted(ctx, payload)
	case "plugin.continuum.requests.cancelled":
		return d.cancel.HandleCancelled(ctx, payload)
	}
	d.log.Debug("ignoring event", "name", eventName)
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/consumer/consumer.go internal/consumer/consumer_test.go
git commit -m "feat(consumer): dispatcher skeleton"
```

### Task 7.2: Submit handler — happy path

**Files:**
- Create: `internal/consumer/submit.go`
- Modify: `internal/consumer/consumer_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSubmitHappyPath(t *testing.T) {
	// fakes: store, enricher, radarr/sonarr clients, publisher
	// payload: movie, tmdbId 603, year 1999
	// registry: one enabled radarr with empty rules (catch-all)
	// expect:
	//   - store.UpsertRequestQueued called once
	//   - store.SetRoutedArr called with arr.id and a non-empty trace
	//   - radarr.Add called with title/tmdbId/year
	//   - store.MarkSubmitted called with externalID from radarr.Add
	//   - publisher.Submitted called once
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `submit.go`**

```go
package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/arr"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

type SubmitHandler struct {
	Store     *store.Store
	Enricher  routing.Enricher
	Radarr    func(url, apiKey string) *arr.Radarr   // factory; one client per *arr
	Sonarr    func(url, apiKey string) *arr.Sonarr
	Events    *event.Publisher
	SecretKey string
	Log       hclog.Logger
}

func (h *SubmitHandler) HandleSubmitted(ctx context.Context, p map[string]any) error {
	ev, err := parseSubmitPayload(p)
	if err != nil { return err }

	r := &store.Request{
		ID: ev.RequestID, TMDBID: ev.TMDBID, MediaType: ev.MediaType,
		Title: ev.Title, Year: ev.Year, PosterURL: ev.PosterURL,
		RequesterUserID: ev.RequesterUserID, RequesterIsAdmin: ev.RequesterIsAdmin,
		Status: "queued",
	}
	if err := h.Store.UpsertRequestQueued(ctx, r); err != nil { return err }

	candidates, err := h.loadCandidates(ctx, ev.MediaType)
	if err != nil { return err }

	chosen, trace := routing.Decide(ctx, candidates, ev, h.Enricher)
	traceJSON, _ := json.Marshal(trace)

	if chosen == nil {
		_ = h.Store.MarkUnrouted(ctx, ev.RequestID, traceJSON, "no registered *arr matched")
		_ = h.Events.Unrouted(ctx, ev.RequestID, "no registered *arr matched")
		return nil
	}

	if err := h.Store.SetRoutedArr(ctx, ev.RequestID, *chosen, traceJSON); err != nil { return err }

	a, err := h.Store.GetArr(ctx, *chosen)
	if err != nil || a == nil { return fmt.Errorf("get arr %d: %w", *chosen, err) }

	apiKey, err := crypto.Open(h.SecretKey, a.APIKey)
	if err != nil { return fmt.Errorf("decrypt api_key: %w", err) }

	externalID, addErr := h.submitToArr(ctx, a, apiKey, ev)
	switch {
	case addErr == nil:
		if err := h.Store.MarkSubmitted(ctx, ev.RequestID, externalID); err != nil { return err }
		_ = h.Events.Submitted(ctx, ev.RequestID)
		return nil
	case arr.IsAlreadyExists(addErr):
		// 409 — treat as already there (set status submitted; external_id may be 0)
		_ = h.Store.MarkSubmitted(ctx, ev.RequestID, externalID)
		_ = h.Events.Submitted(ctx, ev.RequestID)
		return nil
	default:
		_ = h.Store.MarkFailed(ctx, ev.RequestID, addErr.Error())
		_ = h.Events.Failed(ctx, ev.RequestID, addErr.Error())
		return nil
	}
}

func (h *SubmitHandler) loadCandidates(ctx context.Context, mediaType string) ([]routing.Candidate, error) {
	kind := "radarr"; if mediaType == "tv" { kind = "sonarr" }
	rows, err := h.Store.ListEnabledArrsByKind(ctx, kind)
	if err != nil { return nil, err }
	out := make([]routing.Candidate, 0, len(rows))
	for _, r := range rows {
		rules, _ := routing.ParseRules(r.RulesJSON)
		out = append(out, routing.Candidate{ID: r.ID, Name: r.Name, Kind: r.Kind, Rules: rules})
	}
	return out, nil
}

func (h *SubmitHandler) submitToArr(ctx context.Context, a *store.RegisteredArr, apiKey string, ev routing.RequestEvent) (int, error) {
	switch a.Kind {
	case "radarr":
		c := h.Radarr(a.URL, apiKey)
		return c.Add(ctx, arr.MovieAdd{
			Title: ev.Title, TMDBID: ev.TMDBID, Year: ev.Year,
			QualityProfileID: a.QualityProfileID,
			RootFolderPath:   a.RootFolderPath,
		})
	case "sonarr":
		c := h.Sonarr(a.URL, apiKey)
		return c.Add(ctx, arr.SeriesAdd{
			Title: ev.Title, TMDBID: ev.TMDBID, Year: ev.Year,
			QualityProfileID:  a.QualityProfileID,
			LanguageProfileID: a.LanguageProfileID,
			RootFolderPath:    a.RootFolderPath,
		})
	}
	return 0, fmt.Errorf("unknown arr kind %q", a.Kind)
}

func parseSubmitPayload(p map[string]any) (routing.RequestEvent, error) {
	// Defensively coerce types — events arrive as map[string]any.
	// Use a small helper or type-assert each field. Return error on missing
	// required fields (requestId, tmdbId, mediaType).
	return routing.RequestEvent{}, nil
}
```

Implement `parseSubmitPayload` with explicit type assertions.

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/consumer
git commit -m "feat(consumer): submit handler with routing + arr submit"
```

### Task 7.3: Submit handler — error paths

**Files:**
- Modify: `internal/consumer/consumer_test.go`

- [ ] **Step 1: Add tests covering each error path**

```go
func TestSubmitNoMatchUnrouted(t *testing.T)             { /* registry empty → unrouted, event published */ }
func TestSubmitArrReturns409TreatedAsSubmitted(t *testing.T) { /* arr.IsAlreadyExists(err) → MarkSubmitted */ }
func TestSubmitArrReturns500MarksFailed(t *testing.T)    { /* MarkFailed + Failed event */ }
func TestSubmitArrUnreachableMarksFailed(t *testing.T)   { /* network error → after one immediate retry → MarkFailed */ }
func TestSubmitDoesNotFallThroughToNextArr(t *testing.T) { /* on 500 we do NOT POST to the next priority arr */ }
func TestSubmitTMDBPrimaryFailureStillRoutes(t *testing.T) {
	// rules use Group A only → no TMDB call needed; routing should succeed
}
```

- [ ] **Step 2: Run; some will fail. Add the immediate retry to `submitToArr`** (one retry on transport error; no retry on 4xx/5xx response).

- [ ] **Step 3: Run tests to verify pass**

- [ ] **Step 4: Commit**

```bash
git add internal/consumer
git commit -m "feat(consumer): submit handler error paths + retry on transport"
```

### Task 7.4: Cancel handler

**Files:**
- Create: `internal/consumer/cancel.go`
- Modify: `internal/consumer/consumer_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCancelUnknownIDIsNoOp(t *testing.T) { /* GetRequest returns nil → no-op, no error */ }
func TestCancelTerminalIsNoOp(t *testing.T)   { /* status=imported → no-op */ }
func TestCancelSubmittedDeletesAtArrAndMarks(t *testing.T) {
	// status=submitted, external_id set → DELETE /api/v3/movie/{id}, MarkCancelled, publish
}
func TestCancelDownloadingDeletesAtArrAndMarks(t *testing.T) { /* mirror */ }
func TestCancelArrUnreachableStillMarksLocal(t *testing.T)    { /* DELETE fails → MarkCancelled anyway */ }
func TestCancelOrphanedRoutedArrSkipsDelete(t *testing.T)     { /* routed_arr_id NULL → no DELETE, mark cancelled */ }
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `cancel.go`**

```go
package consumer

import (
	"context"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/arr"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

type CancelHandler struct {
	Store     *store.Store
	Radarr    func(url, apiKey string) *arr.Radarr
	Sonarr    func(url, apiKey string) *arr.Sonarr
	Events    *event.Publisher
	SecretKey string
	Log       hclog.Logger
}

func (h *CancelHandler) HandleCancelled(ctx context.Context, p map[string]any) error {
	id, _ := p["requestId"].(string)
	if id == "" { return nil }
	r, err := h.Store.GetRequest(ctx, id)
	if err != nil || r == nil { return nil }
	switch r.Status {
	case "imported", "failed", "cancelled", "unrouted":
		return nil
	}
	if r.RoutedArrID != nil && r.ExternalID != nil {
		a, err := h.Store.GetArr(ctx, *r.RoutedArrID)
		if err == nil && a != nil {
			apiKey, err := crypto.Open(h.SecretKey, a.APIKey)
			if err == nil {
				switch a.Kind {
				case "radarr":
					_ = h.Radarr(a.URL, apiKey).Delete(ctx, *r.ExternalID)
				case "sonarr":
					_ = h.Sonarr(a.URL, apiKey).Delete(ctx, *r.ExternalID)
				}
			}
		}
	}
	if err := h.Store.MarkCancelled(ctx, id); err != nil { return err }
	_ = h.Events.Cancelled(ctx, id)
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/consumer
git commit -m "feat(consumer): cancel handler"
```

---

## Phase 8 — Poll loop

Mirrors arrproxy with one change: per-arr fanout instead of one global loop. Each registered *arr gets its own goroutine so a slow instance can't starve the others.

### Task 8.1: Per-row poll, movie path

**Files:**
- Create: `internal/poll/poll.go`
- Create: `internal/poll/poll_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestPollMovieHasFileMarksImported(t *testing.T) { /* GET /api/v3/movie/X → hasFile=true → MarkImported, publish */ }
func TestPollMovieInQueueMarksDownloading(t *testing.T)        { /* in queue → MarkDownloading once → publish on transition */ }
func TestPollMovieInQueueAgainNoTransitionNoEvent(t *testing.T) { /* second poll → publish not called again */ }
func TestPollMovieStaleMarksFailed(t *testing.T)                 { /* not hasFile, not in queue, submitted_at < now-72h → MarkFailed */ }
func TestPollMovieNotStaleLeavesAlone(t *testing.T)              { /* same but submitted_at recent → no change */ }
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement movie path in `poll.go`**

```go
package poll

import (
	"context"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/arr"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

type Deps struct {
	Store           *store.Store
	Radarr          func(url, apiKey string) *arr.Radarr
	Sonarr          func(url, apiKey string) *arr.Sonarr
	Events          *event.Publisher
	StaleAfterHours int
	SecretKey       string
}

type Poller struct {
	deps func() *Deps
	log  hclog.Logger
}

func New(deps func() *Deps, log hclog.Logger) *Poller { return &Poller{deps: deps, log: log} }

func (p *Poller) pollOne(ctx context.Context, d *Deps, r *store.Request, a *store.RegisteredArr, apiKey string, now time.Time) {
	switch r.MediaType {
	case "movie":
		p.pollMovie(ctx, d, r, a, apiKey, now)
	case "tv":
		p.pollTV(ctx, d, r, a, apiKey, now)
	}
	_ = d.Store.UpdateLastPolled(ctx, r.ID, now)
}

func (p *Poller) pollMovie(ctx context.Context, d *Deps, r *store.Request, a *store.RegisteredArr, apiKey string, now time.Time) {
	if r.ExternalID == nil { return }
	c := d.Radarr(a.URL, apiKey)
	if hasFile, err := c.HasFile(ctx, *r.ExternalID); err == nil && hasFile {
		_ = d.Store.MarkImported(ctx, r.ID)
		_ = d.Events.Imported(ctx, r.ID)
		return
	}
	if inQueue, err := c.InQueue(ctx, *r.ExternalID); err == nil && inQueue {
		if t, _ := d.Store.MarkDownloading(ctx, r.ID); t {
			_ = d.Events.Downloading(ctx, r.ID)
		}
		return
	}
	if r.SubmittedAt != nil && now.Sub(*r.SubmittedAt) > time.Duration(d.StaleAfterHours)*time.Hour {
		msg := "stuck past staleness threshold"
		_ = d.Store.MarkFailed(ctx, r.ID, msg)
		_ = d.Events.Failed(ctx, r.ID, msg)
	}
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/poll
git commit -m "feat(poll): movie path with hasFile/queue/staleness"
```

### Task 8.2: Per-row poll, TV path

**Files:**
- Modify: `internal/poll/poll.go`
- Modify: `internal/poll/poll_test.go`

- [ ] **Step 1: Write the failing tests**

Mirror Task 8.1 — `percentOfEpisodes==100 → imported`, `seriesId in queue → downloading`, staleness identical.

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `pollTV` (mirror `pollMovie`)**

```go
func (p *Poller) pollTV(ctx context.Context, d *Deps, r *store.Request, a *store.RegisteredArr, apiKey string, now time.Time) {
	if r.ExternalID == nil { return }
	c := d.Sonarr(a.URL, apiKey)
	if pct, err := c.PercentImported(ctx, *r.ExternalID); err == nil && pct >= 100 {
		_ = d.Store.MarkImported(ctx, r.ID)
		_ = d.Events.Imported(ctx, r.ID)
		return
	}
	if inQueue, err := c.InQueue(ctx, *r.ExternalID); err == nil && inQueue {
		if t, _ := d.Store.MarkDownloading(ctx, r.ID); t {
			_ = d.Events.Downloading(ctx, r.ID)
		}
		return
	}
	if r.SubmittedAt != nil && now.Sub(*r.SubmittedAt) > time.Duration(d.StaleAfterHours)*time.Hour {
		msg := "stuck past staleness threshold"
		_ = d.Store.MarkFailed(ctx, r.ID, msg)
		_ = d.Events.Failed(ctx, r.ID, msg)
	}
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/poll
git commit -m "feat(poll): tv path with percentImported"
```

### Task 8.3: Per-arr fanout + scheduled task

**Files:**
- Create: `internal/poll/scheduled.go`
- Modify: `internal/poll/poll_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRunGroupsByArrAndFansOut(t *testing.T) {
	// 3 registered arrs; rows split across them.
	// Use a fake clock and a fake Radarr that records call order.
	// Assert that all three arrs see at least one call concurrently
	// (channel-based timing) and a slow arr does not block the others.
}
func TestRunSkipsRowsWithMissingOrDisabledArr(t *testing.T) {
	// row with routed_arr_id=NULL or arr.enabled=false → skipped, status unchanged
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `Run` and `scheduled.go`**

`scheduled.go` exposes the `scheduled_task.v1` capability via the SDK. The arrproxy file is the precedent — port it with the rename. The interesting piece is `Run`:

```go
// in internal/poll/poll.go

import "sync"

func (p *Poller) Run(ctx context.Context) error {
	d := p.deps()
	if d == nil { return nil }
	rows, err := d.Store.ListPollable(ctx)
	if err != nil { return err }
	byArr := groupByArr(rows)

	var wg sync.WaitGroup
	now := time.Now().UTC()
	for arrID, group := range byArr {
		a, err := d.Store.GetArr(ctx, arrID)
		if err != nil || a == nil || !a.Enabled {
			p.log.Debug("skipping group: arr missing/disabled", "arr_id", arrID)
			continue
		}
		apiKey, err := crypto.Open(d.SecretKey, a.APIKey)
		if err != nil {
			p.log.Warn("decrypt api_key failed", "arr_id", arrID, "err", err)
			continue
		}
		wg.Add(1)
		go func(a *store.RegisteredArr, apiKey string, group []*store.Request) {
			defer wg.Done()
			for _, r := range group {
				p.pollOne(ctx, d, r, a, apiKey, now)
			}
		}(a, apiKey, group)
	}
	wg.Wait()
	return nil
}

func groupByArr(rows []*store.Request) map[int64][]*store.Request {
	out := map[int64][]*store.Request{}
	for _, r := range rows {
		if r.RoutedArrID == nil { continue }
		out[*r.RoutedArrID] = append(out[*r.RoutedArrID], r)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/poll
git commit -m "feat(poll): per-arr fanout + scheduled task binding"
```

---

## Phase 9 — Admin HTTP API

### Task 9.1: Server skeleton + admin guard

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/auth/identity.go` (port from arrproxy verbatim)
- Create: `internal/server/handlers_test.go`

- [ ] **Step 1: Port the auth/identity helper**

```bash
cp /opt/continuum-plugin-arrproxy/internal/auth/identity.go internal/auth/identity.go
sed -i 's|continuum-plugin-arrproxy|continuum-plugin-arrouter|g' internal/auth/identity.go
```

- [ ] **Step 2: Write the failing test for the admin guard**

```go
package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/server"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

func TestRequireAdminBlocksNonAdmin(t *testing.T) {
	h := server.New(&server.Deps{Store: &store.Store{}}).Handler()
	req := httptest.NewRequest("GET", "/api/admin/registry", nil)
	req.Header.Set("X-Continuum-User-Role", "user")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden { t.Fatalf("got %d", rr.Code) }
}

func TestRequireAdminAllowsAdmin(t *testing.T) {
	// admin role + minimal Deps → 200 (or 500 from missing fixture is fine,
	// just not 403)
}
```

- [ ] **Step 3: Run to verify failure**

- [ ] **Step 4: Implement `server.go`**

```go
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/auth"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/event"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/poll"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

type Deps struct {
	Store     *store.Store
	Enricher  routing.Enricher
	Events    *event.Publisher
	Poll      *poll.Poller
	SecretKey string
	WebFS     http.FileSystem // dist/ for the SPA + assets
}

type Server struct{ deps *Deps }

func New(d *Deps) *Server { return &Server{deps: d} }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Route("/api/admin", func(r chi.Router) {
		r.Use(requireAdmin)
		r.Route("/registry", s.registryRoutes)
		r.Route("/requests", s.requestsRoutes)
		r.Post("/route-test", s.handleRouteTest)
	})

	// SPA + assets — see Task 10.2 for theme injection on the prerender
	r.Get("/admin/*", s.handleSPA)
	r.Get("/assets/*", http.FileServer(s.deps.WebFS).ServeHTTP)
	return r
}

func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := auth.FromRequest(r)
		if !id.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 5: Run tests to verify pass**

- [ ] **Step 6: Commit**

```bash
git add internal/auth internal/server
git commit -m "feat(server): admin-guarded chi router skeleton"
```

### Task 9.2: Registry CRUD endpoints

**Files:**
- Create: `internal/server/registry_handlers.go`
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestRegistryListReturnsRowsOrderedByKindPriority(t *testing.T) {}
func TestRegistryCreateValidatesRulesAndEncryptsAPIKey(t *testing.T) {}
func TestRegistryCreateRejectsBadKind(t *testing.T)                  {}
func TestRegistryCreateRejectsInvalidRules(t *testing.T)             {}
func TestRegistryUpdatePartialFields(t *testing.T)                   {}
func TestRegistryUpdateRotatesAPIKeyOnlyWhenProvided(t *testing.T)   {}
func TestRegistryDeleteHardDeletesButPreservesRequests(t *testing.T) {}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement handlers**

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/crypto"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/routing"
	"github.com/ContinuumApp/continuum-plugin-arrouter/internal/store"
)

type registryDTO struct {
	ID                int64           `json:"id,omitempty"`
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	URL               string          `json:"url"`
	APIKey            string          `json:"api_key,omitempty"` // write-only; never emitted on read
	HasAPIKey         bool            `json:"has_api_key,omitempty"` // read-only
	RootFolderPath    string          `json:"root_folder_path"`
	QualityProfileID  *int            `json:"quality_profile_id,omitempty"`
	LanguageProfileID *int            `json:"language_profile_id,omitempty"`
	Priority          int             `json:"priority"`
	Enabled           bool            `json:"enabled"`
	RulesJSON         json.RawMessage `json:"rules"`
}

func (s *Server) registryRoutes(r chi.Router) {
	r.Get("/", s.handleRegistryList)
	r.Post("/", s.handleRegistryCreate)
	r.Get("/{id}", s.handleRegistryGet)
	r.Patch("/{id}", s.handleRegistryUpdate)
	r.Delete("/{id}", s.handleRegistryDelete)
	r.Post("/{id}/test-connection", s.handleRegistryTestConnection)
}

func (s *Server) handleRegistryList(w http.ResponseWriter, r *http.Request) { /* … */ }

func (s *Server) handleRegistryCreate(w http.ResponseWriter, r *http.Request) {
	var dto registryDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "bad json", 400); return
	}
	if dto.Kind != "radarr" && dto.Kind != "sonarr" {
		http.Error(w, "bad kind", 400); return
	}
	rules, err := routing.ParseRules(dto.RulesJSON)
	if err != nil { http.Error(w, err.Error(), 400); return }
	if err := routing.ValidateRules(rules); err != nil { http.Error(w, err.Error(), 400); return }

	sealed, err := crypto.Seal(s.deps.SecretKey, dto.APIKey)
	if err != nil { http.Error(w, "seal: "+err.Error(), 500); return }

	a := &store.RegisteredArr{
		Name: dto.Name, Kind: dto.Kind, URL: dto.URL, APIKey: sealed,
		RootFolderPath: dto.RootFolderPath,
		QualityProfileID: dto.QualityProfileID, LanguageProfileID: dto.LanguageProfileID,
		Priority: dto.Priority, Enabled: dto.Enabled, RulesJSON: dto.RulesJSON,
	}
	id, err := s.deps.Store.CreateArr(r.Context(), a)
	if err != nil { http.Error(w, err.Error(), 500); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

// PATCH: only update fields that are present in the body. APIKey is rotated
// only when non-empty in the input.
func (s *Server) handleRegistryUpdate(w http.ResponseWriter, r *http.Request) { /* … */ }

func (s *Server) handleRegistryDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.deps.Store.DeleteArr(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500); return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRegistryGet(w http.ResponseWriter, r *http.Request) { /* read-only DTO; HasAPIKey=true if APIKey non-empty */ }

var ErrNotFound = errors.New("not found")
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat(server): registry CRUD endpoints"
```

### Task 9.3: Test-connection endpoint

**Files:**
- Modify: `internal/server/registry_handlers.go`
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestTestConnectionUsesPostedKeyNotStored(t *testing.T) {
	// optional `api_key` in body → use that for the GET /system/status
	// (so admins can test a new key before saving)
}
func TestTestConnectionFallsBackToStoredKey(t *testing.T) {
	// no api_key in body → decrypt the stored one
}
func TestTestConnectionReturnsInstanceVersion(t *testing.T) {}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement** — call `arr.SystemStatus(ctx, url, apiKey)` (port if needed from arrproxy; otherwise add a one-liner).

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat(server): registry test-connection endpoint"
```

### Task 9.4: Route-test endpoint

**Files:**
- Create: `internal/server/route_test_handler.go`
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRouteTestReturnsChosenAndTrace(t *testing.T) {
	// POST /api/admin/route-test {tmdbId, mediaType}
	// → {chosen: <id>|null, trace: {...}}
	// must NOT write any rows
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement**

```go
func (s *Server) handleRouteTest(w http.ResponseWriter, r *http.Request) {
	var in struct{ TMDBID int `json:"tmdbId"`; MediaType string `json:"mediaType"`; Title string `json:"title"`; Year int `json:"year"` }
	_ = json.NewDecoder(r.Body).Decode(&in)
	candidates, err := s.loadCandidates(r.Context(), in.MediaType)
	if err != nil { http.Error(w, err.Error(), 500); return }
	ev := routing.RequestEvent{TMDBID: in.TMDBID, MediaType: in.MediaType, Title: in.Title, Year: in.Year}
	chosen, trace := routing.Decide(r.Context(), candidates, ev, s.deps.Enricher)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"chosen": chosen, "trace": trace})
}

func (s *Server) loadCandidates(ctx context.Context, mediaType string) ([]routing.Candidate, error) {
	// duplicate of the consumer's loadCandidates — extract to a routing helper
	// in a follow-up task if it diverges.
	return nil, nil
}
```

(Extract `loadCandidates` to a shared `routing` helper to avoid duplication.)

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/server internal/routing
git commit -m "feat(server): /route-test endpoint with shared candidate loader"
```

### Task 9.5: Requests queue endpoints

**Files:**
- Create: `internal/server/requests_handlers.go`
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestRequestsListPaginatesAndFiltersByStatus(t *testing.T) {}
func TestRequestsGetReturnsTrace(t *testing.T)                  {}
func TestRetryRequeuesFailedRow(t *testing.T)                   { /* status was failed; after POST /retry it should re-run submit */ }
func TestRetryOnNonFailedRowIs400(t *testing.T)                 {}
func TestReRouteOnUnroutedRowReRunsRouting(t *testing.T)        { /* status was unrouted; route-pipeline runs again */ }
func TestReRouteOnNonUnroutedRowIs400(t *testing.T)             {}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement**

```go
func (s *Server) requestsRoutes(r chi.Router) {
	r.Get("/", s.handleRequestsList)
	r.Get("/{id}", s.handleRequestsGet)
	r.Post("/{id}/retry", s.handleRequestsRetry)
	r.Post("/{id}/re-route", s.handleRequestsReRoute)
}
```

`Retry` re-uses the consumer's `SubmitHandler` against the existing row's payload; `ReRoute` calls the same `routing.Decide` flow used by submit, but only after asserting the row is `unrouted`.

To avoid duplicating the consumer's submit body, extract a `Submit(ctx, ev, requestRow)` method on `consumer.SubmitHandler` that takes a pre-existing row and skips the initial `UpsertRequestQueued`.

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/server internal/consumer
git commit -m "feat(server): requests queue + retry/re-route"
```

---

## Phase 10 — Admin SPA

The SPA mirrors `continuum-plugin-requests/web/`. Many config files (vite, tsconfig, tailwind, postcss) can be copied verbatim.

### Task 10.1: Vite scaffold

**Files:**
- Create: `web/{package.json,tsconfig.json,tsconfig.node.json,vite.config.ts,tailwind.config.ts,postcss.config.js,index.html}`
- Create: `web/src/{main.tsx,App.tsx,index.css}`
- Create: `web/embed.go`

- [ ] **Step 1: Copy scaffold files from requests plugin**

```bash
cp /opt/continuum-plugin-requests/web/package.json web/
cp /opt/continuum-plugin-requests/web/tsconfig*.json web/
cp /opt/continuum-plugin-requests/web/vite.config.ts web/
cp /opt/continuum-plugin-requests/web/tailwind.config.ts web/
cp /opt/continuum-plugin-requests/web/postcss.config.js web/
cp /opt/continuum-plugin-requests/web/index.html web/
cp /opt/continuum-plugin-requests/web/embed.go web/
```

- [ ] **Step 2: Edit `web/package.json`**

Change `"name"` to `"continuum-plugin-arrouter-web"`. Keep deps; add nothing extra yet.

- [ ] **Step 3: Edit `web/embed.go`**

Update the package import path / comment to reference arrouter:

```go
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var embeddedFS embed.FS

func DistFS() http.FileSystem {
	sub, _ := fs.Sub(embeddedFS, "dist")
	return http.FS(sub)
}
```

- [ ] **Step 4: Create `web/src/main.tsx`**

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import "./index.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter basename="/admin">
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);
```

- [ ] **Step 5: Create `web/src/App.tsx` (stub)**

```tsx
import { Routes, Route, Navigate } from "react-router-dom";

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<div>Arrouter admin (stub)</div>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
```

- [ ] **Step 6: Create `web/src/index.css`**

Copy continuum's design-token base from `continuum-plugin-requests/web/src/index.css`. This pulls in all `--theme-*` custom properties so the page picks up whatever theme is set on `<html>`.

- [ ] **Step 7: Install + smoke build**

Run:
```bash
cd web && pnpm install && pnpm run build
```
Expected: produces `web/dist/` with `index.html` + `assets/`. Then `cd .. && go build ./...` succeeds (the embed picks up dist/).

- [ ] **Step 8: Commit**

```bash
git add web/
git commit -m "feat(web): vite/react scaffold with theme tokens"
```

### Task 10.2: Theme injection prerender

**Files:**
- Create: `internal/server/prerender_handler.go`
- Modify: `internal/server/handlers_test.go`

The plugin's SPA handler must inject `data-theme="<theme>"` onto `<html>` at request time. Resolve the theme from `?theme=…` (continuum's sidebar appends it) or from a request header continuum may inject. Fall back to a single neutral default only if none is present.

- [ ] **Step 1: Write the failing test**

```go
func TestPrerenderInjectsThemeFromQuery(t *testing.T) {
	srv := server.New(&server.Deps{WebFS: fakeFS{indexHTML: `<html><body></body></html>`}}).Handler()
	req := httptest.NewRequest("GET", "/admin/?theme=midnight-cinema", nil)
	req.Header.Set("X-Continuum-User-Role", "admin")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if !strings.Contains(rr.Body.String(), `<html data-theme="midnight-cinema">`) {
		t.Fatalf("missing theme: %s", rr.Body.String())
	}
}
func TestPrerenderInjectsThemeFromHeader(t *testing.T) {
	// X-Continuum-Theme header takes precedence over query if both present
}
func TestPrerenderFallsBackWhenAbsent(t *testing.T) {
	// no query/header → "default"
}
func TestPrerenderRewritesAssetURLs(t *testing.T) {
	// vite emits /assets/foo.js — when served under /plugins/{id}/admin we need
	// continuum's prefix preserved. The standard pattern (mirror requests plugin)
	// is to leave them as relative paths and trust continuum's reverse proxy.
}
```

- [ ] **Step 2: Run to verify failure**

- [ ] **Step 3: Implement `prerender_handler.go`**

```go
package server

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
)

var htmlTagRE = regexp.MustCompile(`(?i)<html(\s[^>]*)?>`)

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	f, err := s.deps.WebFS.Open("/index.html")
	if err != nil { http.Error(w, "spa missing", 500); return }
	defer f.Close()
	stat, _ := f.Stat()
	buf := make([]byte, stat.Size())
	_, _ = f.Read(buf)

	theme := r.Header.Get("X-Continuum-Theme")
	if theme == "" { theme = r.URL.Query().Get("theme") }
	if theme == "" { theme = "default" }

	out := htmlTagRE.ReplaceAll(buf, []byte(`<html data-theme="`+strings.ReplaceAll(theme, `"`, `&quot;`)+`">`))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store") // theme varies per request
	_, _ = w.Write(bytes.TrimSpace(out))
}
```

- [ ] **Step 4: Run tests to verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat(server): SPA prerender with theme injection"
```

### Task 10.3: API client + types

**Files:**
- Create: `web/src/api/types.ts`
- Create: `web/src/api/client.ts`

- [ ] **Step 1: Write `types.ts`**

```ts
export type Kind = "radarr" | "sonarr";
export type Combinator = "all" | "any";
export type Op = "eq"|"ne"|"in"|"not_in"|"gt"|"gte"|"lt"|"lte"|"between"|"contains"|"starts_with"|"regex";

export interface Rule { field: string; op: Op; value: unknown; }
export interface Group { match: Combinator; rules: Rule[]; }
export interface Rules { match: Combinator; groups: Group[]; }

export interface RegisteredArr {
  id: number;
  name: string;
  kind: Kind;
  url: string;
  has_api_key: boolean;
  root_folder_path: string;
  quality_profile_id?: number;
  language_profile_id?: number;
  priority: number;
  enabled: boolean;
  rules: Rules;
}

export type Status = "queued"|"submitted"|"downloading"|"imported"|"failed"|"cancelled"|"unrouted";

export interface RequestRow {
  id: string;
  tmdb_id: number;
  media_type: "movie"|"tv";
  title: string;
  year: number;
  poster_url?: string;
  status: Status;
  routed_arr_id?: number;
  routed_arr_name?: string;
  external_id?: number;
  error?: string;
  match_trace?: unknown;
  submitted_at?: string;
  last_polled_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface RouteTestResult { chosen: number | null; trace: unknown; }
```

- [ ] **Step 2: Write `client.ts`**

```ts
import type { RegisteredArr, RequestRow, RouteTestResult, Rules } from "./types";

const base = "/api/admin";

async function j<T>(r: Response): Promise<T> {
  if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
  return r.json();
}

export const api = {
  listArrs: () => fetch(`${base}/registry`).then(j<RegisteredArr[]>),
  getArr:   (id: number) => fetch(`${base}/registry/${id}`).then(j<RegisteredArr>),
  createArr:(input: Partial<RegisteredArr> & { api_key: string; rules: Rules }) =>
    fetch(`${base}/registry`, { method: "POST", body: JSON.stringify(input), headers: {"Content-Type":"application/json"} }).then(j<{id:number}>),
  updateArr:(id: number, patch: Partial<RegisteredArr> & { api_key?: string }) =>
    fetch(`${base}/registry/${id}`, { method: "PATCH", body: JSON.stringify(patch), headers: {"Content-Type":"application/json"} }).then(r => { if (!r.ok) throw new Error(r.statusText); }),
  deleteArr:(id: number) =>
    fetch(`${base}/registry/${id}`, { method: "DELETE" }).then(r => { if (!r.ok) throw new Error(r.statusText); }),
  testConnection:(id: number, api_key?: string) =>
    fetch(`${base}/registry/${id}/test-connection`, { method: "POST", body: JSON.stringify({api_key}), headers: {"Content-Type":"application/json"} }).then(j<{version: string; instanceName?: string}>),
  routeTest:(input: { tmdbId: number; mediaType: "movie"|"tv"; title?: string; year?: number }) =>
    fetch(`${base}/route-test`, { method: "POST", body: JSON.stringify(input), headers: {"Content-Type":"application/json"} }).then(j<RouteTestResult>),
  listRequests:(p: {status?: string; page?: number; limit?: number}) => {
    const q = new URLSearchParams();
    if (p.status) q.set("status", p.status);
    if (p.page)   q.set("page", String(p.page));
    if (p.limit)  q.set("limit", String(p.limit));
    return fetch(`${base}/requests?${q}`).then(j<{rows: RequestRow[]; total: number}>);
  },
  retry:    (id: string) => fetch(`${base}/requests/${id}/retry`, { method: "POST" }).then(r => { if (!r.ok) throw new Error(r.statusText); }),
  reRoute:  (id: string) => fetch(`${base}/requests/${id}/re-route`, { method: "POST" }).then(r => { if (!r.ok) throw new Error(r.statusText); }),
};
```

- [ ] **Step 3: Build**

Run: `cd web && pnpm run build`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add web/src/api
git commit -m "feat(web): api client + types"
```

### Task 10.4: Layout + routing

**Files:**
- Modify: `web/src/App.tsx`
- Create: `web/src/components/Layout.tsx`

- [ ] **Step 1: Write `Layout.tsx`**

A header with three nav links (Registry, New *arr, Queue). Tailwind classes match continuum's app shell — pull patterns from `continuum-plugin-requests/web/src/components/AppShell.tsx` if present.

- [ ] **Step 2: Wire routes in `App.tsx`**

```tsx
import { Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";
import RegistryListPage from "./pages/RegistryListPage";
import RegistryEditorPage from "./pages/RegistryEditorPage";
import RequestsQueuePage from "./pages/RequestsQueuePage";

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<RegistryListPage />} />
        <Route path="/registry/new" element={<RegistryEditorPage />} />
        <Route path="/registry/:id" element={<RegistryEditorPage />} />
        <Route path="/queue" element={<RequestsQueuePage />} />
      </Routes>
    </Layout>
  );
}
```

- [ ] **Step 3: Stub the three pages** so the build passes

`web/src/pages/{RegistryListPage,RegistryEditorPage,RequestsQueuePage}.tsx` each export a default component returning a `<div>{name} (stub)</div>`. They get filled in over the next tasks.

- [ ] **Step 4: Build**

Run: `cd web && pnpm run build`

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): app shell + page routing"
```

### Task 10.5: Registry list page

**Files:**
- Modify: `web/src/pages/RegistryListPage.tsx`
- Create: `web/src/components/RegistryTable.tsx`

- [ ] **Step 1: Implement `RegistryTable.tsx`**

Columns: name, kind pill, url (truncated), priority, enabled toggle, actions (edit / delete / test connection). Uses `api.listArrs` on mount and `api.deleteArr` / `api.updateArr` for inline actions.

- [ ] **Step 2: Implement `RegistryListPage.tsx`**

Wraps the table. Includes a "Add registered *arr" button linking to `/registry/new`.

- [ ] **Step 3: Build + manually open in dev**

```bash
cd web && pnpm run dev
```
Visit `http://localhost:5173/admin/`. Confirm the page renders with empty state copy when no arrs exist.

- [ ] **Step 4: Commit**

```bash
git add web/src
git commit -m "feat(web): registry list page"
```

### Task 10.6: Registry editor — connection panel

**Files:**
- Create: `web/src/components/ConnectionPanel.tsx`
- Modify: `web/src/pages/RegistryEditorPage.tsx`

- [ ] **Step 1: Implement `ConnectionPanel.tsx`**

Form fields: name, kind (radio movie/tv), url, api_key (masked input with "rotate" affordance on edit), root_folder_path, quality_profile_id, language_profile_id, priority, enabled. Includes a "Test connection" button calling `api.testConnection`.

- [ ] **Step 2: Wire into `RegistryEditorPage.tsx`**

For `/registry/new`: blank form. For `/registry/:id`: load via `api.getArr`, prefill, submit via `api.updateArr` with only changed fields. The CollectionBuilder component (Task 10.8) is rendered below the connection panel.

- [ ] **Step 3: Manual smoke check** in dev mode — create an *arr, edit it, delete it.

- [ ] **Step 4: Commit**

```bash
git add web/src
git commit -m "feat(web): registry editor connection panel"
```

### Task 10.7: Port CollectionBuilder

**Files:**
- Create: `web/src/components/CollectionBuilder.tsx`
- Create: `web/src/components/CollectionBuilder.test.tsx`

- [ ] **Step 1: Copy from continuum**

```bash
cp /opt/continuum/web/src/components/collections/CollectionBuilder.tsx web/src/components/
cp /opt/continuum/web/src/components/collections/CollectionBuilder.test.tsx web/src/components/
```

- [ ] **Step 2: Strip everything CollectionBuilder uses that arrouter doesn't need**

Remove from the imported component:
- Sort controls and `QuerySort` references
- `limit` field
- `media_scope` toggle
- `library_ids` picker
- The "preview matches" panel (replaced by Test rules)

Update the helper imports — `createEmptyQueryDefinition` / `normalizeQueryDefinition` are tied to continuum's full schema. Replace with local helpers in `web/src/lib/rules.ts`:

```ts
import type { Rules } from "../api/types";

export function emptyRules(): Rules { return { match: "all", groups: [] }; }
export function normalizeRules(input: unknown): Rules {
  // tolerant parse: missing match → "all"; missing groups → []
  const r = (input ?? {}) as Partial<Rules>;
  return {
    match: r.match === "any" ? "any" : "all",
    groups: Array.isArray(r.groups) ? r.groups.map(g => ({
      match: g.match === "any" ? "any" : "all",
      rules: Array.isArray(g.rules) ? g.rules : [],
    })) : [],
  };
}
```

- [ ] **Step 3: Strip the field catalog dependency**

Note where the original references continuum's field list / operator restrictions — leave a TODO comment. Task 10.8 fills in arrouter's vocabulary.

- [ ] **Step 4: Run the test (it will mostly fail; that's fine for now)**

Run: `cd web && pnpm run test`
Expected: at least the test file compiles. Failing assertions are OK at this checkpoint.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): port CollectionBuilder skeleton (vocab pending)"
```

### Task 10.8: Adapt CollectionBuilder field catalog

**Files:**
- Create: `web/src/lib/fieldCatalog.ts`
- Modify: `web/src/components/CollectionBuilder.tsx`
- Modify: `web/src/components/CollectionBuilder.test.tsx`

- [ ] **Step 1: Define the catalog**

```ts
// web/src/lib/fieldCatalog.ts
import type { Op, Kind } from "../api/types";

export type FieldType = "string"|"number"|"bool"|"date"|"string_array";
export type FieldGroup = "A"|"B"|"C-keywords"|"C-content_rating";

export interface FieldDef {
  name: string;
  label: string;
  type: FieldType;
  group: FieldGroup;
  ops: Op[];
  kinds: Kind[]; // restricted to these arr kinds; empty = both
}

const STRING_OPS: Op[] = ["eq","ne","in","not_in","contains","starts_with","regex"];
const NUMBER_OPS: Op[] = ["eq","ne","in","not_in","gt","gte","lt","lte","between"];
const BOOL_OPS:   Op[] = ["eq","ne"];
const DATE_OPS:   Op[] = ["eq","ne","gt","gte","lt","lte","between"];
const ARRAY_OPS:  Op[] = ["contains","in","not_in"];

export const FIELD_CATALOG: FieldDef[] = [
  // Group A
  {name:"mediaType",        label:"Media type",         type:"string", group:"A", ops:["eq","ne","in","not_in"], kinds:[]},
  {name:"libraryId",        label:"Library id",         type:"string", group:"A", ops:STRING_OPS, kinds:[]},
  {name:"year",             label:"Year",               type:"number", group:"A", ops:NUMBER_OPS, kinds:[]},
  {name:"decade",           label:"Decade",             type:"number", group:"A", ops:NUMBER_OPS, kinds:[]},
  {name:"requesterUserId",  label:"Requester user id",  type:"string", group:"A", ops:STRING_OPS, kinds:[]},
  {name:"requesterIsAdmin", label:"Requester is admin", type:"bool",   group:"A", ops:BOOL_OPS,   kinds:[]},
  {name:"title",            label:"Title",              type:"string", group:"A", ops:STRING_OPS, kinds:[]},
  {name:"tmdbId",           label:"TMDB id",            type:"number", group:"A", ops:NUMBER_OPS, kinds:[]},
  // Group B (common)
  {name:"original_language", label:"Original language", type:"string", group:"B", ops:["eq","ne","in","not_in"], kinds:[]},
  {name:"original_title",    label:"Original title",    type:"string", group:"B", ops:STRING_OPS, kinds:[]},
  {name:"genres",            label:"Genres",            type:"string_array", group:"B", ops:ARRAY_OPS, kinds:[]},
  {name:"runtime",           label:"Runtime (min)",     type:"number", group:"B", ops:NUMBER_OPS, kinds:[]},
  {name:"vote_average",      label:"Vote average",      type:"number", group:"B", ops:NUMBER_OPS, kinds:[]},
  {name:"vote_count",        label:"Vote count",        type:"number", group:"B", ops:NUMBER_OPS, kinds:[]},
  {name:"popularity",        label:"Popularity",        type:"number", group:"B", ops:NUMBER_OPS, kinds:[]},
  {name:"adult",             label:"Adult",             type:"bool",   group:"B", ops:BOOL_OPS, kinds:[]},
  {name:"status",            label:"Status",            type:"string", group:"B", ops:["eq","ne","in","not_in"], kinds:[]},
  {name:"production_companies", label:"Production companies", type:"string_array", group:"B", ops:ARRAY_OPS, kinds:[]},
  {name:"production_countries", label:"Production countries", type:"string_array", group:"B", ops:ARRAY_OPS, kinds:[]},
  {name:"spoken_languages",   label:"Spoken languages",  type:"string_array", group:"B", ops:ARRAY_OPS, kinds:[]},
  // Group B (movie-only)
  {name:"release_date",         label:"Release date",         type:"date",   group:"B", ops:DATE_OPS,   kinds:["radarr"]},
  {name:"budget",               label:"Budget (USD)",         type:"number", group:"B", ops:NUMBER_OPS, kinds:["radarr"]},
  {name:"revenue",              label:"Revenue (USD)",        type:"number", group:"B", ops:NUMBER_OPS, kinds:["radarr"]},
  {name:"belongs_to_collection",label:"Belongs to collection",type:"string", group:"B", ops:STRING_OPS, kinds:["radarr"]},
  {name:"imdb_id",              label:"IMDB id",              type:"string", group:"B", ops:["eq","ne","in","not_in"], kinds:["radarr"]},
  // Group B (tv-only)
  {name:"networks",         label:"Networks",         type:"string_array", group:"B", ops:ARRAY_OPS, kinds:["sonarr"]},
  {name:"origin_country",   label:"Origin country",   type:"string_array", group:"B", ops:ARRAY_OPS, kinds:["sonarr"]},
  {name:"first_air_date",   label:"First air date",   type:"date",   group:"B", ops:DATE_OPS,   kinds:["sonarr"]},
  {name:"last_air_date",    label:"Last air date",    type:"date",   group:"B", ops:DATE_OPS,   kinds:["sonarr"]},
  {name:"type",             label:"Type",             type:"string", group:"B", ops:["eq","ne","in","not_in"], kinds:["sonarr"]},
  {name:"in_production",    label:"In production",    type:"bool",   group:"B", ops:BOOL_OPS, kinds:["sonarr"]},
  {name:"number_of_seasons",label:"Number of seasons",type:"number", group:"B", ops:NUMBER_OPS, kinds:["sonarr"]},
  {name:"number_of_episodes",label:"Number of episodes",type:"number", group:"B", ops:NUMBER_OPS, kinds:["sonarr"]},
  {name:"created_by",       label:"Created by",       type:"string_array", group:"B", ops:ARRAY_OPS, kinds:["sonarr"]},
  // Group C
  {name:"keywords",       label:"Keywords (extra TMDB call)",       type:"string_array", group:"C-keywords",       ops:ARRAY_OPS, kinds:[]},
  {name:"content_rating", label:"Content rating (extra TMDB call)", type:"string",       group:"C-content_rating", ops:["eq","ne","in","not_in"], kinds:[]},
];

export function fieldsForKind(kind: Kind): FieldDef[] {
  return FIELD_CATALOG.filter(f => f.kinds.length === 0 || f.kinds.includes(kind));
}
```

- [ ] **Step 2: Wire `FIELD_CATALOG` into the rule editor**

Replace any continuum-specific field-list import in `CollectionBuilder.tsx` with `import { fieldsForKind } from "../lib/fieldCatalog";`. The component should accept a `kind: Kind` prop and call `fieldsForKind(kind)` to drive the field picker.

The picker groups by `field.group` and adds the "(extra TMDB call)" hint already encoded in the label for Group C entries.

Per-field operator restrictions are read from `field.ops`.

- [ ] **Step 3: Update / write tests**

Cover at minimum:
- Picker for `kind="radarr"` excludes `networks`, `origin_country`, etc.
- Picker for `kind="sonarr"` excludes `release_date`, `budget`, etc.
- Selecting a numeric field hides string-only operators.
- Selecting `regex` op shows a free-text input.
- Selecting `between` op shows two value inputs.

- [ ] **Step 4: Run tests**

Run: `cd web && pnpm run test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): wire arrouter field catalog into CollectionBuilder"
```

### Task 10.9: Test rules panel

**Files:**
- Create: `web/src/components/RuleTestPanel.tsx`
- Modify: `web/src/pages/RegistryEditorPage.tsx`

- [ ] **Step 1: Implement `RuleTestPanel.tsx`**

Inputs: TMDB id, media type. Button "Test routing". On submit, calls `api.routeTest` and renders the response trace as a collapsible per-arr summary.

- [ ] **Step 2: Embed under the rule builder in the editor page**

- [ ] **Step 3: Manual smoke check** with a real registry row.

- [ ] **Step 4: Commit**

```bash
git add web/src
git commit -m "feat(web): rule test panel using /route-test"
```

### Task 10.10: Requests queue page

**Files:**
- Modify: `web/src/pages/RequestsQueuePage.tsx`
- Create: `web/src/components/RequestsQueue.tsx`
- Create: `web/src/components/StatusPill.tsx`

- [ ] **Step 1: Implement `StatusPill.tsx`**

A thin wrapper that maps `Status` → Tailwind color classes (`queued`/`submitted`: blue; `downloading`: indigo; `imported`: green; `failed`/`unrouted`: red; `cancelled`: gray).

- [ ] **Step 2: Implement `RequestsQueue.tsx`**

Paginated table: id (linked), title (year), kind pill, status pill, routed_arr_name, error preview, actions. Actions: "Retry" (for `failed`), "Re-route" (for `unrouted`). Calls `api.retry` / `api.reRoute` and refetches on success.

- [ ] **Step 3: Implement `RequestsQueuePage.tsx`** as a thin wrapper with status filter dropdown.

- [ ] **Step 4: Manual smoke check.**

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): requests queue page with retry/re-route"
```

### Task 10.11: Verify dist/ is embedded into the binary

- [ ] **Step 1: Build everything end-to-end**

```bash
cd web && pnpm run build && cd ..
go build ./cmd/continuum-plugin-arrouter
```

- [ ] **Step 2: Confirm assets are inside the binary**

```bash
strings continuum-plugin-arrouter | grep 'theme tokens placeholder text from index.css' | head -1
```

(Pick a unique short string from `index.css` to grep for.) Expected: a hit, proving the embed works.

- [ ] **Step 3: Commit (no code change — just a checkpoint)**

```bash
git commit --allow-empty -m "checkpoint: SPA dist embedded into binary"
```

---

## Phase 11 — Wire main.go + finalize manifest

### Task 11.1: Configure global_config_schema

**Files:**
- Modify: `cmd/continuum-plugin-arrouter/manifest.json`

- [ ] **Step 1: Add the global config keys from the spec**

Insert into `"global_config_schema"`:

```json
[
  {"key":"database_url",          "title":"Postgres connection string", "json_schema":"{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\"}},\"required\":[\"value\"]}", "required":true,  "secret":false},
  {"key":"tmdb.api_key",           "title":"TMDB API key",                "json_schema":"{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\"}},\"required\":[\"value\"]}", "required":true,  "secret":true },
  {"key":"tmdb.language",          "title":"TMDB language",               "json_schema":"{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\"}}}", "required":false},
  {"key":"poll_interval_seconds",  "title":"Poll interval (seconds)",     "json_schema":"{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"integer\",\"minimum\":10,\"maximum\":600}}}", "required":false},
  {"key":"stale_after_hours",      "title":"Stale after (hours)",         "json_schema":"{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"integer\",\"minimum\":1}}}", "required":false},
  {"key":"secret_key",             "title":"Secret encryption key",        "json_schema":"{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\",\"minLength\":16}},\"required\":[\"value\"]}", "required":true, "secret":true}
]
```

(Match the exact field names continuum's manifest schema uses — copy from `continuum-plugin-arrproxy/cmd/continuum-plugin-arrproxy/manifest.json` for the precise `admin_form` convention.)

- [ ] **Step 2: Commit**

```bash
git add cmd/continuum-plugin-arrouter/manifest.json
git commit -m "chore(manifest): add global_config_schema"
```

### Task 11.2: Runtime config loader

**Files:**
- Create: `internal/runtime/runtime.go`

- [ ] **Step 1: Port arrproxy's runtime.go**

```bash
cp /opt/continuum-plugin-arrproxy/internal/runtime/runtime.go internal/runtime/runtime.go
sed -i 's|continuum-plugin-arrproxy|continuum-plugin-arrouter|g' internal/runtime/runtime.go
```

- [ ] **Step 2: Extend `Config` with arrouter-specific fields**

```go
type Config struct {
	DatabaseURL       string
	TMDBAPIKey        string
	TMDBLanguage      string
	PollIntervalSecs  int
	StaleAfterHours   int
	SecretKey         string
}
```

Add a `LoadConfig(globals map[string]any) (Config, error)` that pulls these from the host-supplied global config map. Defaults: `PollIntervalSecs=30`, `StaleAfterHours=72`, `TMDBLanguage="en-US"`. `DatabaseURL`, `TMDBAPIKey`, `SecretKey` are required.

- [ ] **Step 3: Test**

Add unit tests covering required-field validation and defaults.

- [ ] **Step 4: Commit**

```bash
git add internal/runtime
git commit -m "feat(runtime): config loader with required + default fields"
```

### Task 11.3: Wire main.go

**Files:**
- Modify: `cmd/continuum-plugin-arrouter/main.go`

- [ ] **Step 1: Port arrproxy's main.go**

```bash
cp /opt/continuum-plugin-arrproxy/cmd/continuum-plugin-arrproxy/main.go cmd/continuum-plugin-arrouter/main.go
sed -i 's|continuum-plugin-arrproxy|continuum-plugin-arrouter|g' cmd/continuum-plugin-arrouter/main.go
```

- [ ] **Step 2: Adapt for arrouter's wider surface**

The arrproxy version creates `radarrPtr`, `sonarrPtr` as singletons. Arrouter needs *factories* (per-row clients), so replace those atomic.Pointers with closures. Also wire:
- `tmdb.New` + `tmdb.NewCache` → `routing.Enricher`
- `consumer.SubmitHandler{...}` and `consumer.CancelHandler{...}` constructed from store + factories + publisher
- `consumer.New(submitH, cancelH, log.Named("consumer"))` registered as the `event_consumer.v1` handler
- `poll.New(...)` registered as the `scheduled_task.v1` handler
- `server.New(&server.Deps{... WebFS: web.DistFS()})` registered as the `http_routes.v1` handler

Sketch:

```go
// after pgxpool init + migrate
sec := cfg.SecretKey
tmdbClient := tmdb.New("https://api.themoviedb.org/3", cfg.TMDBAPIKey, cfg.TMDBLanguage)
enricher   := tmdb.NewCache(tmdbClient, 24*time.Hour)
publisher  := event.New(host, "continuum.arrouter", logger.Named("event"))

radarrFactory := func(url, key string) *arr.Radarr { return arr.NewRadarr(url, key, http.DefaultClient) }
sonarrFactory := func(url, key string) *arr.Sonarr { return arr.NewSonarr(url, key, http.DefaultClient) }

submitH := &consumer.SubmitHandler{Store: storePtr, Enricher: enricher, Radarr: radarrFactory, Sonarr: sonarrFactory, Events: publisher, SecretKey: sec, Log: logger.Named("submit")}
cancelH := &consumer.CancelHandler{Store: storePtr, Radarr: radarrFactory, Sonarr: sonarrFactory, Events: publisher, SecretKey: sec, Log: logger.Named("cancel")}
disp    := consumer.New(submitH, cancelH, logger.Named("consumer"))

pollerDeps := func() *poll.Deps { return &poll.Deps{Store: storePtr, Radarr: radarrFactory, Sonarr: sonarrFactory, Events: publisher, StaleAfterHours: cfg.StaleAfterHours, SecretKey: sec} }
poller     := poll.New(pollerDeps, logger.Named("poll"))

httpDeps := &server.Deps{Store: storePtr, Enricher: enricher, Events: publisher, Poll: poller, SecretKey: sec, WebFS: web.DistFS()}
httpSrv  := server.New(httpDeps)

// register capabilities with the SDK runtime
```

The exact SDK registration calls (e.g. `rt.RegisterEventConsumer(disp)`, `rt.RegisterScheduled(poller)`, `rt.RegisterHTTPHandler(httpSrv.Handler())`) should match how arrproxy already wires its capabilities — copy the registration block verbatim, just swap the handler types.

- [ ] **Step 3: Build + run against the dev compose stack**

```bash
make build
docker compose up -d postgres
psql "$DATABASE_URL" -c "CREATE ROLE plugin_arrouter LOGIN PASSWORD 'pw'; CREATE SCHEMA arrouter AUTHORIZATION plugin_arrouter;"
./continuum-plugin-arrouter
```

(Or attach via continuum's plugin host as you would for any other plugin — see continuum's own docs for the local-install procedure.)

- [ ] **Step 4: Smoke test end-to-end**

Manual checklist:
1. Open `/plugins/{installation_id}/admin/`. Theme matches continuum's active theme.
2. Add a Radarr registry row with empty rules. Test connection succeeds.
3. Submit a request via continuum.requests. Check that the row appears in the Queue page with status `submitted`.
4. Wait for the poll cycle. Status advances to `downloading` then `imported`.
5. Add a second Radarr row with priority=1 and a rule that doesn't match. Submit a new request. Confirm it routed to the priority=100 catch-all.
6. Add a rule using `keywords` field. Trigger a request. Confirm exactly one secondary TMDB call was made (use `nginx`-style logging or observe the cache hits in logs).

- [ ] **Step 5: Commit**

```bash
git add cmd/continuum-plugin-arrouter/main.go
git commit -m "feat: wire main.go orchestrator"
```

### Task 11.4: README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write README**

Sections: what it does, install (operator must pre-create role + schema; example commands), config (link to manifest), how rules work (link to SPEC), comparison with `continuum.arrproxy`. Mirror `continuum-plugin-arrproxy/README.md` shape.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README"
```

---

## Phase 12 — `continuum-plugin-requests` delta

**This phase happens in the `continuum-plugin-requests` repo**, not `continuum-plugin-arrouter`. Switch directories before starting:

```bash
cd /opt/continuum-plugin-requests
```

### Task 12.1: Extend manifest subscriptions

**Files:**
- Modify: `cmd/continuum-plugin-requests/manifest.json`

- [ ] **Step 1: Add arrouter subscriptions**

In the existing `event_consumer.v1` capability's `subscriptions` array, append:

```json
"plugin.continuum.arrouter.submitted",
"plugin.continuum.arrouter.downloading",
"plugin.continuum.arrouter.imported",
"plugin.continuum.arrouter.failed",
"plugin.continuum.arrouter.cancelled",
"plugin.continuum.arrouter.unrouted"
```

- [ ] **Step 2: Bump plugin version**

`"version"` → `"0.2.0"` (or whatever the next minor is — check current).

- [ ] **Step 3: Commit**

```bash
git add cmd/continuum-plugin-requests/manifest.json
git commit -m "feat(manifest): subscribe to plugin.continuum.arrouter.* events"
```

### Task 12.2: Rename Arrproxy* handlers to Router*

**Files:**
- Modify: `internal/fulfill/watcher.go`
- Modify: `internal/fulfill/watcher_test.go`

- [ ] **Step 1: Mechanically rename**

```bash
sed -i 's/handleArrproxy/handleRouter/g' internal/fulfill/watcher.go internal/fulfill/watcher_test.go
```

Update the comment header at the top of `watcher.go` — currently mentions "arrproxy" specifically; change to "router (arrproxy or arrouter)".

- [ ] **Step 2: Run existing tests**

Run: `go test ./internal/fulfill/...`
Expected: PASS — only the names changed.

- [ ] **Step 3: Commit**

```bash
git add internal/fulfill
git commit -m "refactor(fulfill): rename Arrproxy* handlers to Router*"
```

### Task 12.3: Map both prefixes through the renamed handlers

**Files:**
- Modify: `internal/fulfill/watcher.go`

- [ ] **Step 1: Update the dispatch switch**

Replace the existing `switch` block (around `watcher.go:58`) with:

```go
switch eventName {
case "plugin.continuum.arrproxy.downloading", "plugin.continuum.arrouter.downloading":
    w.handleRouterDownloading(ctx, d, p)
case "plugin.continuum.arrproxy.failed", "plugin.continuum.arrouter.failed":
    w.handleRouterFailed(ctx, d, p)
case "plugin.continuum.arrouter.unrouted":
    // map unrouted → failed, with a synthesized reason
    if reason, _ := p["reason"].(string); reason != "" {
        p["error"] = "no registered *arr matched: " + reason
    } else {
        p["error"] = "no registered *arr matched"
    }
    w.handleRouterFailed(ctx, d, p)
case "plugin.continuum.arrproxy.submitted", "plugin.continuum.arrproxy.imported", "plugin.continuum.arrproxy.cancelled",
     "plugin.continuum.arrouter.submitted",  "plugin.continuum.arrouter.imported",  "plugin.continuum.arrouter.cancelled":
    // no-op; library scanner / explicit cancel UI handles state transitions
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`

- [ ] **Step 3: Commit**

```bash
git add internal/fulfill
git commit -m "feat(fulfill): handle arrouter.* events alongside arrproxy.*"
```

### Task 12.4: Tests for both prefixes + unrouted

**Files:**
- Modify: `internal/fulfill/watcher_test.go`

- [ ] **Step 1: Add test cases**

```go
func TestArrouterDownloadingTriggersHandler(t *testing.T) {
    // mirror the existing arrproxy.downloading test, just with the new prefix
}
func TestArrouterUnroutedSurfacesAsFailedWithReason(t *testing.T) {
    // payload {requestId, reason="no registered *arr matched"}
    // → MediaRequest moves to failed, error column carries the reason
}
func TestArrproxyDownloadingStillWorks(t *testing.T) {
    // regression test — ensure the rename didn't break the arrproxy path
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/fulfill/... -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/fulfill
git commit -m "test(fulfill): cover arrouter prefixes + unrouted mapping"
```

### Task 12.5: Cross-repo smoke test

- [ ] **Step 1: Build both plugins and install in continuum**

Build the arrouter binary (Repo A) and the requests binary (Repo B). Install both via continuum's plugin manager.

- [ ] **Step 2: Run end-to-end**

1. Submit a request via the requests UI.
2. Confirm arrouter routes it.
3. Confirm the requests UI shows the correct downloading → imported transitions (event chain works).
4. Trigger an `unrouted` case (configure all registered *arrs with rules that won't match the request). Confirm the requests UI shows the request as `failed` with the synthesized reason.

- [ ] **Step 3: No commit needed** — manual verification step.

---

## End-of-plan checklist

- [ ] All phases committed in their own repo.
- [ ] `make test` passes in `continuum-plugin-arrouter`.
- [ ] `go test ./...` passes in `continuum-plugin-requests`.
- [ ] `cd web && pnpm run lint && pnpm run test && pnpm run build` clean in arrouter.
- [ ] End-to-end smoke (Phase 12 Task 5) passes.
- [ ] Memory note updated: `/root/.claude/projects/-opt/memory/project_arrouter_design.md` flipped from "spec not yet written" to "implementation complete" (or a follow-up note for any deferred items).

