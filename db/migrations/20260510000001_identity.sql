-- Identity tables: users + auth_methods + opaque single-use tokens.

CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email             CITEXT NOT NULL UNIQUE,
    email_verified_at TIMESTAMPTZ,
    name              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX users_status_idx ON users(status);

CREATE TABLE auth_methods (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('password','google')),
    password_hash    TEXT,
    provider_subject TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ,
    CHECK (
        (provider = 'password' AND password_hash IS NOT NULL AND provider_subject IS NULL)
        OR
        (provider = 'google'   AND provider_subject IS NOT NULL AND password_hash IS NULL)
    )
);

CREATE UNIQUE INDEX auth_methods_user_provider_uniq
    ON auth_methods(user_id, provider);
CREATE UNIQUE INDEX auth_methods_provider_subject_uniq
    ON auth_methods(provider, provider_subject)
    WHERE provider_subject IS NOT NULL;

CREATE TABLE email_verify_tokens (
    token_hash  BYTEA PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email       CITEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX email_verify_tokens_user_active_idx
    ON email_verify_tokens(user_id) WHERE consumed_at IS NULL;

CREATE TABLE password_reset_tokens (
    token_hash  BYTEA PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_user_active_idx
    ON password_reset_tokens(user_id) WHERE consumed_at IS NULL;
