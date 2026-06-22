# Phase 3a — Checkout Core (mock payment) Design

> Sub-phase of Phase 3 (Checkout + Payment). 3a delivers the full cart→order→payment flow behind provider interfaces with a **mock** payment provider and a **mock** shipping provider, hardened for money integrity. 3b swaps in the real Pagar.me adapter; real Melhor Envio and shipping labels/tracking land later. Tag on completion: `v0.6.0-checkout`.

## 1. Scope

### In-scope (3a)

- `inventory` module: per-variant stock with atomic reserve/release (oversell guard), admin set-stock.
- `ordering` module: `Order`/`OrderItem`, order state machine + transition audit, authenticated order read.
- `payment` module: `PaymentProvider` port + `MockProvider`, `Charge`, signed + idempotent webhook handler.
- `shipping` module: `ShippingProvider` port + `MockShipping` (stubbed quotes).
- `checkout` module: orchestrates cart→quote→order→reserve→charge; owns coupons (table + admin + application).
- Endpoints: `POST /checkout/quote`, `POST /checkout/confirm`, `GET /me/orders`, `GET /me/orders/{id}`, `POST /payments/webhook`, `PUT /admin/variants/{id}/stock`, `POST /admin/coupons`.
- Reservation TTL + river release job (expire unpaid orders, free stock).
- Money-integrity policy (§7) enforced end-to-end.

### Deferred

| Item | Phase | Reason |
|---|---|---|
| Pagar.me adapter (Pix/card/installments) + real webhook signature | 3b | Mock-first unblocks the flow; real gateway last |
| Real Melhor Envio quotes | 3b/3c | Mock keeps 3a offline-testable |
| Shipping label generation + tracking | 4 | Post-sale |
| Order cancellation + refund | 4 | Depends on payment capture + business rules |
| Order states preparing/shipped/delivered | 4 | Fulfillment lifecycle |
| Per-user coupon limit, stacking, BOGO, free-shipping, first-purchase | 5 | "Cupons avançados" |
| Per-method reservation TTL (boleto = days) | 3b | Pix-only window in 3a |
| Card tokenization / PCI handling | 3b | No card data in 3a |

### Out of scope (decided)

- Guest checkout — login required; anon cart merges on login (Phase 2b).

## 2. Architecture

### Module layout (5 new bounded contexts)

```
internal/modules/
├── inventory/
│   ├── domain/          # Stock, StockReservation, ReservationStatus, sentinel errors
│   ├── application/     # InventoryService, ports (StockRepository)
│   ├── infrastructure/  # Postgres repo: atomic conditional reserve/release
│   ├── jobs/            # release_expired_reservations river job
│   └── transport/       # admin set-stock handler
├── ordering/
│   ├── domain/          # Order, OrderItem, OrderStatus, transitions table, sentinel errors
│   ├── application/     # OrderService (create, transition, read), ports
│   ├── infrastructure/  # Postgres repo
│   └── transport/       # GET /me/orders, GET /me/orders/{id}
├── payment/
│   ├── domain/          # Charge, ChargeStatus, PaymentEvent, sentinel errors
│   ├── application/     # PaymentProvider port, ChargeService, webhook application
│   ├── infrastructure/  # MockProvider, Postgres charge repo, webhook-event store
│   └── transport/       # POST /payments/webhook
├── shipping/
│   ├── domain/          # Quote, sentinel errors
│   ├── application/     # ShippingProvider port, ShippingService
│   └── infrastructure/  # MockShipping
└── checkout/
    ├── domain/          # Coupon, CouponType, Quote aggregate, sentinel errors
    ├── application/     # CheckoutService (orchestrator), CouponService, ports
    ├── infrastructure/  # Postgres repos (checkout_quotes, coupons, idempotency_keys)
    └── transport/       # POST /checkout/quote, /checkout/confirm, POST /admin/coupons
```

### Orchestration & boundaries

`checkout` is the orchestrator. It depends only on the **application-layer ports** of the other modules (never their infrastructure), consistent with the existing project rule (`golang-naming` / DI). Concretely, `CheckoutService` consumes interfaces it declares in `checkout/application/ports.go`, satisfied by the other modules' services:

