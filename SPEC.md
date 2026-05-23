# silo.arrouter — design

A rule-based multi-*arr router. Receives request events from `silo.requests`,
evaluates the request (with on-demand TMDB enrichment) against an admin-curated
list of registered Radarr / Sonarr instances, and forwards to **the first
matching *arr in priority order**. Polls each registered *arr periodically to
track download/import progress and publishes status events back so subscribers
(today: `silo.requests`) can mirror state.

This plugin is designed for installs that need more than one Radarr or Sonarr
target. Admins define the available instances and the rules that choose between
them.

## Capabilities (manifest)

- `event_consumer.v1` — subscribes to:
  - `plugin.silo.requests.submitted`
  - `plugin.silo.requests.cancelled`
- `scheduled_task.v1` — `poll` task, runs every `poll_interval_seconds`
  (default 30s).
- `http_routes.v1` — admin SPA at `/admin` (admin-only) with three pages:
  registry list, registry editor, requests queue. `/api/admin/*` for the SPA's
  data calls. Same theme-injection rule as every other silo plugin SPA
  (see "SPA theme handling" below).

## Identity

| | |
|---|---|
| Plugin id | `silo.arrouter` |
| Schema | `arrouter` |
| Postgres role | `plugin_arrouter` |
| Repo | `silo-plugin-arr-request-router` |

## DB (schema-per-plugin)

Operator pre-creates the role + grants:

```sql
CREATE ROLE plugin_arrouter WITH LOGIN PASSWORD '...';
CREATE SCHEMA arrouter AUTHORIZATION plugin_arrouter;
```

Tables:

```sql
CREATE TABLE arrouter.registered_arr (
  id                  BIGSERIAL PRIMARY KEY,
  name                TEXT NOT NULL,                 -- admin-friendly label
  kind                TEXT NOT NULL CHECK (kind IN ('radarr','sonarr')),
  url                 TEXT NOT NULL,
  api_key             TEXT NOT NULL,                 -- encrypted at rest (see below)
  root_folder_path    TEXT NOT NULL,
  quality_profile_id  INTEGER,                       -- nullable → first profile
  language_profile_id INTEGER,                       -- sonarr v3 only
  priority            INTEGER NOT NULL DEFAULT 100,  -- ascending; lower wins
  enabled             BOOLEAN NOT NULL DEFAULT true,
  rules_json          JSONB NOT NULL DEFAULT '{"match":"all","groups":[]}'::jsonb,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX registered_arr_kind_priority_idx
  ON arrouter.registered_arr (kind, priority) WHERE enabled;

CREATE TABLE arrouter.request (
  id                  TEXT PRIMARY KEY,              -- same id silo.requests uses
  tmdb_id             INTEGER NOT NULL,
  media_type          TEXT NOT NULL CHECK (media_type IN ('movie','tv')),
  title               TEXT NOT NULL,
  year                INTEGER NOT NULL DEFAULT 0,
  poster_url          TEXT,
  requester_user_id   TEXT NOT NULL,
  requester_is_admin  BOOLEAN NOT NULL DEFAULT false,
  status              TEXT NOT NULL CHECK (status IN
    ('queued','submitted','downloading','imported','failed','cancelled','unrouted')),
  routed_arr_id       BIGINT REFERENCES arrouter.registered_arr(id) ON DELETE SET NULL,
  external_id         INTEGER,                        -- *arr's internal id, after submit
  error               TEXT,
  match_trace         JSONB,                          -- per-*arr eval trace, for diagnostics
  submitted_at        TIMESTAMPTZ,
  last_polled_at      TIMESTAMPTZ,
  completed_at        TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX request_status_idx ON arrouter.request (status)
  WHERE status IN ('submitted','downloading');
CREATE INDEX request_tmdb_idx ON arrouter.request (tmdb_id, media_type);
CREATE INDEX request_routed_arr_idx ON arrouter.request (routed_arr_id)
  WHERE status IN ('submitted','downloading');
```

Notes:

