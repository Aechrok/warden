ALTER TABLE users DROP COLUMN IF EXISTS password_hash;

ALTER TABLE sso_config
    DROP COLUMN IF EXISTS sso_enabled,
    DROP COLUMN IF EXISTS enforce_sso;