- `CartReader` ← cart (`Get`, `Clear` / mark converted)
- `PriceReader` ← catalog (`VariantUnitPrice` — authoritative current price)
- `Reserver` ← inventory (`Reserve`, `Release`, `Commit`)
- `OrderWriter` ← ordering (`Create`, `Transition`)
- `Charger` ← payment (`CreateCharge` via `PaymentProvider`)
- `ShippingQuoter` ← shipping (`Quote`)
- `AddressReader` ← address (`GetByID` for the chosen shipping address)

No import cycles: each provider module exposes a small interface; `cmd/api/main.go` wires concrete services into `checkout.New(Deps{...})`. Wiring order: catalog → cart → address → inventory → ordering → shipping → payment → checkout.

### Inter-module event flow

Synchronous, in-process, via ports. The only async element is the river release job. Payment confirmation is async **from the client's view** (webhook) but in-process for the server.

## 3. Data Model

New migration `db/migrations/20260612000001_checkout.sql`. All money columns `BIGINT` (`int64`), `CHECK (>= 0)` unless noted.

```sql
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
```

## 4. HTTP API

| Method | Path | Auth | Body | Response |
|---|---|---|---|---|
| POST | /checkout/quote | session | `{shipping_address_id, shipping_service_id?, coupon_code?}` | 200 `{quote_id, items[], subtotal, shipping_options[], chosen_shipping, discount, total, expires_at, price_drift?}` |
| POST | /checkout/confirm | session+CSRF, `Idempotency-Key` | `{quote_id}` | 201 `{order, charge:{method, status, pix_payload?}}` / 409 conflict / 422 |
| GET | /me/orders | session | — | 200 `[order summary...]` |
| GET | /me/orders/{id} | session | — | 200 `{order detail}` / 404 |
| POST | /payments/webhook | signature (HMAC) | provider event | 200 / 401 bad sig / 200-noop on dup |
| PUT | /admin/variants/{id}/stock | admin bearer | `{available, version?}` | 200 `{stock}` / 409 version conflict |
| POST | /admin/coupons | admin bearer | `{code, type, value, expires_at?, usage_limit?, min_order_cents?}` | 201 `{coupon}` |

Status policy inherits Phase 2 (§ shared): 200/201/204/400/401/403/404/409/422/429/500.

## 5. Checkout flow

### `POST /checkout/quote` (stateful preview — no order created)

1. Load the user's active cart (empty → 422 `cart_empty`).
2. For each line, fetch **authoritative current unit price from catalog** (`PriceReader`). Compare to the cart's snapshot price (Phase 2b) → set `price_drift=true` if any differ (informational).
3. Fetch shipping options from `ShippingProvider.Quote` (mock). If `shipping_service_id` given, pick it; else default to cheapest. Re-derive its price server-side.
4. If `coupon_code` given, validate (exists, active, not expired, `min_order` met) and compute discount server-side (§7 I4 rounding). Invalid coupon → 422 `coupon_invalid` (does not block; client may re-quote without it — but quote returns the error, no silent drop).
5. Compute `subtotal`, `shipping`, `discount`, `total = max(0, subtotal + shipping - discount)`.
6. Persist a `checkout_quotes` row (cart_fingerprint = hash of sorted (variant_id, qty); locked line prices; chosen shipping; coupon; totals; `expires_at = now + CHECKOUT_QUOTE_TTL`). Return `quote_id` + breakdown.

### `POST /checkout/confirm` (one DB transaction)

Input is only `{quote_id}` + `Idempotency-Key` header — minimal client-trusted surface.

