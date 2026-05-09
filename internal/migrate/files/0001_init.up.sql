CREATE TABLE registered_arr (
  id                  BIGSERIAL PRIMARY KEY,
  name                TEXT NOT NULL,
  kind                TEXT NOT NULL CHECK (kind IN ('radarr','sonarr')),
  url                 TEXT NOT NULL,
  api_key             TEXT NOT NULL,
  root_folder_path    TEXT NOT NULL,
  quality_profile_id  INTEGER,
  language_profile_id INTEGER,
  priority            INTEGER NOT NULL DEFAULT 100,
  enabled             BOOLEAN NOT NULL DEFAULT true,
  rules_json          JSONB NOT NULL DEFAULT '{"match":"all","groups":[]}'::jsonb,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX registered_arr_kind_priority_idx
  ON registered_arr (kind, priority) WHERE enabled;

CREATE TABLE request (
  id                  TEXT PRIMARY KEY,
  tmdb_id             INTEGER NOT NULL,
  media_type          TEXT NOT NULL CHECK (media_type IN ('movie','tv')),
  title               TEXT NOT NULL,
  year                INTEGER NOT NULL DEFAULT 0,
  poster_url          TEXT,
  requester_user_id   TEXT NOT NULL,
  requester_is_admin  BOOLEAN NOT NULL DEFAULT false,
  status              TEXT NOT NULL CHECK (status IN
    ('queued','submitted','downloading','imported','failed','cancelled','unrouted')),
  routed_arr_id       BIGINT REFERENCES registered_arr(id) ON DELETE SET NULL,
  external_id         INTEGER,
  error               TEXT,
  match_trace         JSONB,
  submitted_at        TIMESTAMPTZ,
  last_polled_at      TIMESTAMPTZ,
  completed_at        TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX request_status_idx ON request (status)
  WHERE status IN ('submitted','downloading');
CREATE INDEX request_tmdb_idx ON request (tmdb_id, media_type);
CREATE INDEX request_routed_arr_idx ON request (routed_arr_id)
  WHERE status IN ('submitted','downloading');
