# Arr Request Router — Operator Runbook

Plugin ID: `silo.arrouter`

This document is the **operations playbook** for the plugin: bootstrap order,
runtime invariants, log reading, common failures, and verification flows. For
the high-level overview, capabilities, and event topics see the
[README](../README.md). For everything rule-engine see
[`routing-rules.md`](routing-rules.md); for state-machine semantics see
[`request-lifecycle.md`](request-lifecycle.md); for arr CRUD/encryption see
[`arr-registry.md`](arr-registry.md).

## Bootstrap order

The plugin is safe to install before configuration — it boots with an unwired
state and the consumer/poll loops short-circuit. Bring it up in this order:

1. **Database** — create the `arrouter` schema and a role that owns it; set
   `database_url` (host-managed bootstrap config, not exposed in the admin UI).
2. **First start** — the runner applies `0001_init.up.sql` and
   `0002_app_config.up.sql`. The latter creates a singleton `app_config` row
   (id=1) that holds the operator-editable settings.
3. **Secret key** — on first GET `/api/admin/config` the store auto-generates a
   32-byte random `secret_key` (base64) and persists it back into `app_config`.
   You can override it later, but understand the rotation consequences below.
4. **TMDB key** — set `tmdb.api_key` in the admin Config tab. Until set, any
   rule that references TMDB-derived fields will produce a trace entry with
   `tmdb_primary_error`/`keywords_error`/`content_rating_error`, which the
   evaluator treats as "field missing" (rule cannot match).
5. **Register arrs** — Registry tab. Each new arr must include the API key in
   the create payload; subsequent PATCH calls treat an empty `api_key` as
   "don't rotate".
6. **Smoke test** — use the Route Test tab with a known tmdbId. This is the
   single most useful diagnostic: it runs the full evaluator including TMDB
   enrichment without writing anything to the request table.

## Runtime moving parts

| Subsystem | Lives in | What it does |
| --- | --- | --- |
| Event consumer (`router`) | `internal/consumer` | Listens for `plugin.silo.requests.submitted` / `cancelled`. Submit handler upserts → routes → dispatches to arr. Cancel handler best-effort DELETEs on the arr then marks the row. |
| Poll loop (`poll`) | `internal/poll` | Scheduled task. Groups in-flight rows by `routed_arr_id`, fans out one goroutine per arr, polls sequentially within each arr. Drives `downloading`/`imported`/`failed` transitions. Also runs ad-hoc when the host invokes `ScheduledTaskServer.Run` (e.g. an admin "Run now"). |
| TMDB cache | `internal/tmdb` | In-process cache with 24h-style TTL per `(mediaType, tmdbID)` and per enrichment group. Errors are **never** cached. |
| Crypto | `internal/crypto` | AES-256-GCM (key = SHA-256 of `secret_key` string). Wraps every stored arr `api_key`. |
| Registry store | `internal/store/registry.go` | CRUD on `registered_arr`. `LoadCandidates` filters by kind+enabled and projects to `routing.Candidate`. |
| Request store | `internal/store/request.go` | Status machine. All write methods guard on source status so the same event arriving twice is a no-op. |
| Admin server | `internal/server` | `chi` router gated by `requireAdmin`. Hosts registry CRUD, route-test, request actions, target health, config, and the SPA. |

## Key invariants

- **Candidates are pre-sorted.** `Store.ListEnabledArrsByKind` orders by
  `(priority ASC, id ASC)`. The router relies on that — it never sorts.
- **First match wins, no fallthrough.** When the chosen arr's `AddMovie` /
  `AddSeries` call fails (non-409), the request is marked `failed`. The router
  does **not** retry against the next-priority arr. This is intentional —
  silently sending content to a tier the operator did not pick would surprise
  more often than it would help.
- **409 is success.** Radarr/Sonarr return 409 when the title already exists
  in the library. The submit path treats `arr.IsConflict(err)` as success and
  the row goes to `submitted`. The next poll tick recovers `external_id` via
  the queue endpoints.
- **`updated_at` is mutated by every transition; `last_polled_at` is not.**
  This is deliberate — change-tracking views and the "stale" notion key off
  `submitted_at`/`created_at`, not poll noise.
- **Empty rules = match-all.** A registered arr with `rules_json` = `{}`,
  `{"match":"all","groups":[]}`, or any document with zero groups matches
  every event. This is the catch-all shape — see "Common pitfalls" below.
- **API keys are write-only over the wire.** Admin responses contain
  `has_api_key: true|false`, never the cleartext or ciphertext. Test-connection
  accepts an optional override key so an operator can probe a new credential
  before persisting it.

## Diagnostic flows

### Why was this request not routed?

