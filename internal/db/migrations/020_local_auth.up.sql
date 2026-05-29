ALTER TABLE users ADD COLUMN password_hash text;

ALTER TABLE sso_config
    ADD COLUMN sso_enabled  bool NOT NULL DEFAULT false,
    ADD COLUMN enforce_sso  bool NOT NULL DEFAULT false;
