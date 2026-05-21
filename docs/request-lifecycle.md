# Request lifecycle

This document describes the state machine for rows in the `request` table,
which transitions fire which events, and the admin actions that move rows
between states. It complements [`routing-rules.md`](routing-rules.md) (which
covers the rule engine itself) and the operator runbook in
[`setup-debug-flows.md`](setup-debug-flows.md).

## States

```
                 plugin.continuum.requests.submitted
                              │
                              ▼
                          ┌────────┐
                          │ queued │  (upsert; no row updates if it already exists)
                          └───┬────┘
                              │ rule evaluator
            ┌─────────────────┼──────────────────────┐
            ▼                 ▼                      ▼
       no candidate       chosen=arr            chosen=arr
       matched            AddMovie/Series       AddMovie/Series
            │             returns 2xx            returns 409
            ▼                 │                      │
       ┌───────────┐          ▼                      ▼
       │ unrouted  │      ┌─────────────┐
       │ (term.)   │      │ submitted   │ ◄────────── (409 path: external_id may be 0)
       └─────┬─────┘      └──────┬──────┘
             │                   │ poll: queue non-empty
             │                   ▼
             │              ┌─────────────┐
             │              │ downloading │
             │              └──────┬──────┘
             │                     │ poll: HasFile / 100% episodes
             │                     ▼
             │              ┌────────────┐
             │              │ imported   │  (terminal)
             │              └────────────┘
             │
   admin: Re-route                       admin: Retry
             │                                  │
             └────► queued ◄────────────────────┘
                       ▲                                ▲
                       │                                │
                AddMovie/Series error                   │
                (non-409, non-conflict)                 │
                       │                                │
                       ▼                                │
                  ┌────────┐                            │
                  │ failed │ ─────── admin: Force Fail  │
                  └────────┘             on any         │
                       ▲                 non-terminal   │
                       │                                │
              plugin.continuum.requests.cancelled       │
                       │                                │
                       ▼                                │
                 ┌────────────┐                         │
                 │ cancelled  │ ◄── best-effort arr DELETE
                 └────────────┘
                  (terminal)
```

Terminal states: `imported`, `failed`, `cancelled`, `unrouted`. The cancel
handler is a no-op on rows already in a terminal state.

## Per-state details

### `queued`

Created by `UpsertRequestQueued` with `INSERT … ON CONFLICT (id) DO NOTHING`.
That means a re-emit of the same `requestId` does not bump `updated_at`, which
is on purpose: if a previous submit attempt already advanced the row, we
don't want to flap it back.

The Submit handler immediately attempts routing. The only time a row sits in
`queued` for more than a few milliseconds is between a Retry/Re-route admin
click and the `Submit.Submit` call that follows it.

### `submitted`

`MarkSubmitted` is idempotent against both `queued` and `submitted` source
states. The guarded UPDATE means a stale duplicate event cannot move a
`downloading` row backwards.

`external_id = 0` is possible after the 409 path — the title already existed
on the arr, so the AddMovie/AddSeries response did not include an ID. The
poll loop has all the info it needs (title, tmdbId) to recover the real
external id on the next tick.

### `downloading`

`MarkDownloading` only matches `queued`/`submitted`. The poller checks
`tag.RowsAffected()` to know whether a transition actually happened and only
fires the `downloading` event on real transitions. Re-polling a row already
in `downloading` is silent.

### `imported`

Reached when:

- Radarr: `GET /movie/{id}` returns `hasFile: true`.
- Sonarr: `GET /series/{id}` returns `statistics.percentOfEpisodes >= 100`.

`MarkImported` is guarded to `submitted`/`downloading` so a manual DB poke
cannot push a `failed` row to `imported` accidentally. `completed_at` is set.

### `failed`

Three writers:

1. Submit path on a non-409 error from Radarr/Sonarr — the error string is
   stored verbatim in `request.error`.