- `api_key` is stored encrypted with the same secret-handling pattern silo
  uses for plugin global config secrets. Plaintext at rest is **not** acceptable
  even for an OSS plugin; admins routinely run multiple *arrs and rotating one
  key shouldn't require leaking the rest.
- `match_trace` is best-effort diagnostics ("matched arr #3 because rule
  group 1, rule 2 evaluated true"). Captured at routing time; truncated /
  capped at a few KB.

## Status state machine

```
queued → submitted → downloading → imported   (terminal)
       ↘ failed                                (terminal)
       ↘ cancelled                             (terminal, via cancelled event)
       ↘ unrouted                              (terminal; no rule matched)
```

- `queued` — row inserted, routing not yet attempted (transient).
- `submitted` — chosen *arr accepted; not yet seen in queue/history.
- `downloading` — present in /queue.
- `imported` — present in /history with successful import event, or *arr
  reports the title is already on disk.
- `failed` — chosen *arr returned a non-409 error, staleness threshold
  exceeded, or download-failed event seen in /history.
- `cancelled` — user cancelled upstream; *arr DELETE attempted (best-effort).
- `unrouted` — no enabled, kind-matching *arr's rules matched. **Terminal.**
  Distinct from `failed` so the requests UI can show a useful message
  ("no *arr accepted this") and so admins can find these in the queue.

## Configuration (global_config_schema)

| key                              | type    | required | default | desc                                                     |
|----------------------------------|---------|----------|---------|----------------------------------------------------------|
| `database_url`                   | string  | yes      |         | Postgres DSN for the `arrouter` schema                   |
| `tmdb.api_key`                   | secret  | yes      |         | TMDB v3 API key (for enrichment fields used in rules)    |
| `tmdb.language`                  | string  | no       | `en-US` | TMDB `language` query param for enrichment lookups       |
| `poll_interval_seconds`          | int     | no       | 30      | minimum 10, maximum 600                                  |
| `stale_after_hours`              | int     | no       | 72      | mark `failed` if stuck in submitted/downloading          |
| `secret_key`                     | secret  | yes      |         | symmetric key used to encrypt `registered_arr.api_key`   |

Per-*arr connection settings live in the `registered_arr` table, not in
global config — that's the whole point of this plugin.

## Multi-arr registry

The registry is a single table; routing reads it on every event. No caching
beyond Postgres' own; the table will be tiny (single-digit rows in any sane
deployment).

Admin operations (HTTP API + SPA):

- Create / update / delete a registered *arr.
- Toggle `enabled`.
- Change `priority`.
- Edit `rules_json` via the ported CollectionBuilder UI.
- "Test connection" → `GET {url}/api/v3/system/status` with the supplied key,
  show version + instance name on success.
- "Test rules" → enter a TMDB id + mediaType, run the routing pipeline in
  read-only mode, show which *arr won (or "unrouted") and the per-*arr
  match trace.

## Rules JSON shape

Adapted from silo's `QueryDefinition` (`web/src/api/types.ts` lines
977–1017) with **sort, limit, library_ids, and media_scope dropped**: an
*arr's `kind` is the implicit media filter, and there's nothing to sort or
paginate during routing.

```json
{
  "match": "all",                       // "all" | "any" — top-level combinator
  "groups": [
    {
      "match": "any",                   // "all" | "any" within the group
      "rules": [
        { "field": "original_language", "op": "eq",      "value": "ja" },
        { "field": "genres",            "op": "contains","value": "Animation" }
      ]
    },
    {
      "match": "all",
      "rules": [
        { "field": "year", "op": "gte", "value": 2000 }
      ]
    }
  ]
}
```

Top-level `match` combines the groups; each group's `match` combines its
rules. An empty `groups` array (the default) matches everything — useful for
a "catch-all" *arr at the lowest priority.

### Field vocabulary

Fields fall into three groups by where they come from. The rule editor
groups them visually the same way.

#### Group A — From the request event (always available, no TMDB call)

