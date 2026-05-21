# Arr registry

This document covers the operational details of `registered_arr` rows and the
admin API that manages them: API-key encryption, the `secret_key` rotation
procedure, profile resolution, and the connection-test endpoint. For routing
semantics see [`routing-rules.md`](routing-rules.md); for the request side of
the world see [`request-lifecycle.md`](request-lifecycle.md).

## Schema

```sql
CREATE TABLE registered_arr (
  id                  BIGSERIAL PRIMARY KEY,
  name                TEXT NOT NULL,
  kind                TEXT NOT NULL CHECK (kind IN ('radarr','sonarr')),
  url                 TEXT NOT NULL,
  api_key             TEXT NOT NULL,  -- base64(nonce || ciphertext || tag), AES-256-GCM
  root_folder_path    TEXT NOT NULL,  -- may be empty; resolver fills in
  quality_profile_id  INTEGER,        -- may be NULL; resolver fills in
  language_profile_id INTEGER,        -- Sonarr only; may be NULL
  priority            INTEGER NOT NULL DEFAULT 100,
  enabled             BOOLEAN NOT NULL DEFAULT true,
  rules_json          JSONB NOT NULL DEFAULT '{"match":"all","groups":[]}'::jsonb,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX registered_arr_kind_priority_idx
  ON registered_arr (kind, priority) WHERE enabled;
```

`request.routed_arr_id REFERENCES registered_arr(id) ON DELETE SET NULL` — a
deleted arr does not cascade-delete its requests, it orphans them. See the
"Deleting an arr" section below.

## API key encryption

The store column holds ciphertext only. The Go side (`internal/crypto`):

- **Algorithm:** AES-256-GCM via `crypto/cipher.NewGCM`.
- **Key derivation:** `key32 = SHA-256(secret_key_string)`. Any non-empty
  UTF-8 string is acceptable as `secret_key`; the SHA-256 is what becomes the
  32-byte AES key. The auto-generated default is 32 random bytes, base64-
  encoded (well over the entropy needed; harmless).
- **Nonce:** 12 random bytes per encryption, read from `crypto/rand`.
- **Wire format:** `base64(nonce ‖ ciphertext ‖ auth_tag)`. The base64 is
  stored as-is in `api_key`.
- **Validation:** `Open` returns an error if the key is wrong, the base64
  is malformed, or the auth tag fails. `Seal`/`Open` never panic.

Every write of an API key (`crypto.Seal`) is invoked from the admin server,
never from a hot path. Every read (`crypto.Open`) is on the hot path —
submit, cancel, poll, test-connection, and health probes all call it.

## Where decrypts happen

| Caller | Effect on failure |
| --- | --- |
| `consumer.SubmitHandler` | Returns error from `Submit`; request stays in `queued`. The event consumer logs the error; the host may re-deliver. |
| `consumer.CancelHandler` | Logs `cancel: decrypt api_key` and skips the arr DELETE. `MarkCancelled` still runs. |
| `poll.Run` | Logs `decrypt api_key failed` and skips the entire group of rows routed to that arr this tick. |
| `server.handleRegistryTestConnection` | Returns HTTP 500 `"decrypt failed"`. |
| `server.handleTargetsHealth` | Per-row `probe = "unauthorized"`, `probeError = "decrypt api_key failed"`. |

A decrypt failure on every arr after a restart is the canonical "operator
changed `secret_key`" signal.

## Rotating `secret_key`

The plugin will accept any new `secret_key` value. The consequence is that
every existing `registered_arr.api_key` is unreadable from that moment.

Recovery procedure:

1. Decide on the new key (or remove it from your config — on the next
   `GetAppConfig` the store will mint a fresh random key automatically).
2. Update `secret_key` in the admin Config tab. Save.
3. For each registered arr, PATCH `api_key` with the cleartext key again.
   The handler re-seals it under the new key.
4. Verify with `POST /api/admin/registry/{id}/test-connection` per row, or
   `GET /api/admin/targets/health` for the bulk view.

Until step 3 is complete for an arr, that arr is effectively offline: the
submit/poll loops will skip it.

There is no batch re-key endpoint. This is intentional — re-keying requires
the cleartext, which the plugin does not retain.

## Profile resolution

`internal/arr/profiles.go` provides "fill in the blanks" semantics. If the
admin row leaves `root_folder_path` empty or `quality_profile_id` NULL/0, the
submit path queries the arr for the available options and picks the first
one.

For Radarr (`ResolveRadarrDefaults`):

- `root_folder_path == ""` → `GET /rootfolder` → first entry.
- `quality_profile_id == 0` → `GET /qualityprofile` → first entry's `id`.
- Empty lists from the arr → submit fails with a descriptive error and the
  row goes to `failed`.

For Sonarr (`ResolveSonarrDefaults`):

- Same as above for root folder and quality profile.
- `language_profile_id == 0` → `GET /languageprofile` → first entry's `id`.
  Note: language profile resolution is "best effort" — if the arr returns
  an error or empty list, the request continues with `language_profile_id =
  0`. This matters because Sonarr v3 has language profiles and v4 has
  removed them; v4 ignores the field.

Operators who want **predictable** routing across multiple matching arrs
should set the profile IDs explicitly in the registry. Leaving them blank is
fine for a single-arr deployment.

## Admin REST API

All routes are under `/api/admin/registry` and require the admin role.

