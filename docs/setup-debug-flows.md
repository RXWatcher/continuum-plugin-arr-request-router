# Arr Request Router Setup, Debugging, And Flows

Plugin ID: `continuum.arrouter`
Version documented: `0.1.0`

## Purpose

rule-based request router that sends Continuum request events to the right Radarr/Sonarr
instance.

## Runtime Dependencies

- Continuum plugin host
- Postgres schema for this plugin
- continuum.requests
- TMDB API access for metadata-aware rules
- One or more Radarr/Sonarr targets configured in the admin UI

## Setup Checklist

1. Create the plugin schema and configure database_url.
2. Configure tmdb.api_key, poll interval, stale timeout, and secret_key.
3. Open the plugin admin UI and define Radarr/Sonarr targets and routing rules.
4. Approve representative movie and TV requests.
5. Verify the selected target, root path, quality profile, and final status in the admin UI.

## Configuration Reference

- `database_url`
- `tmdb.api_key`
- `tmdb.language`
- `poll_interval_seconds`
- `stale_after_hours`
- `secret_key`

Use the plugin manifest/admin form as the source of truth for field validation and defaults. Keep database credentials scoped to the plugin schema unless a plugin explicitly needs read access to Continuum core tables.

## Exposed Routes

- `* /api/admin/* [admin]`
- `GET /assets/* [public]`
- `GET /admin/* [admin]`

## Capabilities

- `event_consumer.v1 (router) - Routes request events to a registered Radarr/Sonarr based on rules.`
- `scheduled_task.v1 (poll) - Polls registered *arrs for download/import progress.`
- `http_routes.v1 (admin) - Admin SPA for the Arr Request Router plugin.`
- `request_router.v1 (default) - Routes requests to one of N registered Radarr/Sonarr instances based on rules.`

## Operational Flows

### Rule routing

1. continuum.requests emits an approved request.
2. The router loads request metadata and evaluates configured routing rules.
3. The selected target receives the add/request call.
4. The poll task checks for completion/failure and emits status updates.

## How This Plugin Communicates

- Consumes request events from continuum.requests.
- Calls Radarr/Sonarr targets directly.
- Publishes fulfillment events back to continuum.requests and can be observed by notifications.

## Debugging Runbook

- If no target is selected, inspect rule order and default fallback rules.
- If metadata-dependent rules fail, verify tmdb.api_key and network access.
- Check secret_key length; encrypted fields need a stable key across restarts.
- Use the admin UI to inspect delivery attempts and upstream response bodies.
- Check poll_interval_seconds and stale_after_hours when requests remain pending.

## Log And Health Checks

- Start with Continuum Admin -> Plugins and confirm the installation is enabled.
- Check the plugin process logs around startup for manifest loading, migration, and route registration.
- Check scheduled task logs when a workflow depends on polling or reconciliation.
- Confirm the plugin routes are reachable through Continuum using the access level shown above.
- For database-backed plugins, verify the configured role can connect, create/migrate tables in its schema, and read/write expected rows.

## Common Failure Patterns

- Wrong installation ID selected in a portal or router setting after reinstalling a plugin.
- Plugin database URL points at the public schema instead of the dedicated plugin schema.
- Reverse proxy forwards the SPA route but not `/api/*`, `/api/v1/*`, `/assets/*`, or provider-specific public routes.
- Network checks are run from the operator laptop instead of from the Continuum/plugin runtime network.
- Secrets are regenerated during restart, invalidating signed URLs, encrypted fields, or login state.

## Verification After Changes

1. Restart or reload the plugin installation.
2. Open the plugin route or admin page in Continuum.
3. Exercise the smallest workflow that crosses a plugin boundary.
4. Confirm both the source plugin and destination plugin record the same request/session/login identifier.
5. Leave the scheduled reconciler enough time to run, then confirm terminal state or a useful error.