| field               | type    | source                                       |
|---------------------|---------|----------------------------------------------|
| `mediaType`         | string  | `"movie"` \| `"tv"`                          |
| `libraryId`         | string? | silo library the request was filed for |
| `year`              | int     | event payload                                |
| `decade`            | int     | derived: `year - (year % 10)`                |
| `requesterUserId`   | string  | event payload                                |
| `requesterIsAdmin`  | bool    | event payload                                |
| `title`             | string  | event payload                                |
| `tmdbId`            | int     | event payload                                |

#### Group B — From the primary TMDB call

One call per uncached request: `GET /movie/{id}` or `GET /tv/{id}`. All
Group B fields ride that single response — no per-field cost.

Common (movie + tv):

| field                  | type     | source                                                                 |
|------------------------|----------|------------------------------------------------------------------------|
| `original_language`    | string   | `.original_language`                                                   |
| `original_title`       | string   | movie: `.original_title`; tv: `.original_name`                         |
| `genres`               | string[] | `.genres[].name`                                                       |
| `runtime`              | int?     | movie: `.runtime`; tv: `.episode_run_time[0]` (or 0 if absent)         |
| `vote_average`         | float    | `.vote_average`                                                        |
| `vote_count`           | int      | `.vote_count`                                                          |
| `popularity`           | float    | `.popularity`                                                          |
| `adult`                | bool     | `.adult`                                                               |
| `status`               | string   | `.status` (e.g. `"Released"`, `"Returning Series"`, `"Ended"`)         |
| `production_companies` | string[] | `.production_companies[].name`                                         |
| `production_countries` | string[] | `.production_countries[].iso_3166_1` (e.g. `"US"`, `"KR"`, `"JP"`)     |
| `spoken_languages`     | string[] | `.spoken_languages[].iso_639_1` (e.g. `"en"`, `"ja"`)                  |

Movie-only:

| field                  | type    | source                                            |
|------------------------|---------|---------------------------------------------------|
| `release_date`         | string? | `.release_date` (ISO `YYYY-MM-DD`)                |
| `budget`               | int     | `.budget` (USD)                                   |
| `revenue`              | int     | `.revenue` (USD)                                  |
| `belongs_to_collection`| string? | `.belongs_to_collection.name` (or null)           |
| `imdb_id`              | string? | `.imdb_id` (e.g. `"tt0133093"`)                   |

TV-only:

| field                | type     | source                                              |
|----------------------|----------|-----------------------------------------------------|
| `networks`           | string[] | `.networks[].name`                                  |
| `origin_country`     | string[] | `.origin_country` (e.g. `["KR"]`)                   |
| `first_air_date`     | string?  | `.first_air_date` (ISO `YYYY-MM-DD`)                |
| `last_air_date`      | string?  | `.last_air_date` (ISO `YYYY-MM-DD`)                 |
| `type`               | string   | `.type` (`"Scripted"`, `"Documentary"`, `"Reality"`, `"Miniseries"`, etc.) |
| `in_production`      | bool     | `.in_production`                                    |
| `number_of_seasons`  | int      | `.number_of_seasons`                                |
| `number_of_episodes` | int      | `.number_of_episodes`                               |
| `created_by`         | string[] | `.created_by[].name`                                |

