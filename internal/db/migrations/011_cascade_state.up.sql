CREATE TYPE cascade_status AS ENUM ('pending', 'in_progress', 'completed', 'partial', 'failed');

CREATE TABLE cascade_state (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  hold_id         UUID NOT NULL REFERENCES legal_holds(id) ON DELETE CASCADE,
  custodian_email TEXT NOT NULL,
  instance_id     UUID NOT NULL REFERENCES integration_instances(id),
  status          cascade_status NOT NULL DEFAULT 'pending',
  last_error      TEXT,
  attempts        INT NOT NULL DEFAULT 0,
  completed_at    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(hold_id, custodian_email, instance_id)
);

CREATE INDEX idx_cascade_state_hold ON cascade_state(hold_id);
