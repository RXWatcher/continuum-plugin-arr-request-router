# Routing rules reference

The rule engine lives in `internal/routing`. This document is the definitive
reference for the JSON shape stored in `registered_arr.rules_json`, the
evaluation order, every supported field, every supported operator, and the
common pitfalls that produce surprising routing outcomes.

## JSON shape

```jsonc
{
  "match": "all" | "any",      // top-level combinator across groups
  "groups": [
    {
      "match": "all" | "any",  // combinator across rules inside this group
      "rules": [
        { "field": "<name>", "op": "<operator>", "value": <json> },
        ...
      ]
    },
    ...
  ]
}
```

Notes on parsing (`routing.ParseRules`):

- Empty input (`nil`/`""`) and `{}` are tolerated — both produce
  `Match=all, Groups=nil`, which matches **every** request.
- Missing `match` on the top level or any group defaults to `"all"`. So you
  can omit it for the common case.
- An empty `groups` array also matches everything (the evaluator short-circuits
  before iterating).
- `ValidateRules` is called by the admin handlers on create/update; rules are
  rejected if they reference an unknown field or operator, or if the
  combinator is not `all`/`any`. Stored rules can be assumed valid at
  evaluation time.

## Evaluation order

1. **Filter by kind.** `movie` requests only see `kind="radarr"` candidates;
   `tv` only `kind="sonarr"`. An unknown `mediaType` yields zero candidates →
   the request is `unrouted`.
2. **Sort.** `(priority ASC, id ASC)` — performed by SQL, not the evaluator.
3. **Lazy enrichment.** The router scans all surviving candidates' rules,
   sums the field groups they reference, and calls only the TMDB methods that
   could change an outcome. See "TMDB enrichment" below.
4. **First match wins.** Candidates are walked in order; the first whose
   top-level `match` resolves true is chosen. No further candidates are
   evaluated. The router does **not** fall through to lower-priority candidates
   if dispatch later fails.
5. **No match → unrouted.** The request row goes to status `unrouted` with the
   full trace persisted into `match_trace`.

The evaluator iterates every rule even when the combinator short-circuits.
This is deliberate — the per-rule trace is the operator's primary debugging
tool, so partial traces would be much less useful than the small extra
evaluation cost.

## Field reference

Fields are partitioned into groups; the group dictates which TMDB call (if
any) is required to materialise the value.

### Group A — request event payload

Always available; no external calls.

| Field | Type | Source |
| --- | --- | --- |
| `mediaType` | string (`"movie"`/`"tv"`) | event |
| `libraryId` (alias: `library`) | string | event |
| `year` | int | event |
| `decade` | int | derived: `year - year%10` |
| `title` | string | event |
| `tmdbId` | int | event |
| `requesterUserId` | string | event |
| `requesterIsAdmin` | bool | event |

### Group B — primary TMDB lookup

Loaded by `GET /movie/{id}` or `GET /tv/{id}`. Any field references in this
group triggers exactly one Primary call (cached for the TTL).

Common to both kinds:

| Field | Type | Notes |
| --- | --- | --- |
| `original_language` | string | ISO-639-1, e.g. `"en"`, `"ja"` |
| `original_title` | string | For TV this maps from `original_name` |
| `genres` | string[] | Names, e.g. `["Action","Drama"]` |
| `runtime` | int | Movies: minutes; TV: first entry of `episode_run_time` |
| `vote_average` | float | |
| `vote_count` | int | |
| `popularity` | float | |
| `adult` | bool | |
| `status` | string | TMDB status text |
| `production_companies` | string[] | Company names |
| `production_countries` | string[] | ISO-3166-1 codes |
| `spoken_languages` | string[] | ISO-639-1 codes |

Movie-only (kind-guarded; on TV context these resolve to "missing"):

- `release_date` (string, ISO-8601)
- `budget` (int, USD)
- `revenue` (int, USD)
- `belongs_to_collection` (string — collection name, empty when none)
- `imdb_id` (string)