| Verb | Path | Body | Notes |
| --- | --- | --- | --- |
| `GET` | `/` | — | Returns `registryDTO[]` ordered by `(kind, priority, id)`. |
| `GET` | `/{id}` | — | 404 if no row. |
| `POST` | `/` | full `registryDTO` | `api_key` is **required** on create. Returns `{id}`. |
| `PATCH` | `/{id}` | partial `registryDTO` | Only fields present in body are touched. See key-rotation semantics below. |
| `DELETE` | `/{id}` | — | 204; cascade behaviour described below. |
| `POST` | `/{id}/test-connection` | optional `{api_key}` | Probes `/api/v3/system/status`. Returns 200 with `SystemStatusResponse` or 502 on transport error. |

### DTO conventions

- `api_key` is **write-only**. Reads return `has_api_key: true|false` instead.
- `rules` is a JSON `RawMessage` (the stored JSONB document, byte-for-byte).
- `quality_profile_id` / `language_profile_id` are pointer types in Go; in
  JSON `null` or `0` means "not set, let resolver pick".
- `kind` is constrained to `radarr`/`sonarr` server-side; bad values produce
  HTTP 400.

### PATCH semantics

The handler decodes into a `map[string]json.RawMessage` so it can distinguish
"absent" from "explicit null/zero". Each field is decoded only if its key is
present in the body. For `api_key` specifically:

- **Absent key** → no change.
- **Empty string** → no change. The operator can safely re-PATCH the row
  through the UI without re-entering the key.
- **Non-empty string** → rotate: re-seal with the current `secret_key`.

For `rules`, the handler runs `ParseRules` + `ValidateRules`. Invalid rule
documents are rejected with 400 and the previous rules survive.

### test-connection nuances

`POST /api/admin/registry/{id}/test-connection` accepts an optional body
`{"api_key": "..."}`. If provided, that key is used for the probe **without
persisting** — useful for verifying a new key before saving. If absent, the
stored key is decrypted and used.

The probe targets `/api/v3/system/status` with a default 10s timeout. On
success returns the full `SystemStatusResponse` (version, instanceName,
appName, branch).

### Listing pre-sort

`ListEnabledArrsByKind` orders by `(priority ASC, id ASC)` — the same order
the rule evaluator expects. `LoadCandidates` is the single point that
projects store rows into `routing.Candidate` and is the only thing the
submit path and route-test handler need to call.

`ListArrs` (returned by `GET /api/admin/registry/`) orders by
`(kind, priority, id)` instead so the admin UI groups Radarr / Sonarr blocks
together. Do not rely on this ordering for routing.

## Deleting an arr

`DELETE /api/admin/registry/{id}` removes the row. Because `request`'s
foreign key uses `ON DELETE SET NULL`, every in-flight request that was
routed to the deleted arr now has `routed_arr_id = NULL`:

- The poll loop's `groupByArr` drops these rows (no arr to query). They
  remain in `submitted`/`downloading` forever unless an operator acts.
- The cancel handler also early-returns on `GetArr → (nil, nil)`.
- Admin UI exposes **Force Fail** to push these orphans to `failed`.

Practical sequence for retiring an arr:

1. Set `enabled = false` and let in-flight requests drain (poll loop still
   sees them because they have a valid `routed_arr_id`).
2. Once all rows routed to that arr are terminal, `DELETE` the registry row.

If you must delete eagerly:

1. `DELETE` the registry row.
2. Force-Fail every orphan via the admin Requests tab (or batch via SQL:
   `UPDATE request SET status='failed', error='arr deleted', completed_at=now()
   WHERE routed_arr_id IS NULL AND status IN ('submitted','downloading')`).

## Targets health endpoint

`GET /api/admin/targets/health` is the operator's at-a-glance view. Per arr:

- Configured fields (`id`, `name`, `kind`, `url`, `enabled`, `priority`).
- Live probe — runs concurrently across all enabled arrs with a 4s timeout
  per arr.
  - `reachable` + `version` populated → arr answered `/api/v3/system/status`.
  - `unauthorized` → decrypt failed. Always a `secret_key` mismatch.
  - `unreachable` → transport error or non-2xx.
  - `skipped` → arr disabled; no network call made.
- 24h rolling counters from the request store: `submitted24h`, `failed24h`,
  `imported24h`, last submitted/failure timestamps, last failure message.

The 24h window is a rolling `updated_at > now() - interval '24 hours'`. A
single old failure does not pin "Failed 24h" non-zero forever, which is the
behaviour operators want when triaging.

Output ordering matches the evaluator: `priority ASC, name ASC`.

## Common pitfalls

- **Test-connection passing does not mean routing works.** It only proves
  the URL/API-key/network triple is good. Profile resolution can still fail
  later on AddMovie/AddSeries if the arr has no root folders or quality
  profiles configured.
- **`secret_key` is plaintext-equivalent.** Treat it like a password.
  Backing up the database without the secret leaves the API keys unrecoverable
  ciphertext; backing up both gives full access to every arr.
- **Reusing the same arr URL across two registry rows.** Allowed by the
  schema, useful for splitting routing across the same physical arr (e.g.
  two rules with different root paths). Each row carries its own encrypted
  copy of the API key — rotating it on one row does not propagate to the
  other.
- **A row with `enabled=false` is still polled** for its existing in-flight
  rows (poll fetches the row and checks `Enabled` — if false it skips, but
  the next poll tick re-checks). Disabling an arr stops it from being a
  routing candidate; it does not abandon in-flight requests.
- **Deleted-then-recreated arrs get a new `id`.** Existing orphaned requests
  do **not** automatically rebind to the new arr. They stay orphans.