1. **Idempotency replay (§7 I5):** look up `(user_id, key)`. If found and `request_hash` (hash of quote_id) matches → return stored `response`. If found with a different `request_hash` → 409 `idempotency_conflict`. Else reserve the key row.
2. Load `checkout_quotes` by `quote_id` scoped to `user_id`. Missing/expired (`expires_at < now`) → 409 `quote_expired` (client re-quotes).
3. Load the user's **active** cart; recompute `cart_fingerprint`; mismatch with the quote → 409 `cart_changed` (re-quote).
4. **Honor the locked quote prices.** The quote captured authoritative catalog prices and locked them for its TTL; confirm does **not** re-query the catalog — freshness (step 2) + cart fingerprint (step 3) bound staleness, so the customer pays exactly the quoted total. (Stock and coupon are still re-validated below, because they are shared mutable state the quote cannot lock.)
5. **Reserve stock (§7 I2, I3, I6):** for each line, in **ascending `variant_id` order** (deadlock avoidance), run the atomic conditional decrement. Any line insufficient → roll back the whole tx → 422 `insufficient_stock` (with the offending variant). Insert `stock_reservations` (`status='held'`, `expires_at = now + CHECKOUT_RESERVATION_TTL`).
6. **Lock coupon (§7 C4):** atomic conditional `used_count++`; 0 rows → 422 `coupon_unavailable` → rollback.
7. Create `Order(status='pending_payment')` + `order_items` (snapshot unit price + product + address + shipping). Set `stock_reservations.order_id`.
8. Mark the cart `converted` (§7 cart-guard idempotency — the `carts_user_active_uniq` index means a concurrent second confirm finds no active cart → 409).
9. `Charger.CreateCharge` (mock) for `order.total_cents` → `Charge(status='pending')`. **Charge amount = order.total** (asserted).
10. Record the idempotency response, commit, return `{order, charge}`.

> Steps 1-9 are one transaction. The mock charge creation is in-process (no external call), so it is safe inside the tx; for 3b the external charge call moves to just-after-commit with a compensating path if it fails (noted in §12).

### `POST /payments/webhook` (signed, idempotent — §7 C1, C5)

1. **Verify HMAC signature** over the raw body using the provider secret (`MOCK_WEBHOOK_SECRET` in 3a). Bad/missing → 401. (Always on, even for mock.)
2. Parse `event_id`. One transaction: insert `payment_webhook_events(event_id)` — on conflict (duplicate) → commit no-op, return 200.
3. Look up the `charge` by `provider_charge_id`; unknown → 200 (log) — do not error to the provider.
4. **Verify `event.amount == charge.amount_cents == order.total_cents`** (§7 C5). Mismatch → do NOT transition; log + alert, return 200.
5. Apply the effect by event type, as a **valid forward transition only**:
   - `paid` on a `pending_payment` order → commit reservations (`held→committed`, guarded), `charge.status='paid'`, order → `paid`.
   - `paid` on an `expired` order (§7 C2) → re-reserve all items atomically (ascending variant order). Success → commit, order → `paid`. Any insufficient → order → `paid_awaiting_stock`, log + alert (manual/refund), **never drop the payment**.
   - `failed` on `pending_payment` → release reservations (`held→released`), release the coupon redemption if any (`used_count--`, guarded), `charge.status='failed'`, order → `payment_failed`.
   - `paid` on an already-`paid` order, or any non-permitted transition → no-op (idempotent), return 200.
6. Record `order_status_transitions`. Commit.

### Release job (`release_expired_reservations`, river, every `CHECKOUT_RELEASE_INTERVAL`)

For each order `pending_payment` whose held reservations are past `expires_at`, **in one tx guarded by status**: re-check the order is still `pending_payment` and has **no `paid` charge** (paid-wins, §7 C2); if clear → release reservations (`held→released`), release the coupon redemption if any (`used_count--`, guarded), order → `expired`, record transition. The status guards make it race-safe against a concurrent webhook.

## 6. Order state machine

```
pending_payment ──paid(webhook)──────────────▶ paid
pending_payment ──failed(webhook)────────────▶ payment_failed
pending_payment ──ttl elapsed(job)───────────▶ expired
expired         ──paid(webhook), stock ok────▶ paid
expired         ──paid(webhook), no stock────▶ paid_awaiting_stock
```

Terminal in 3a: `paid`, `payment_failed`, `paid_awaiting_stock`. `expired` is **not** terminal w.r.t. an incoming `paid` (C2). Cancellation and fulfillment states (`preparing/shipped/delivered`) are Phase 4. Every transition is validated against this table; invalid transitions are rejected/ignored and never mutate stock or charge.

## 7. Security & Money-Integrity Policy

This is the binding contract for the implementation. Each rule maps to a review finding (C* critical, I* important, N* note).

