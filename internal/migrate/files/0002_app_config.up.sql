CREATE TABLE IF NOT EXISTS app_config (
  id         integer PRIMARY KEY DEFAULT 1,
  data       jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT app_config_singleton CHECK (id = 1)
);

INSERT INTO app_config (id, data)
VALUES (1, '{}'::jsonb)
ON CONFLICT (id) DO NOTHING;