TV-only (kind-guarded; on movie context these resolve to "missing"):

- `networks` (string[])
- `origin_country` (string[])
- `first_air_date`, `last_air_date` (strings, ISO-8601)
- `type` (string — e.g. `"Scripted"`, `"Documentary"`)
- `in_production` (bool)
- `number_of_seasons`, `number_of_episodes` (ints)
- `created_by` (string[] — creator names)

The kind guard is silent: a movie-only field on a TV request returns
`(nil, false)` from `GetField`, and the rule evaluator records
`note: "missing"`. The rule cannot match, but the candidate is **not**
disqualified for that alone — surrounding combinators decide.

### Group C — secondary TMDB lookups

Each field in this group triggers its own dedicated TMDB call, only when
referenced.

| Field | Type | TMDB endpoint |
| --- | --- | --- |
| `keywords` | string[] | `/movie/{id}/keywords` (movie key `keywords`) or `/tv/{id}/keywords` (key `results`) |
| `content_rating` | string | `/movie/{id}/release_dates` (first non-empty US certification) or `/tv/{id}/content_ratings` (US `rating`) |

For `content_rating`, only the US entry is consulted. If none exists or the
certification is empty, the field resolves to "missing" (the API call itself
succeeds — the empty string is intentionally treated as "not available").

## Operator reference

The full operator set lives in `internal/routing/operators.go`. Strings are
**case-insensitive** wherever they participate.

| Operator | Actual type expected | Value shape | Semantics |
| --- | --- | --- | --- |
| `eq` | string / number / bool | scalar | Case-insensitive for strings; exact for bool; float-coerced for numbers. Type mismatch (e.g. string vs number) → no match with `type mismatch` note. |
| `ne` | same as `eq` | scalar | Inverse of `eq`. |
| `in` | string / number / bool | array of scalars | True iff actual equals any element by `eq` semantics. |
| `not_in` | same as `in` | array | Inverse of `in`. |
| `gt`, `gte`, `lt`, `lte` | number | number | Numeric ordering. Non-numeric actual → `type mismatch`. |
| `between` | number | `[low, high]` (inclusive) | Both bounds required and numeric. |
| `contains` | string OR string[] | string | String: substring (case-insensitive). Array: membership (case-insensitive equality). |
| `starts_with` | string | string | Case-insensitive prefix match. |
| `regex` | string | string (RE2 pattern) | Go RE2 syntax. Invalid pattern → no match with `invalid regex: <err>` note. Never panics. |

### Type coercion rules

- Numeric actuals (int, int64, float64, `json.Number`) are coerced to
  `float64` before comparison. `"5" == 5` is **not** equal — string vs number
  is a `type mismatch`.
- String compares are always case-insensitive. There is no case-sensitive
  variant.
- `contains` and `in` are the two array-aware operators. Use `contains` when
  the field is a list (`genres`, `keywords`, `networks`, …) and you have a
  single needle. Use `in` when the field is a scalar and you have a list of
  acceptable values.

## Diagnostic trace shape

Every routing decision (production submit and admin route-test) produces a
`routing.Trace`, persisted to `request.match_trace` (JSONB) and returned by
`POST /api/admin/route-test`:

```jsonc
{
  "tmdb_primary_error":   "tmdb /movie/603: 404",   // omitted if no error
  "keywords_error":       "...",                    // omitted if no error
  "content_rating_error": "...",                    // omitted if no error
  "chosen_arr_id": 7,                               // omitted on unrouted
  "candidates": [
    {
      "arr_id": 3,
      "arr_name": "Radarr 4K",
      "match": "all",
      "matched": false,
      "groups": [
        {
          "match": "any",
          "matched": false,
          "rules": [
            { "field": "genres", "op": "contains", "matched": false,
              "note": "" },
            { "field": "runtime", "op": "gt", "matched": false,
              "note": "type mismatch" }
          ]
        }
      ]
    },
    { "arr_id": 7, "arr_name": "Radarr 1080p", "match": "all",
      "matched": true, "groups": [...] }
  ]
}
```