1. Open the request in the admin Requests tab; status will be `unrouted`.
2. Inspect `match_trace`. It is the JSON-serialised `routing.Trace`:
   - `tmdb_primary_error` / `keywords_error` / `content_rating_error` set →
     TMDB call failed; rules that depend on those fields silently fail with
     `note: "missing"`. Fix the API key, network, or the offending rule.
   - `candidates` is empty → no enabled arr of the right `kind` exists. Check
     that you have at least one enabled Radarr for movies and Sonarr for TV.
   - `candidates` non-empty but every entry has `matched: false` → at least one
     `RuleResult.note` will explain (`type mismatch`, `invalid regex: …`,
     `missing` for fields that needed TMDB data that wasn't loaded).
3. Re-run the same payload in Route Test (no DB writes) while you tweak rules.
4. When the rules are fixed, click Re-route. The row transitions
   `unrouted` → `queued` and is re-submitted.

### Why is a request stuck on submitted/downloading?

1. Confirm the row's `routed_arr_id` still points to an existing, enabled arr.
   If `routed_arr_id` is NULL (the arr was deleted; `ON DELETE SET NULL` ran),
   the poll loop can't reach it. Use **Force Fail** from the admin UI to push
   it to a terminal state.
2. Look at `last_polled_at`. If it is more than ~`poll_interval_seconds` old,
   the poll loop is not running or returning early. Common causes:
   - `database_url` invalid → poll exits immediately. Check process logs.
   - `crypto.Open` failed for the arr's API key → the entire arr group is
     skipped this tick with a `decrypt api_key failed` warning. This is the
     post-rotation symptom (see encryption section in
     [`arr-registry.md`](arr-registry.md)).
3. If `last_polled_at` is fresh but state never advances, check the arr's UI:
   the queue may show an indexer/download client error. The poll loop only
   surfaces transitions; it does not interpret failed download statuses
   (deliberate — the operator's existing Radarr/Sonarr notifications cover
   that).
4. The stale guard (`stale_after_hours`, default 72) only triggers when the
   item is no longer in the arr's queue **and** has no file. A stuck download
   that the arr still reports as in-queue will never hit the stale path.

### Why is the API returning 403?

`requireAdmin` middleware rejects anything without the
`X-Silo-User-Role: admin` header. In normal operation the plugin host
stamps this; getting 403 from outside Silo means you are bypassing the
host (e.g. hitting the plugin port directly). Don't do that.

## Logs to grep

The plugin uses `hclog` via the SDK. Useful patterns:

| Message | Source | Means |
| --- | --- | --- |
| `decrypt api_key failed` | `poll.Run`, `consumer.Cancel`, `targets_health_handler` | `secret_key` has changed since this arr was saved, or the ciphertext is corrupt. |
| `marshal route trace failed; persisting empty trace` | `consumer.Submit` | Should never fire in practice — trace shape is pure data. If it does, file a bug. |
| `MarkSubmitted failed` / `MarkImported failed` / `MarkFailed failed` | `poll`, `consumer` | DB write failed during a transition. The next poll cycle will retry. |
| `radarr GetMovie error` / `sonarr GetSeries error` | `poll.pollMovie/pollTV` | Arr unreachable or returned non-2xx. Row stays in current state; next poll tick retries. |
| `publish event` (warn) | `event.Publisher` | The plugin host's event bus rejected a publish. Usually transient; not retried. |

## Health endpoint

`GET /api/admin/targets/health` probes every enabled arr concurrently with a
4-second timeout, merges in 24h rolling counters from `request`, and returns
one row per registered arr (disabled ones included, but their `probe` field is
`"skipped"`). Use this from the admin UI's Targets page or curl it directly
when triaging.

Probe states:

- `reachable` — `/api/v3/system/status` returned 2xx; `version` populated.
- `unauthorized` — could not decrypt the stored API key. **Always** indicates
  a `secret_key` mismatch on that row.
- `unreachable` — network error or non-2xx from the arr.
- `skipped` — arr is disabled.

## Verification after changes

1. Restart the plugin installation (host UI or `systemctl`/process manager —
   whatever the host uses).
2. `GET /api/admin/config` should return your latest values.
3. `GET /api/admin/registry/` should list all arrs with the expected
   `has_api_key: true`.
4. `POST /api/admin/registry/{id}/test-connection` should return 200 with the
   arr's version.
5. Approve one real request from `silo.requests`. Observe the row in the
   admin Requests tab transition `queued` → `submitted` (immediate) →
   `downloading` (next poll after the arr picks it up) → `imported` (when the
   arr finishes).
6. The corresponding events should also appear on
   `plugin.silo.arrouter.*` for downstream consumers (notifications etc.)
   to pick up.

## Detailed references

- [Routing rules grammar and field reference](routing-rules.md)
- [Request lifecycle and admin actions](request-lifecycle.md)
- [Arr registry: encryption, profile resolution, edge cases](arr-registry.md)
- [Plugin specification](../SPEC.md)
