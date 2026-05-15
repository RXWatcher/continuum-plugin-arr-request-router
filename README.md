# continuum-plugin-arr-request-router

Arr Request Router is a rule-based multi-*arr router for the Continuum media server. It consumes
`plugin.continuum.requests.submitted` and `plugin.continuum.requests.cancelled`
events, evaluates each request against an admin-curated list of registered
Radarr/Sonarr instances using configurable rule groups, and forwards to the
first matching *arr in priority order.

Arr Request Router manages an arbitrary number of *arr instances. An admin
creates and prioritizes them via the built-in admin SPA; each instance carries
its own URL, API key, root folder path, quality profile, and a set of routing
rules.

## Install — operator pre-flight

**1. Create the Postgres role and schema** in continuum's database:

```sql
CREATE ROLE plugin_arrouter WITH LOGIN PASSWORD '<chosen by operator>';
CREATE SCHEMA arrouter AUTHORIZATION plugin_arrouter;
GRANT CONNECT ON DATABASE continuum TO plugin_arrouter;
```

The plugin runs its own migrations on startup against the `arrouter` schema.

**2. Get a TMDB v3 API key.** Register at <https://www.themoviedb.org/settings/api>.
The plugin uses it to enrich requests with keyword and content-rating data
for rule evaluation.

**3. Generate a secret key** (16+ random characters). This is used to
AES-GCM-encrypt API keys stored in the database. Rotating it invalidates all
stored API keys — every registered *arr will need its API key re-entered via
the admin SPA.

## Configuration

Set in continuum's plugin admin UI (fields are defined in `manifest.json`
under `global_config_schema`):

| Key | Required | Description |
|---|---|---|
| `database_url` | yes | `postgres://plugin_arrouter:<pw>@<host>/<db>?search_path=arrouter` |
| `tmdb.api_key` | yes | TMDB v3 read API key |
| `tmdb.language` | no | TMDB language tag; defaults to `en-US` |
| `secret_key` | yes | AES-GCM key for API key encryption; min 16 chars |
| `poll_interval_seconds` | no | Status poll interval; default 30, min 10, max 600 |
| `stale_after_hours` | no | Mark stuck requests failed after N hours; default 72 |

## Rules

Each registered *arr carries a top-level rule set (match `all` or `any` of N
rule groups). Each group is a set of individual field rules. A request is
routed to an *arr only if its rule set matches.

Example (matches English-language horror movies):

```json
{
  "match": "all",
  "groups": [
    {
      "match": "all",
      "rules": [
        { "field": "genre", "op": "contains", "value": "Horror" },
        { "field": "original_language", "op": "eq", "value": "en" }
      ]
    }
  ]
}
```

See [SPEC.md](SPEC.md) for the full field catalog, operator reference, and
routing algorithm.

## Build

```
make build                              # pnpm run build + go build
./continuum-plugin-arr-request-router manifest    # print manifest JSON
```

```
make test    # go test ./...
```

The Makefile runs `pnpm run build` inside `web/` before `go build` so the
embedded SPA is always up-to-date.

## Develop

The SDK is referenced via `replace` in `go.mod` (points at
`/opt/worktrees/continuum-plugin-sdk-rh`). Adjust the path to your local SDK
checkout. Run `make test` for backend tests; `pnpm run dev` inside `web/` for
the frontend hot-reload server.

## License

MIT
