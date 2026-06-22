-- Commerce tables: addresses + carts + cart_items.

CREATE TABLE addresses (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_name  TEXT NOT NULL,
    postal_code     TEXT NOT NULL CHECK (postal_code ~ '^[0-9]{8}$'),
    street          TEXT NOT NULL,
    number          TEXT NOT NULL,
    complement      TEXT,
    neighborhood    TEXT NOT NULL,
    city            TEXT NOT NULL,
    state           CHAR(2) NOT NULL,
    is_default      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX addresses_user_idx ON addresses(user_id);
CREATE UNIQUE INDEX addresses_user_default_uniq
    ON addresses(user_id) WHERE is_default = TRUE;

CREATE TABLE carts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID REFERENCES users(id) ON DELETE CASCADE,
    anon_session_id TEXT,
    status          TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active','merged','abandoned','converted')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (user_id IS NOT NULL AND anon_session_id IS NULL)
        OR
        (user_id IS NULL AND anon_session_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX carts_user_active_uniq
    ON carts(user_id) WHERE status = 'active' AND user_id IS NOT NULL;
CREATE UNIQUE INDEX carts_anon_active_uniq
    ON carts(anon_session_id) WHERE status = 'active' AND anon_session_id IS NOT NULL;

CREATE TABLE cart_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id          UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id       UUID NOT NULL REFERENCES catalog_variants(id) ON DELETE RESTRICT,
    quantity         INT NOT NULL CHECK (quantity > 0 AND quantity <= 99),
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents >= 0),
    added_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX cart_items_cart_variant_uniq ON cart_items(cart_id, variant_id);