Cross-kind rules (e.g. a `networks` rule on a Radarr *arr's rules, or a
`belongs_to_collection` rule on a Sonarr *arr's rules) are allowed; the
field is treated as missing on the wrong kind, so referencing rules
evaluate false. The rule editor surfaces a warning but doesn't reject.

#### Group C — From secondary TMDB calls (lazy, fetched only if a rule references them)

These fields require an additional TMDB call per uncached request, so
arrouter only fetches them when at least one candidate *arr's rules
reference a Group C field. Each group is cached independently (24h TTL,
same `sync.Map` mechanism).

| field            | type     | TMDB endpoint                                                    |
|------------------|----------|------------------------------------------------------------------|
| `keywords`       | string[] | movie: `/movie/{id}/keywords` (`.keywords[].name`); tv: `/tv/{id}/keywords` (`.results[].name`) |
| `content_rating` | string?  | movie: `/movie/{id}/release_dates`, US `.release_dates[].certification` (first non-empty); tv: `/tv/{id}/content_ratings`, US `.results[].rating` |

`content_rating` resolves to the US certification by convention (`"PG-13"`,
`"R"`, `"TV-MA"`, etc.); admins who care about non-US ratings can file an
issue with a motivating use case. The lazy-fetch policy means an installation
that never uses `keywords` or `content_rating` pays no extra TMDB cost.

### Date-typed fields

`release_date`, `first_air_date`, and `last_air_date` are ISO 8601 strings.
Lexical and chronological order coincide for ISO dates, so they work with
`eq`, `ne`, `gt`, `gte`, `lt`, `lte`, and `between` (where `value` is
`[lowDate, highDate]`). The rule editor surfaces a date picker for these
fields but stores ISO strings in `rules_json`.

### Operators

| op             | applies to              | notes                                 |
|----------------|-------------------------|---------------------------------------|
| `eq`, `ne`     | scalar                  | string compare is case-insensitive    |
| `in`, `not_in` | scalar vs. array value  |                                       |
| `gt`, `gte`, `lt`, `lte` | numeric       |                                       |
| `between`      | numeric                 | `value` is `[low, high]` inclusive    |
| `contains`     | string or string array  | substring (string) / membership (array), case-insensitive |
| `starts_with`  | string                  | case-insensitive                      |
| `regex`        | string                  | RE2; rules with invalid regex fail closed (rule = false) and surface in match_trace |

Type mismatches (e.g. `gt` against a string) evaluate to false and are
recorded in the trace — never an error.

### Evaluation

1. Load enabled `registered_arr` rows where `kind` matches the event's
   `mediaType` (`movie → radarr`, `tv → sonarr`), ordered by `priority ASC, id ASC`.
2. Inspect the candidates' `rules_json` to determine which enrichment groups
   are needed: Group B is needed if any rule references a Group B field;
   Group C-keywords / C-content_rating each only if at least one rule
   references that specific field.
3. Lazily resolve only the needed enrichment groups (cache hit → free;
   miss → one TMDB call per group, populate cache).
4. For each candidate, evaluate `rules_json` against the merged context.
   First match wins.
5. If no candidate matches: status = `unrouted`, publish
   `plugin.silo.arrouter.unrouted`.

A request whose candidates all use only Group A fields incurs zero TMDB
calls. A request that triggers `keywords`-based routing incurs at most two
TMDB calls (primary + keywords) the first time, then nothing for 24h.

### TMDB enrichment cache

Three independent in-memory `sync.Map`s, each keyed by
`("movie"|"tv", tmdbId)` and holding `{ payload, fetchedAt }` with a 24h
TTL:

- **Primary** — `/movie/{id}` or `/tv/{id}` response (Group B).
- **Keywords** — `/movie/{id}/keywords` or `/tv/{id}/keywords` response.
- **Content ratings** — `/movie/{id}/release_dates` or `/tv/{id}/content_ratings` response.

No DB table, no cross-process sharing — the plugin runs single-process and
the cache is a latency optimization, not a source of truth. Each group is
populated on first reference; an installation that never uses Group C
fields never touches the keywords/ratings caches.

On TMDB error (any group): rule evaluation proceeds with the affected
fields treated as "missing" (rules referencing them evaluate false). The
event handling does **not** fail. The error is logged and surfaced in
match_trace.

## Event flow

### On `plugin.silo.requests.submitted`

Event payload:

```json
{
  "requestId": "...",
  "requesterUserId": "...",
  "requesterIsAdmin": false,
  "mediaType": "movie",
  "tmdbId": 603,
  "title": "...",
  "year": 1999,
  "posterUrl": "...",
  "libraryId": "..."
}
```

1. INSERT `arrouter.request` row, status=`queued`.
2. Run the routing pipeline (above).
3. **No match** → status=`unrouted`, completed_at=now(), match_trace stored,
   publish `plugin.silo.arrouter.unrouted`. **Stop.**
4. **Match** → record `routed_arr_id`, then POST to the chosen *arr:
   - Movie: `POST {url}/api/v3/movie`
     ```json
     {
       "title": "...", "tmdbId": 603, "year": 1999,
       "qualityProfileId": <registered or first>,
       "rootFolderPath": "<registered>",
       "monitored": true,
       "minimumAvailability": "announced",
       "addOptions": {"searchForMovie": true}
     }
     ```
   - TV: `POST {url}/api/v3/series`
     ```json
     {
       "title": "...", "tmdbId": 603, "year": 1999,
       "qualityProfileId": <registered or first>,
       "languageProfileId": <registered or first>,
       "rootFolderPath": "<registered>",
       "monitored": true,
       "seasonFolder": true,
       "addOptions": {"searchForMissingEpisodes": true, "monitor": "all"}
     }
     ```
5. On success: status=`submitted`, capture `external_id`, publish
   `plugin.silo.arrouter.submitted`.
6. On *arr error:
   - `409` "already exists" → status=`submitted`, treat as already there,
     publish `plugin.silo.arrouter.submitted`.
   - Other 4xx/5xx → status=`failed`, error = body, publish
     `plugin.silo.arrouter.failed`.
   - Network unreachable → status=`failed` after one immediate retry; do
     **not** fall through to the next-priority *arr (the rule said this *arr
     should handle it; falling through would silently route to a different
     library and is a footgun).

Sonarr: if `tmdbId` lookup yields no series in
`/api/v3/series/lookup?term=tmdb:N`, fall back to title/year search and pick
the first hit.

### On `plugin.silo.requests.cancelled`

Event payload:

```json
{ "requestId": "...", "requesterUserId": "..." }
```

1. SELECT row WHERE id=requestId.
2. If not found, or terminal (imported/failed/cancelled/unrouted): no-op.
3. If status IN ('submitted','downloading') and `external_id` is set:
   - Movie: `DELETE {url}/api/v3/movie/{external_id}?deleteFiles=false&addImportListExclusion=false`
   - TV: `DELETE {url}/api/v3/series/{external_id}?deleteFiles=false&addImportListExclusion=false`
   - URL/key resolved via `routed_arr_id`. If the registered *arr was
     deleted (`routed_arr_id IS NULL`), skip the DELETE.
4. UPDATE status=`cancelled`, completed_at=now().
5. Publish `plugin.silo.arrouter.cancelled`.

### Periodic poll (every `poll_interval_seconds`)

For each row with status IN ('submitted','downloading') AND
`routed_arr_id IS NOT NULL`:

1. Resolve the registered *arr (skip if missing or `enabled=false`; do not
   transition status — admin may re-enable).
2. **Movie path** (Radarr):
   - `GET /api/v3/movie/{external_id}` → `hasFile`?
     - If yes → status=`imported`, completed_at=now(); publish imported.
   - `GET /api/v3/queue?movieId={external_id}` → in queue?
     - If yes → status=`downloading`; publish downloading **only on transition**.
   - Otherwise: if `submitted_at < now() - stale_after_hours` →
     status=`failed`; publish failed.
3. **TV path** (Sonarr):
   - `GET /api/v3/series/{external_id}` → `statistics.percentOfEpisodes`?
     - If `100` → status=`imported`, completed_at=now(); publish imported.
   - `GET /api/v3/queue?seriesId={external_id}` → in queue?
     - status=`downloading` if so.
   - Stale check same as movie.
4. Update `last_polled_at` on every iteration regardless of transition.

Polling fans out per-*arr in parallel (one goroutine per registered *arr) so
a slow instance can't starve the others; per-*arr inner loop is sequential.

### Events arrouter publishes

| Event                                    | When                                              |
|------------------------------------------|---------------------------------------------------|
| `plugin.silo.arrouter.submitted`    | After successful POST to chosen *arr             |
| `plugin.silo.arrouter.downloading`  | First time row enters `downloading`              |
| `plugin.silo.arrouter.imported`     | Row reaches `imported`                            |
| `plugin.silo.arrouter.failed`       | Submission error or staleness                    |
| `plugin.silo.arrouter.cancelled`    | After cancel (whether *arr DELETE succeeded)     |
| `plugin.silo.arrouter.unrouted`     | Routing produced no match (terminal)             |

Payloads include request id, timestamps, and an optional error. `unrouted` adds
an optional `reason` string, typically `"no registered *arr matched"`.

## Silo.requests changes

The requests plugin subscribes to Arr Request Router status events so it can
mirror routing outcomes:

- `plugin.silo.arrouter.submitted`
- `plugin.silo.arrouter.downloading`
- `plugin.silo.arrouter.imported`
- `plugin.silo.arrouter.failed`
- `plugin.silo.arrouter.cancelled`
- `plugin.silo.arrouter.unrouted`

`plugin.silo.arrouter.unrouted` is surfaced as a normal failed request
with reason `"no registered *arr matched"`.

## Admin HTTP routes

All `/admin*` and `/api/admin/*` routes require admin
(`X-Silo-User-Role: admin`, the standard plugin proxy header).

### Pages (SPA)

- `GET /admin` — registry list. Each row: name, kind pill, url, priority,
  enabled toggle, "edit" / "delete" / "test connection".
- `GET /admin/registry/new`, `GET /admin/registry/{id}` — registry editor.
  Connection panel + the ported CollectionBuilder for `rules_json`.
  Includes the "Test rules" panel (TMDB id + media type → routing trace).
- `GET /admin/queue` — recent requests with status pills, error details, and
  the routed *arr's name. Includes a "retry" action for `failed` rows and
  a "re-route" action for `unrouted` rows (re-runs the routing pipeline as
  if a fresh event had arrived).

### JSON API

- `GET    /api/admin/registry` — list, ordered by `kind, priority`.
- `POST   /api/admin/registry` — create.
- `GET    /api/admin/registry/{id}` — fetch one.
- `PATCH  /api/admin/registry/{id}` — update.
- `DELETE /api/admin/registry/{id}` — delete (soft? — no; hard delete, but
  `request.routed_arr_id` is `ON DELETE SET NULL` so history survives).
- `POST   /api/admin/registry/{id}/test-connection` → `GET /system/status` on
  the *arr.
- `POST   /api/admin/route-test` — body `{ tmdbId, mediaType }`; returns
  `{ chosen: <id>|null, trace: [...] }` without writing anything.
- `GET    /api/admin/requests` — paginated list. Query: `status`, `page`, `limit`.
- `GET    /api/admin/requests/{id}` — single row + match trace + audit.
- `POST   /api/admin/requests/{id}/retry` — for `failed` rows: re-attempt
  POST to the originally-routed *arr; flip status back to `queued`.
- `POST   /api/admin/requests/{id}/re-route` — for `unrouted` rows: re-run
  the routing pipeline and proceed as if the request had just arrived.

### Rule editor: CollectionBuilder port

We **port** silo's `web/src/components/collections/CollectionBuilder.tsx`
into this plugin's `web/` rather than re-implementing a guided builder from
scratch. The QueryDefinition shape is structurally compatible after dropping
sort/limit/library_ids/media_scope; the field vocabulary is what changes
most.

Port scope:

- Copy `CollectionBuilder.tsx`, its test, and any small helpers it depends
  on (group/rule edit components, operator-set definitions).
- Strip features irrelevant to routing: sort controls, limit, media_scope
  toggle, library picker.
- Replace the field catalog with arrouter's vocabulary (Groups A/B/C from
  the field-vocabulary section). The field picker groups options by source
  and tags Group C fields with a "(extra TMDB call)" hint so admins can
  see the cost.
