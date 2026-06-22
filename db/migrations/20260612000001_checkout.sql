-- inventory
CREATE TABLE inventory_stock (
  variant_id  UUID PRIMARY KEY REFERENCES catalog_variants(id) ON DELETE CASCADE,
  available   INT  NOT NULL CHECK (available >= 0),
  reserved    INT  NOT NULL DEFAULT 0 CHECK (reserved >= 0),
  version     INT  NOT NULL DEFAULT 0,            -- lost-update guard for admin set-stock only
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stock_reservations (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id    UUID NOT NULL,                       -- FK added after orders table
  variant_id  UUID NOT NULL REFERENCES catalog_variants(id) ON DELETE RESTRICT,
  quantity    INT  NOT NULL CHECK (quantity > 0),
  status      TEXT NOT NULL DEFAULT 'held'
                CHECK (status IN ('held','committed','released')),
  expires_at  TIMESTAMPTZ NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX stock_reservations_order_idx ON stock_reservations(order_id);
CREATE INDEX stock_reservations_expiry_idx
  ON stock_reservations(expires_at) WHERE status = 'held';

-- ordering
CREATE TABLE orders (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  status          TEXT NOT NULL
                    CHECK (status IN ('pending_payment','paid','payment_failed',
                                      'expired','paid_awaiting_stock')),
  subtotal_cents  BIGINT NOT NULL CHECK (subtotal_cents >= 0),
  shipping_cents  BIGINT NOT NULL CHECK (shipping_cents >= 0),
  discount_cents  BIGINT NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
  total_cents     BIGINT NOT NULL CHECK (total_cents >= 0),
  coupon_code     TEXT,
  address_snapshot   JSONB NOT NULL,               -- frozen recipient/postal/street/...
  shipping_snapshot  JSONB NOT NULL,               -- frozen service id/name/price/eta
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX orders_user_idx ON orders(user_id, created_at DESC);

ALTER TABLE stock_reservations
  ADD CONSTRAINT stock_reservations_order_fk
  FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;

CREATE TABLE order_items (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id          UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  variant_id        UUID NOT NULL REFERENCES catalog_variants(id) ON DELETE RESTRICT,
  quantity          INT  NOT NULL CHECK (quantity > 0 AND quantity <= 99),
  unit_price_cents  BIGINT NOT NULL CHECK (unit_price_cents >= 0),
  product_snapshot  JSONB NOT NULL,                -- name/slug/sku/size/color at purchase
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX order_items_order_idx ON order_items(order_id);

CREATE TABLE order_status_transitions (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  from_status  TEXT,
  to_status    TEXT NOT NULL,
  reason       TEXT NOT NULL,
  actor        TEXT NOT NULL,                       -- 'system'|'webhook'|'job'|'admin'
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX order_status_transitions_order_idx ON order_status_transitions(order_id);

-- payment
CREATE TABLE charges (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id           UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
  provider           TEXT NOT NULL,                 -- 'mock'|'pagarme'
  provider_charge_id TEXT NOT NULL,
  method             TEXT NOT NULL,                 -- 'pix'|'card'|'boleto'
  status             TEXT NOT NULL
                       CHECK (status IN ('pending','paid','failed','refunded')),
  amount_cents       BIGINT NOT NULL CHECK (amount_cents >= 0),
  raw_payload        JSONB,                          -- NEVER PAN/CVV (see §7 N1)
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX charges_provider_charge_uniq ON charges(provider, provider_charge_id);
CREATE INDEX charges_order_idx ON charges(order_id);

CREATE TABLE payment_webhook_events (
  event_id     TEXT PRIMARY KEY,                    -- provider event id — idempotency key
  provider     TEXT NOT NULL,
  charge_id    UUID REFERENCES charges(id) ON DELETE SET NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- checkout
CREATE TABLE coupons (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code            TEXT NOT NULL UNIQUE,
  type            TEXT NOT NULL CHECK (type IN ('fixed','percent')),
  value           BIGINT NOT NULL CHECK (value > 0),  -- cents (fixed) or basis 1..100 (percent)
  expires_at      TIMESTAMPTZ,
  usage_limit     INT,                                -- NULL = unlimited
  used_count      INT NOT NULL DEFAULT 0,
  min_order_cents BIGINT,
  active          BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (type <> 'percent' OR value BETWEEN 1 AND 100)
);

CREATE TABLE checkout_quotes (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  cart_fingerprint TEXT NOT NULL,                    -- hash of cart lines (variant+qty)
  lines_snapshot   JSONB NOT NULL,                   -- locked unit prices per variant
  shipping_snapshot JSONB NOT NULL,                  -- chosen service id/name/price/eta
  coupon_code      TEXT,
  subtotal_cents   BIGINT NOT NULL,
  shipping_cents   BIGINT NOT NULL,
  discount_cents   BIGINT NOT NULL,
  total_cents      BIGINT NOT NULL,
  expires_at       TIMESTAMPTZ NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX checkout_quotes_user_idx ON checkout_quotes(user_id);

CREATE TABLE idempotency_keys (
  key          TEXT NOT NULL,
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  request_hash TEXT NOT NULL,
  order_id     UUID REFERENCES orders(id) ON DELETE SET NULL,
  response     JSONB,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, key)
);
