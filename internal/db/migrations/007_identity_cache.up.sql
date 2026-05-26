CREATE TABLE identity_cache (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email       TEXT NOT NULL,
  instance_id UUID NOT NULL REFERENCES integration_instances(id) ON DELETE CASCADE,
  data        JSONB NOT NULL,
  fetched_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  ttl_seconds INT NOT NULL DEFAULT 900,
  UNIQUE(email, instance_id)
);

CREATE INDEX idx_identity_cache_email ON identity_cache(email);
