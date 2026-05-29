CREATE TABLE sso_config (
    singleton            bool        PRIMARY KEY DEFAULT true CHECK (singleton),
    oidc_issuer          text        NOT NULL DEFAULT '',
    oidc_internal_issuer text        NOT NULL DEFAULT '',
    oidc_client_id       text        NOT NULL DEFAULT '',
    oidc_credentials_enc bytea,
    oidc_redirect_url    text        NOT NULL DEFAULT '',
    updated_at           timestamptz NOT NULL DEFAULT now()
);

INSERT INTO sso_config DEFAULT VALUES;