- Per-field operator restrictions enforce sensible combinations:
  string-array fields (`genres`, `networks`, `keywords`,
  `production_companies`, `production_countries`, `spoken_languages`,
  `origin_country`, `created_by`) accept `contains` / `in` / `not_in`;
  numeric fields hide string-only operators; bool fields expose only
  `eq` / `ne`; date fields surface a date picker but store ISO strings.
- Field availability follows kind: when editing a Radarr *arr, the picker
  hides TV-only fields (`networks`, `origin_country`, etc.) and vice versa.
  Cross-kind rules already in `rules_json` are still rendered (with a
  warning) so legacy data never disappears.
- The "preview matches" affordance from CollectionBuilder is replaced by
  the "Test rules" panel (TMDB-id-driven, server-side).

Visual / UX should match silo's existing CollectionBuilder closely —
admins moving between silo's collections and arrouter's rules should
feel they're using the same tool.

## SPA theme handling

Same standing rule as every silo plugin SPA: the prerender / SPA
handler must inject `data-theme="<theme>"` onto `<html>` at request time.
Resolve the theme from the `?theme=...` query param silo's sidebar
appends (see `silo/web/src/components/AppSidebar.tsx` `buildPluginHref`)
or from silo's injected header. Do **not** hardcode a default theme —
fall back to whatever silo sends, with a single neutral fallback only
if no theme is supplied. Pull in the same CSS custom-property values
silo uses, via the ported design tokens in this plugin's `index.css`.