Note the iteration is exhaustive within `candidates` only up to the first
match — entries after `chosen_arr_id` are not evaluated and are not present.

`note` values you will see:

- `"missing"` — field unknown, or the field's data group failed to load (most
  often a TMDB error or a kind-only field on the wrong media type).
- `"type mismatch"` — operator could not coerce the value (or the value JSON
  did not decode into the expected shape).
- `"invalid regex: ..."` — the rule's value did not parse as RE2.
- `"unknown op: ..."` — should never appear post-validate; would indicate a
  rule was bypassed validation.

## Common pitfalls

- **Catch-all priority.** Put the empty-rules catch-all at the **highest**
  numeric priority (lowest preference). `(priority ASC, id ASC)` sort means
  small priority numbers are evaluated first. A catch-all at priority 0 will
  swallow everything before any specific rule fires.
- **Missing TMDB metadata silently disables rules.** A 404 from TMDB sets
  `tmdb_primary_error` and every Group B / Group C rule in this evaluation
  resolves to "missing". With `match: "all"` that means the candidate cannot
  match; with `match: "any"` other rules can still rescue it. Operators who
  expect TMDB to be optional often want to phrase rules so that the catch-all
  still has a chance.
- **`secret_key` rotation invalidates every stored API key.** Once rotated,
  every arr will fail `crypto.Open` until you re-enter its API key via PATCH.
  See [`arr-registry.md`](arr-registry.md) for the recovery procedure.
- **`contains` on a single-string field is substring, not equality.** Use
  `eq` if you mean equality. Use `contains` on lists or for "title contains
  the word foo".
- **Numeric strings are not numbers.** TMDB-derived `imdb_id` is a string
  like `"tt0133093"`. Comparing it with `gt`/`lt` etc. produces
  `type mismatch`. Use `regex` or `starts_with` instead.
- **Empty TV-only fields on a movie request.** `networks`, `created_by`,
  `number_of_seasons` etc. always resolve to "missing" for movies. Don't put
  them inside a group that uses `match: "all"` on a Radarr — that group will
  always be false.
- **Case folding can surprise.** "EN" matches "en", "Drama" matches "drama".
  If you want exact case-sensitive matching you cannot get it from the
  built-in operators; pre-normalise externally.
- **Aliases.** `library` is a deprecated alias for `libraryId`. Use
  `libraryId` in new rules; both will continue to resolve to the same value.

## Worked examples

A 4K-target rule keyed on a library id:

```json
{
  "match": "all",
  "groups": [
    {
      "match": "all",
      "rules": [
        { "field": "libraryId", "op": "eq", "value": "movies-4k" }
      ]
    }
  ]
}
```

Foreign-language movies (anything non-English) at higher priority than the
default:

```json
{
  "match": "all",
  "groups": [
    {
      "match": "all",
      "rules": [
        { "field": "original_language", "op": "ne", "value": "en" }
      ]
    }
  ]
}
```

Anime via Sonarr (Japanese origin OR anime keyword OR network in a known list):

```json
{
  "match": "all",
  "groups": [
    {
      "match": "any",
      "rules": [
        { "field": "origin_country", "op": "contains", "value": "JP" },
        { "field": "keywords",       "op": "contains", "value": "anime" },
        { "field": "networks",       "op": "in",
          "value": ["Crunchyroll", "Tokyo MX", "TV Tokyo"] }
      ]
    }
  ]
}
```

Adult-only target gated on requester admin flag AND TMDB adult bit:

```json
{
  "match": "all",
  "groups": [
    {
      "match": "all",
      "rules": [
        { "field": "requesterIsAdmin", "op": "eq", "value": true },
        { "field": "adult",            "op": "eq", "value": true }
      ]
    }
  ]
}
```

Catch-all (the lowest-preference target — give this the **highest** priority
number):

```json
{}
```
