# Arr Request Router for Continuum

`continuum.arrouter` is a rule-based router for Continuum movie and TV
requests. It receives approved requests from `continuum.requests`, evaluates
operator-defined rules, and sends each request to the first matching Radarr or
Sonarr target.

Use this plugin when one Arr stack is not enough: multiple quality tiers,
language-specific instances, separate 4K targets, regional libraries, or
requester/group based routing.

## Detailed Operations Docs

- [Setup, debugging, and communication flows](docs/setup-debug-flows.md)

## Features

- Consumes submitted and cancelled request events from `continuum.requests`.
- Maintains an admin-managed registry of Radarr and Sonarr targets.
- Encrypts stored target API keys at rest.
- Enriches requests with TMDB metadata for rule evaluation.
- Routes requests by priority to the first enabled matching target.
- Polls routed targets for download and import progress.
- Provides an admin SPA for targets, rules, route testing, queue visibility,
  retry, and reroute operations.

## Configuration

| Key | Required | Description |
|---|---|---|
| `database_url` | yes | Postgres DSN for the `arrouter` schema. |
| `tmdb.api_key` | yes | TMDB v3 API key for metadata enrichment. |
| `tmdb.language` | no | TMDB language tag. Defaults to `en-US`. |
| `poll_interval_seconds` | no | Poll interval in seconds. |
| `stale_after_hours` | no | Time before stuck requests are marked failed. |
| `secret_key` | yes | Symmetric key used to encrypt registered target API keys. Rotating it invalidates stored target keys. |

Example DSN:

```text
postgres://plugin_arrouter:password@postgres:5432/continuum?search_path=arrouter&sslmode=disable
```

## Database Setup

```sql
CREATE ROLE plugin_arrouter WITH LOGIN PASSWORD '<chosen>';
CREATE SCHEMA arrouter AUTHORIZATION plugin_arrouter;
GRANT CONNECT ON DATABASE continuum TO plugin_arrouter;
```

## Routing Model

Each registered target stores type, base URL, encrypted API key, root folder,
quality profile, optional Sonarr language profile, priority, enabled state, and
rules JSON. Rules can match the original request and enriched TMDB metadata,
including title, year, genres, keywords, content ratings, requester attributes,
and library context.

Lower priority numbers are evaluated first. The first enabled target whose
rules match receives the request.

## Build And Test

```bash
make build
make test
```