- **C1 — Webhook is always signed.** `/payments/webhook` verifies an HMAC signature over the raw request body against the provider secret, including the mock provider (`MOCK_WEBHOOK_SECRET`). No unsigned path exists. There is no "mark paid" endpoint reachable without a valid signature.
- **C2 — Payment never lost to expiry.** A `paid` event on an `expired` order re-reserves stock; if stock is gone the order becomes `paid_awaiting_stock` and is flagged (log + metric) for manual resolution/refund. The release job only expires orders that are still `pending_payment` **and** have no `paid` charge, in a status-guarded transaction. Paid always wins.
- **C3 — Server is the source of truth for money.** Unit prices come from the catalog (authoritative), locked into a `checkout_quotes` row at quote time for `CHECKOUT_QUOTE_TTL`. Confirm honors the quote only if fresh and the cart fingerprint matches; otherwise 409 re-quote. Shipping price is re-derived server-side from the chosen service; coupon discount is recomputed server-side. The client never supplies any price, discount, or total — only `quote_id`, `shipping_address_id`, `shipping_service_id`, `coupon_code`.
- **C4 — Coupon redemption is race-safe.** `used_count` is incremented with an atomic conditional `UPDATE ... WHERE id=? AND active AND (usage_limit IS NULL OR used_count < usage_limit) AND (expires_at IS NULL OR expires_at > now()) RETURNING ...` inside the confirm tx. Zero rows → reject (422). No path can exceed `usage_limit`.
- **C5 — Webhook integrity.** Idempotent on provider `event_id` (insert + effect in one tx; duplicate → no-op). Amount verified (`event == charge == order.total`) before any transition. Only valid forward transitions applied; stale/out-of-order events ignored.
- **I1 — Quote↔Confirm binding.** Confirm references a persisted quote with a TTL; a stale/changed cart or expired quote forces a re-quote (409). The customer is charged the total they were quoted, or asked to re-quote — never a silently different amount.
- **I2 — Multi-item reserve is all-or-nothing with deterministic lock order.** Reservations for all lines happen in one tx, variants locked in ascending `variant_id` order to avoid deadlocks; partial failure rolls back the whole confirm.
- **I3 — Oversell guard is the conditional decrement.** Reserve = `UPDATE inventory_stock SET available=available-$qty, reserved=reserved+$qty, version=version+1 WHERE variant_id=$id AND available >= $qty` returning the row; zero rows → insufficient. `version` is used only for the admin set-stock lost-update guard, not as the reserve mechanism.
- **I4 — Total invariant + rounding.** `total = max(0, subtotal + shipping - discount)`. Discount is capped at `subtotal` (shipping is never discounted in 3a). Percentage discount = round-half-up to the nearest cent: `discount = (subtotal*pct + 50) / 100` (integer math). `total` and the charge amount always reconcile.
- **I5 — Idempotency-Key binds (user, key, request).** Keyed by `(user_id, key)`. Replay returns the stored response only if `request_hash` matches; a reused key with a different body → 409. A key never crosses users.
- **I6 — Reservation transitions are exactly-once.** `held → committed` / `held → released` use a status-guarded `UPDATE ... WHERE status='held'`; duplicate webhooks or a webhook racing the job can only transition once.
- **I7 — No IDOR on orders.** Every `/me/orders` query is scoped by the session `user_id`; cross-user access returns 404.
- **N1 — No card secrets stored.** `charges.raw_payload` never holds PAN/CVV. 3b uses Pagar.me client-side tokenization.
- **N2 — Reservation TTL is Pix-shaped (30m).** Per-method TTL (boleto = days) deferred to 3b.

## 8. Cross-cutting

### Provider interfaces (ports)

```go
// internal/modules/payment/application
type PaymentProvider interface {
    CreateCharge(ctx context.Context, req ChargeRequest) (Charge, error)
    VerifyWebhook(payload []byte, signature string) (Event, error)
    // Refund / CapturePix added in 3b
}

// internal/modules/shipping/application
type ShippingProvider interface {
    Quote(ctx context.Context, req QuoteRequest) ([]Quote, error)
    // CreateLabel / Track added in Phase 4
}
```

`MockProvider` (payment): `CreateCharge` returns a `pending` charge with a deterministic `provider_charge_id`; `VerifyWebhook` validates the HMAC and decodes a mock event. Tests construct signed mock events and POST them to `/payments/webhook` (the same path the real provider will hit). `MockShipping`: returns a fixed set of services (e.g. PAC/SEDEX-like) with stubbed prices/ETAs derived from the address region.

