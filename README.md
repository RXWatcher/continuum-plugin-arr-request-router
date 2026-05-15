# Arr Request Router Plugin

`continuum.arrouter` is a rule-based request router for Continuum media
requests. It routes approved movie and TV requests to one of many registered
Radarr or Sonarr instances by evaluating admin-defined rules.

## What It Does

- Consumes submitted and cancelled request events from `continuum.requests`.
- Maintains an admin-managed registry of Radarr and Sonarr targets.
- Encrypts stored target API keys at rest.
- Enriches requests with TMDB metadata for rule evaluation.
- Routes each request to the first matching enabled target by priority.
- Polls routed targets for download/import progress.
- Publishes status events back to Continuum.
- Provides an admin SPA for registry management, rule editing, route tests, and
  request queue operations.

## Capabilities

| Capability | ID | Purpose |
|---|---|---|
| `event_consumer.v1` | `router` | Handles request submitted/cancelled events. |
| `scheduled_task.v1` | `poll` | Polls registered Radarr/Sonarr targets. |
| `http_routes.v1` | `admin` | Serves admin API and admin SPA. |
| `request_router.v1` | `default` | Advertises request-router support to Continuum. |

## HTTP Routes

| Route | Access | Purpose |
|---|---|---|
| `/api/admin/*` | admin | Registry, route test, queue, retry, and reroute API. |
| `/assets/*` | public | Static admin UI assets. |
| `/admin/*` | admin | Navigable admin UI labelled `Request Routing`. |

## Configuration

| Key | Required | Description |
|---|---|---|
| `database_url` | yes | Postgres DSN using the `arrouter` schema. |
| `tmdb.api_key` | yes | TMDB v3 API key for metadata enrichment. |
| `tmdb.language` | no | TMDB language tag. Defaults to `en-US`. |
| `poll_interval_seconds` | no | Poll interval in seconds. |
| `stale_after_hours` | no | Time before stuck requests are marked failed. |
| `secret_key` | yes | Secret used to encrypt registered target API keys. |

Example `database_url`:

```text
postgres://plugin_arrouter:password@postgres:5432/continuum?search_path=arrouter&sslmode=disable
```

## Database Setup

```sql
CREATE ROLE plugin_arrouter WITH LOGIN PASSWORD '<chosen>';
CREATE SCHEMA arrouter AUTHORIZATION plugin_arrouter;
GRANT CONNECT ON DATABASE continuum TO plugin_arrouter;
```

The plugin applies its own migrations inside the configured schema.

## Routing Model

Each registered target has:

- type: Radarr or Sonarr
- base URL
- encrypted API key
- root folder path
- quality profile
- optional language profile for Sonarr
- priority
- enabled/disabled state
- rules JSON

Rules can match fields from the original request and enriched TMDB metadata,
including title, year, genres, keywords, content ratings, requester attributes,
and library context. Lower priority numbers are tried first. The first enabled
target whose rules match receives the request.

## Event Flow

Input subscriptions:

- `plugin.continuum.requests.submitted`
- `plugin.continuum.requests.cancelled`

Published status events:

- `plugin.continuum.arrouter.submitted`
- `plugin.continuum.arrouter.downloading`
- `plugin.continuum.arrouter.imported`
- `plugin.continuum.arrouter.failed`
- `plugin.continuum.arrouter.cancelled`
- `plugin.continuum.arrouter.unrouted`

`unrouted` means no enabled target matched the request rules.

## Build And Test

```bash
go test ./...
go build -buildvcs=false -o continuum-plugin-arr-request-router ./cmd/continuum-plugin-arr-request-router
```

If web assets are changed, build the web project before packaging the binary.

## Operational Notes

- Rotating `secret_key` invalidates stored target API keys.
- TMDB failures do not automatically fail requests; rules depending on missing
  enriched fields evaluate as not matched.
- The poll loop fans out by target so a slow target does not block the entire
  queue.
- Prefer explicit catch-all targets at low priority for fallback behavior.

## Repository Status

This is a first-party Continuum plugin owned by the Continuum project.
