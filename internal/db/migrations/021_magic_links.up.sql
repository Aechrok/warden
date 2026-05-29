CREATE TABLE magic_links (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    token      text        UNIQUE NOT NULL DEFAULT encode(gen_random_bytes(32), 'hex'),
    email      text        NOT NULL,
    role_name  text        REFERENCES roles(name) ON DELETE SET NULL,
    invited_by uuid        REFERENCES users(id) ON DELETE SET NULL,
    label      text        NOT NULL DEFAULT '',
    used_at    timestamptz,
    expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days',
    created_at timestamptz NOT NULL DEFAULT now()
);
