CREATE TYPE outbox_status AS ENUM ('pending', 'claimed', 'done');

CREATE TABLE outbox (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  topic        TEXT NOT NULL,
  payload      JSONB NOT NULL,
  status       outbox_status NOT NULL DEFAULT 'pending',
  attempts     INT NOT NULL DEFAULT 0,
  last_error   TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  claimed_at   TIMESTAMPTZ,
  done_at      TIMESTAMPTZ
);

CREATE INDEX idx_outbox_status ON outbox(status) WHERE status != 'done';