2. Poll path's stale guard — `error = "stuck past staleness threshold"` after
   `stale_after_hours` has elapsed since `submitted_at` (or `created_at` if
   submit hasn't run).
3. Admin **Force Fail** — `error = "force-failed by admin"`. Used for orphans
   (rows whose `routed_arr_id` is NULL because the arr was deleted).

`MarkFailed` is guarded against terminal states, so retries/duplicates won't
overwrite a `failed`/`imported` row.

### `cancelled`

Driven by the `plugin.continuum.requests.cancelled` event. Sequence:

1. Parse `requestId`; missing → no-op.
2. Load row; not found or already terminal → no-op.
3. If `routed_arr_id` AND `external_id` are both set, best-effort
   `DeleteMovie`/`DeleteSeries`. Errors are logged but do not block the next
   step.
4. `MarkCancelled` (guarded against terminal states).
5. Publish `cancelled`.

The arr DELETE is intentionally best-effort: an unreachable arr should not
leave the row stuck in a non-terminal state.

### `unrouted`

Only the submit path writes this state, and only from `queued`. The full
`match_trace` is persisted into `match_trace` JSONB and the human-readable
reason (currently always `"no registered *arr matched"`) is stored in
`request.error`. `completed_at` is set even though the row is "recoverable"
via Re-route — operators care more about a stable completion timestamp than
re-armable rows.

## Polling

See `internal/poll/poll.go`. Per Run iteration:

1. `Store.ListPollable` returns all rows in `submitted` or `downloading`.
2. Group by `RoutedArrID`. Rows with NULL `RoutedArrID` (orphans) are dropped
   — they cannot be polled. Use Force Fail to remove them.
3. For each arr: fetch the row, decrypt `api_key`, spawn a goroutine. Within
   the goroutine, the rows for that arr are polled **sequentially** (so a
   slow arr serialises its own load and doesn't fan out further).
4. After every per-row poll (success or no-op), `UpdateLastPolled` writes
   `last_polled_at`. This explicitly does **not** touch `updated_at`.

The scheduled task is wired to fire on `poll_interval_seconds` (default 30,
clamped 10–600) and is also exposed over gRPC as `ScheduledTaskServer.Run` so
the host can trigger an ad-hoc tick (e.g. an admin "Run now" button — UI for
this is not yet exposed but the backend supports it).

### Movie poll path

```
GetMovie(externalID)
   ├── hasFile=true  → MarkImported (if not already imported) → Imported event
   └── hasFile=false → QueueByMovie(externalID)
                          ├── non-empty → MarkDownloading (if transitioned) → Downloading event
                          └── empty    → maybeMarkStale
```

### TV poll path

```
GetSeries(externalID)
   ├── stats.percentOfEpisodes ≥ 100 → MarkImported (if not already) → Imported event
   └── < 100                         → QueueBySeries(externalID)
                                          ├── non-empty → MarkDownloading (if transitioned) → Downloading event
                                          └── empty    → maybeMarkStale
```

`maybeMarkStale` is a no-op when `StaleAfterHours <= 0`, which provides an
escape hatch for operators who want to disable the auto-fail behaviour
entirely.

## Admin actions on `request`

All under `/api/admin/requests`:

| Endpoint | Required source state | What it does |
| --- | --- | --- |
| `GET /` | — | Paginated list (50/page, max 200). Optional `status=` filter. |
| `GET /{id}` | — | Detail, with `routed_arr_name` resolved best-effort (N+1 query — fine for the small registry). |
| `POST /{id}/retry` | `failed` only | `MarkRetrying` clears per-attempt fields → calls `Submit.Submit` synchronously → 202 Accepted. |
| `POST /{id}/re-route` | `unrouted` only | `MarkReRouting` clears `match_trace` and reason → re-runs `Submit.Submit` → 202 Accepted. |
| `POST /{id}/force-fail` | non-terminal only | Writes `failed` with `"force-failed by admin"`. Does **not** call the arr. |

Important gotchas:

- **Retry/Re-route re-run the routing decision from scratch.** TMDB metadata
  may have changed since the last attempt, and rule changes will take effect.
  This is the desired behaviour but worth flagging — the new decision can
  pick a different arr than the original attempt.
- **Force-Fail does not contact the arr.** If the arr is reachable, prefer
  the original `cancelled` flow (delete on the arr too). Force-Fail exists
  for orphans and operator-driven recovery.
- **`MarkRetrying` clears `external_id`**, so the next submit attempt does not
  carry over a stale id. The 409 path will rediscover it if the arr already
  has the title.
- **Bulk operations are not exposed.** Operators wanting to retry many failed
  rows at once should do so via direct SQL on the schema or by scripting
  against the admin API.

## Event topics published

All under `plugin.continuum.arrouter.`:

| Topic | Fired by | Payload |
| --- | --- | --- |
| `submitted` | submit path (both AddMovie/Series success and 409) | `{ requestId }` |
| `downloading` | poll path on real `submitted → downloading` transition | `{ requestId }` |
| `imported` | poll path on real transition to `imported` | `{ requestId }` |
| `failed` | submit path (dispatch error), poll path (stale guard), admin force-fail | `{ requestId, error }` |
| `cancelled` | cancel handler after `MarkCancelled` | `{ requestId }` |
| `unrouted` | submit path when no candidate matched | `{ requestId, error }` |

`event.Publisher.Publish` swallows broker errors after logging — events are
"fire and forget" for the plugin. Downstream consumers (notifications etc.)
that need stronger delivery should handle that themselves.

## Status-machine guards

Each `Mark*` function has an explicit `AND status IN (…)` clause. The matrix:

| Function | Allowed source states |
| --- | --- |
| `MarkSubmitted` | `queued`, `submitted` |
| `MarkDownloading` | `queued`, `submitted` |
| `MarkImported` | `submitted`, `downloading` |
| `MarkFailed` | `queued`, `submitted`, `downloading` |
| `MarkCancelled` | `queued`, `submitted`, `downloading` |
| `MarkUnrouted` | `queued` |
| `MarkRetrying` | `failed` |
| `MarkReRouting` | `unrouted` |

If a transition is rejected by the guard, the function returns `nil` (no
error). The caller observes "nothing happened" through ignored row counts —
this is intentional, idempotent behaviour for an at-least-once event bus.
