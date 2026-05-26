CREATE TYPE approval_status AS ENUM ('pending', 'approved', 'rejected', 'expired');

CREATE TABLE approval_requests (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  requester_id  UUID NOT NULL REFERENCES users(id),
  action_key    TEXT NOT NULL,
  instance_id   UUID REFERENCES integration_instances(id),
  target_email  TEXT,
  params        JSONB NOT NULL DEFAULT '{}',
  reason        TEXT,
  status        approval_status NOT NULL DEFAULT 'pending',
  reviewer_id   UUID REFERENCES users(id),
  review_note   TEXT,
  expires_at    TIMESTAMPTZ NOT NULL,
  reviewed_at   TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_approvals_status ON approval_requests(status) WHERE status = 'pending';
