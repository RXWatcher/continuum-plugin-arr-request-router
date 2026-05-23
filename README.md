# Arr Request Router for Silo

`silo.arrouter` is a rule-based request router that consumes request events from [`silo-plugin-requests`](https://github.com/RXWatcher/silo-plugin-requests) and forwards each request to one of N registered Radarr/Sonarr instances based on operator-defined rules.

Use it when one Arr stack is not enough: multiple quality tiers, language- or region-specific instances, separate 4K targets, or requester/group based routing.

## Category

Lives under **Requests**.

Install at most one of [`silo-plugin-arr-proxy`](https://github.com/RXWatcher/silo-plugin-arr-proxy) or `silo-plugin-arr-request-router` per Silo installation. They both fulfil the `request_router` capability — `arr-proxy` fronts a single Arr Proxy backend, while this plugin selects across N directly-registered Radarr/Sonarr instances.

## Capabilities

| Type | ID | Purpose |
| --- | --- | --- |
| `event_consumer.v1` | `router` | Subscribes to submitted and cancelled request events from `silo.requests` and dispatches them to the chosen Radarr/Sonarr. |
| `scheduled_task.v1` | `poll` | Polls registered Radarr/Sonarr instances for download/import progress and publishes lifecycle events. |
| `http_routes.v1` | `admin` | Admin SPA for managing the arr registry, rules, queue, and route testing. |
| `request_router.v1` | `default` | Declares this plugin as the rule-based router for the Requests category. |

## Dependencies

- Consumes `plugin.silo.requests.submitted` and `plugin.silo.requests.cancelled` from [`silo-plugin-requests`](https://github.com/RXWatcher/silo-plugin-requests).
- Manages N external Radarr and Sonarr instances directly via their HTTP APIs; no separate Arr Proxy backend.
- Requires a dedicated Postgres schema (`arrouter`) for the arr registry, request state, and rule storage.

Host: [`ContinuumApp/silo`](https://github.com/ContinuumApp/silo). SDK: [`ContinuumApp/continuum-plugin-sdk`](https://github.com/ContinuumApp/continuum-plugin-sdk).

## External services

- **Radarr / Sonarr** — one or more instances registered through the admin UI; the plugin calls each instance's HTTP API to dispatch lookups, add requests, poll status, and cancel/delete.
- **TMDB v3** — used to enrich each request with metadata (primary movie/TV record, keywords, content rating) so rules can match on genres, language, runtime, ratings, networks, and so on.

## Routing rules

Each registered arr stores a JSON rules document with a top-level `match` combinator (`all` or `any`) and a list of groups; each group has its own combinator and a list of `(field, op, value)` predicates. Fields cover the request event itself (`mediaType`, `libraryId`, `year`, `decade`, `title`, `tmdbId`, `requesterUserId`, `requesterIsAdmin`) and TMDB-enriched data (`original_language`, `genres`, `runtime`, `vote_average`, `popularity`, movie-only fields like `release_date`/`imdb_id`, TV-only fields like `networks`/`number_of_seasons`, plus `keywords` and `content_rating`).

Candidates are filtered by kind (`movie` → Radarr, `tv` → Sonarr), sorted by `(priority ASC, id ASC)`, and evaluated in order. The first enabled candidate whose rules match wins; an empty rules document matches everything and is the natural shape for a catch-all lowest-priority target. TMDB enrichment is lazy — only the API calls referenced by candidate rules are issued for any given request. Every decision produces a diagnostic trace (per-candidate match result, group results, TMDB errors) that drives the admin route-test UI.

## Arr registry

Registered arrs are stored in the `registered_arr` table with their name, kind (`radarr`|`sonarr`), base URL, root folder, quality profile, optional Sonarr language profile, priority, enabled flag, and rules JSON. API keys are encrypted at rest with AES-256-GCM via `internal/crypto/secret.go`; the AES key is derived from the `secret_key` app-config value (SHA-256 of the configured string). API keys are never echoed back from the admin API — responses expose a boolean `has_api_key` instead. Rotating `secret_key` invalidates every stored arr API key, which must then be re-entered.

## Configuration

| Key | Required | Description |
| --- | --- | --- |
| `database_url` | yes | Postgres DSN for the dedicated `arrouter` schema (host-managed bootstrap config). |
| `tmdb.api_key` | yes | TMDB v3 API key used for metadata enrichment when evaluating rules. |
| `tmdb.language` | no | TMDB language tag for localised titles and content ratings. Defaults to `en-US`. |
| `poll_interval_seconds` | no | Interval between background polls of registered arrs. Defaults to 30; clamped to 10–600. |
| `stale_after_hours` | no | Age after which an in-flight request is marked failed. Defaults to 72. |
| `secret_key` | yes | Symmetric key used to encrypt registered arr API keys at rest. Auto-generated on first start if not provided; rotating it invalidates all stored arr API keys. |

Example DSN:

```text
postgres://plugin_arrouter:password@postgres:5432/silo?search_path=arrouter&sslmode=disable
```

Bootstrap the schema with:

```sql
CREATE ROLE plugin_arrouter WITH LOGIN PASSWORD '<chosen>';
CREATE SCHEMA arrouter AUTHORIZATION plugin_arrouter;
GRANT CONNECT ON DATABASE silo TO plugin_arrouter;
```

## Event subscriptions

- `plugin.silo.requests.submitted` — upserts a `queued` request row, enriches via TMDB, evaluates routing rules, dispatches to the chosen Radarr/Sonarr, and emits a `submitted` or `unrouted` event.
- `plugin.silo.requests.cancelled` — best-effort DELETE on the routed arr (when an external ID is recorded), marks the request `cancelled` in the store, and emits `cancelled`.

## Event publications

All events are published under the `plugin.silo.arrouter.` prefix:

- `submitted` — request was accepted and dispatched to an arr.
- `downloading` — the routed arr has begun fetching the media.
- `imported` — the routed arr has imported the media into the library.
- `failed` — terminal failure; the payload includes an `error` string.
- `cancelled` — the request was cancelled before or during download.
- `unrouted` — no enabled registered arr matched; the payload includes an `error` string.

## Detailed docs

- [Setup, debugging, and communication flows](docs/setup-debug-flows.md)
- [Plugin specification](SPEC.md)

## Build and release

```bash
make build   # builds the web SPA then the Go binary
make test    # runs Go and web tests
```

CI builds linux-amd64 binaries on push to main via the reusable workflow in [RXWatcher/silo-plugin-repository](https://github.com/RXWatcher/silo-plugin-repository) and publishes them to the catalog at [`./binaries/`](https://github.com/RXWatcher/silo-plugin-repository/tree/main/binaries).