Factories selected by env (`PAYMENT_PROVIDER`, `SHIPPING_PROVIDER`), mirroring the Phase 2a `email.NewSenderFromConfig` pattern.

### Logging / observability

Structured slog. Business metrics (Prometheus): orders created, paid, expired, `paid_awaiting_stock` (alertable), reserve failures (oversell attempts), coupon rejections. The `paid_awaiting_stock` counter must be alert-wired (it means money taken without stock).

## 9. Error handling

Sentinel errors per module, package-prefixed, mapped at the transport boundary (own `mapErrorToHTTP` per module — never catalog's `responsex.WriteError`). Key mappings:

| Error | Status / code |
|---|---|
| cart empty | 422 `cart_empty` |
| quote missing/expired | 409 `quote_expired` |
| cart changed since quote | 409 `cart_changed` |
| insufficient stock | 422 `insufficient_stock` |
| coupon invalid/unavailable | 422 `coupon_invalid` / `coupon_unavailable` |
| idempotency conflict | 409 `idempotency_conflict` |
| order not found / cross-user | 404 `not_found` |
| bad webhook signature | 401 `invalid_signature` |
| stock version conflict (admin) | 409 `stock_conflict` |

## 10. Testing strategy

- **Unit:** domain (state-machine transition validity, total/rounding math, coupon discount), services with mock repos/providers.
- **Integration (testcontainers Postgres/Redis):** reserve/release atomicity incl. the conditional decrement under contention; coupon `used_count` conditional; webhook-event idempotency; reservation status guards.
- **E2E (full stack, mock providers, offline):**
  - `CheckoutE2E_HappyPath`: signup→login→admin set-stock→add to cart→quote→confirm→**signed mock webhook `paid`**→order `paid` + reservation `committed` + stock decremented.
  - `CheckoutE2E_Oversell`: stock=1, two concurrent confirms → exactly one succeeds, other 422.
  - `CheckoutE2E_PaidAfterExpiry`: confirm→force reservation expiry + release job→order `expired`→signed `paid` webhook→re-reserve ok→`paid` (and a stock-gone variant→`paid_awaiting_stock`).
  - `CheckoutE2E_QuoteStale`: confirm with expired quote→409; cart mutated after quote→409 `cart_changed`.
  - `CheckoutE2E_CouponLimit`: usage_limit=1, two confirms → second 422 `coupon_unavailable`.
  - `CheckoutE2E_Idempotent`: same `Idempotency-Key` twice → one order; different body same key → 409.
  - `CheckoutE2E_WebhookForgeryRejected`: unsigned / bad-HMAC webhook → 401, order unchanged.
  - `CheckoutE2E_WebhookAmountMismatch`: `paid` event with wrong amount → no transition.
  - `CheckoutE2E_OrderIDOR`: user B reads user A's order → 404.

## 11. Configuration

```
PAYMENT_PROVIDER=mock|pagarme            # default mock
SHIPPING_PROVIDER=mock|melhorenvio       # default mock
MOCK_WEBHOOK_SECRET=...                   # required (HMAC for mock webhook); 3b adds PAGARME_WEBHOOK_SECRET
CHECKOUT_QUOTE_TTL=15m
CHECKOUT_RESERVATION_TTL=30m
CHECKOUT_RELEASE_INTERVAL=5m
```

## 12. Deferred decisions / open questions

- **3b external charge ordering:** with a real gateway, `CreateCharge` is an external call and must move to just-after-commit, with a compensating release if the charge call fails after the order/reservation tx committed (or a two-phase "order created → charge → reservation finalize"). 3a's in-process mock sidesteps this; 3b must address it explicitly.
- **Quote storage growth:** `checkout_quotes` accumulates; add a cleanup job (or TTL index) in 3b.
- **Coupon redemption release (decided, baked into §5):** when an order transitions to `payment_failed` or `expired`, its coupon redemption is released (`used_count--`, status-guarded) — a non-purchase must never burn a coupon use.
- **paid_awaiting_stock resolution:** manual/admin refund flow is Phase 4; 3a only flags + alerts.