Test by hitting `/plugins/{id}/admin` from a browser session where the user
has a non-default theme set, and verify `<html data-theme="...">` matches.

## Error handling

- **TMDB enrichment fails** (network, 4xx, 5xx, rate-limit): proceed with
  enriched fields treated as missing; rules referencing them evaluate false;
  log + record in match_trace; do not fail the event.
- **Chosen *arr returns non-409 4xx/5xx on submit**: status=`failed`. Do
  **not** try the next-priority *arr (intentional — see "Event flow" §6).
- **Chosen *arr unreachable on poll**: leave row alone, increment
  last_polled_at, log. Staleness check still applies (an *arr that's been
  down for 3 days will eventually mark its rows `failed`).
- **Registered *arr deleted while a request is in flight**:
  `routed_arr_id` becomes NULL, polling skips the row. Admin can manually
  fail the orphaned row from the queue page.
- **Invalid `rules_json`** (malformed JSON, unknown field, unknown op):
  validated on save (PATCH/POST returns 400); already-stored bad data
  evaluates as "rule false" with a trace entry, never panics.
- **Cancelled event for an unknown request id**: no-op (idempotent).

## Testing

- **Unit**: rule evaluator (one table-driven test per operator + edge cases:
  case-insensitivity, type mismatch → false, regex compile failure → false,
  date comparison via ISO strings, missing-field-after-TMDB-failure → false);
  per-group TMDB cache TTL behavior; lazy-fetch decision (rules using only
  Group A → no TMDB calls; rules using Group B → primary call only; rules
  using `keywords` → primary + keywords, but not content_ratings);
  encryption round-trip for api_key.
