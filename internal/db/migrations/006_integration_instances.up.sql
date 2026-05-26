CREATE TABLE integration_instances (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name             TEXT NOT NULL UNIQUE,
  plugin_id        TEXT NOT NULL,
  credentials_enc  BYTEA,
  is_active        BOOLEAN NOT NULL DEFAULT true,
  last_health_ok   BOOLEAN,
  last_health_at   TIMESTAMPTZ,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
