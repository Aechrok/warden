CREATE TYPE hold_status AS ENUM ('active', 'released', 'expired');

CREATE TABLE hold_templates (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name                TEXT NOT NULL UNIQUE,
  description         TEXT,
  provider_glob       TEXT NOT NULL DEFAULT '*',
  blocked_actions     TEXT[] NOT NULL DEFAULT '{}',
  expiration_days     INT,
  notes_template      TEXT,
  created_by          UUID REFERENCES users(id),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE legal_holds (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name          TEXT NOT NULL,
  description   TEXT,
  template_id   UUID REFERENCES hold_templates(id) ON DELETE SET NULL,
  status        hold_status NOT NULL DEFAULT 'active',
  placed_by     UUID REFERENCES users(id),
  expires_at    TIMESTAMPTZ,
  released_at   TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE legal_hold_custodians (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  hold_id        UUID NOT NULL REFERENCES legal_holds(id) ON DELETE CASCADE,
  email          TEXT NOT NULL,
  added_by       UUID REFERENCES users(id),
  removed_at     TIMESTAMPTZ,
  removed_by     UUID REFERENCES users(id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(hold_id, email)
);

CREATE TABLE legal_hold_blocked_actions (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  plugin_id        TEXT NOT NULL,
  action_key       TEXT NOT NULL,
  description      TEXT,
  UNIQUE(plugin_id, action_key)
);
