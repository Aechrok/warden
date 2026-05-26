CREATE TABLE breakglass_incidents (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  operator_id   UUID NOT NULL REFERENCES users(id),
  action_key    TEXT NOT NULL,
  instance_id   UUID REFERENCES integration_instances(id),
  target_email  TEXT,
  reason        TEXT NOT NULL,
  reviewed_by   UUID REFERENCES users(id),
  reviewed_at   TIMESTAMPTZ,
  review_note   TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