- **Integration** (real Postgres, fake *arrs + fake TMDB via httptest):
  - Event → row inserted, status transitions correctly through the lifecycle.
  - Multiple registered *arrs, priority order respected.
  - First match wins; second-priority *arr never receives POST.
  - No match → `unrouted`, event published.
  - 409 on submit → treated as `submitted`.
  - Cancel before submit (queued) → DELETE not attempted.
  - Cancel after submit → DELETE attempted on the originally-routed *arr.
  - Poll transitions: queued in *arr, hasFile, staleness.
  - Registered *arr disabled mid-flight → poll skips, no status change.
  - Registered *arr deleted mid-flight → poll skips, status unchanged.
  - Group C field referenced → secondary TMDB call observed; not referenced → not observed.
  - TMDB primary call fails → routing still proceeds, Group B rules
    evaluate false, match_trace records the error.
- **Rule editor**: keep CollectionBuilder's existing tests; adapt for the
  trimmed schema and arrouter field vocabulary.
- **Manual**: admin SPA against a real Radarr + Sonarr in dev compose,
  including theme-injection check across two themes.

## Open questions / out of scope

- **Tag-based routing on the *arr side** (writing tags to Radarr/Sonarr based
  on which rule matched): out of scope for v1; admins can still set tags
  manually inside the *arr.
- **Per-rule overrides** of `quality_profile_id` / `root_folder_path`: out of
  scope for v1. Use multiple registered *arrs (same instance, different
  config rows, different rules) if you need this — slightly clumsy but
  unblocks the use case.
- **Round-robin / weighted routing within a tier**: out of scope. Priority is
  strictly a tiebreaker; if you need round-robin, file an issue with a
  motivating use case.
