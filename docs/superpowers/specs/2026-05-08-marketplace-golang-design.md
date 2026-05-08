# Design Spec — Marketplace Golang

**Date:** 2026-05-08
**Status:** Draft (pending user review)
**Author:** Brainstorming session (Claude + user)
**Module:** `github.com/danilloboing/marketplace-golang`

---

## 1. Product Overview

### 1.1 Project type

E-commerce **single-vendor** (não marketplace multi-vendor) — o vendedor é a empresa dona da plataforma. Sem onboarding de sellers externos, sem split de pagamento, sem comissão.

**Nicho:** Roupas e acessórios femininos.

### 1.2 Target market

**v1:** Brasil exclusivo. Pix, parcelamento cartão, CPF/CNPJ, Correios + transportadoras, BRL.
**Futuro:** arquitetura preparada para internacionalização (i18n, multi-currency, gateway intercambiável).

### 1.3 Delivery strategy

MVP completo entregue em **fases incrementais**, cada fase produzindo um slice vertical funcional. Não-negociável: cada fase atende padrão de qualidade de produção (testes, observabilidade, segurança).

### 1.4 Out of scope (v1)

- Frontend (web/mobile) — apenas backend REST API
- Marketplace multi-vendor
- Marketing automation, abandoned cart recovery, push notifications
- Programa de fidelidade, gift card
- App mobile nativo
- Recomendação personalizada (ML/IA)
- Live commerce, social commerce
- Subscription / recurrence
- Internacional / multi-currency / multi-idioma

---

## 2. Architectural Decisions

### 2.1 Stack core

| Camada | Escolha | Justificativa |
|---|---|---|
| Linguagem | **Go 1.23+** | Performance, type safety, deploy fácil, concorrência nativa |
| HTTP framework | **`go-chi/chi`** | Stdlib-compatible, middleware idiomático, sem mágica |
| Database | **PostgreSQL 16+** | Robustez, FTS nativo, JSONB, transações fortes, ecossistema |
| DB driver | **`jackc/pgx/v5`** | Performance superior a `lib/pq`, suporte a PG features modernas |
| DB queries | **sqlc** | Type-safe, validação compile-time contra schema, zero reflection, zero SQL injection by design |
| Migrations | **Atlas** | Declarative schema-as-code, drift detection, planejamento atômico |
| Cache + Sessions | **Redis 7+** | KV rápido, padrão indústria, TTL nativo |
| Job queue | **river** | Postgres-backed = transacional com domain data, sem SPOF Redis para jobs críticos |
| Storage | **Cloudflare R2** | Zero egress fee (crítico para imagens), S3-compatible, ~$0.50/mês MVP |
| CDN | **Cloudflare** (free tier) | Edge BR, TLS, DDoS, WAF — incluído com R2 |
| Image processing | **`davidbyttow/govips`** (libvips bindings) | Gera variantes (thumb/médio/grande/webp) no upload, zero recurring cost |
| Search | **PostgreSQL FTS + `pg_trgm`** | Built-in, zero infra extra, suficiente até 100k SKUs |
| Email | **AWS SES** | Custo mais baixo em escala ($0.10/1k emails), reputação BR |
| Logger | **`log/slog`** (stdlib) | Structured JSON, vendor-neutral, sem dep externa |
| Metrics | **Prometheus client** + `/metrics` endpoint | Padrão indústria, qualquer backend consome |
| Tracing | **OpenTelemetry SDK** | Vendor-neutral, OTLP exporter |
| Errors / panics | **Sentry SDK (`sentry-go`)** | Free tier 5k events/mês |
| Validation | **`go-playground/validator`** | Struct tags, padrão Go ecosystem |
| Config | **`caarlos0/env`** + structs + `.env` | Type-safe, simples, sem viper bloat |
| UUID | **`google/uuid` v7** | Time-ordered, indexável bem em Postgres |
| HTTP client (externos) | **`go-resty/resty`** | Retries, timeouts, baseURL |
| Tests | **`testing` + `testify` + `testcontainers-go`** | Stdlib + asserts + Postgres/Redis real em integration |
| Mocks | **`mockery`** | Geração estática a partir de interfaces |
| OpenAPI | **`oapi-codegen`** (spec-first) | Gera handlers stub a partir de YAML |
| i18n (preparação futura) | **`nicksnyder/go-i18n`** | Mensagens user-facing via chaves desde v1 (default `pt-BR`) |
| Security headers | Middleware custom ou **`unrolled/secure`** | CSP, HSTS, X-Frame-Options, X-Content-Type-Options |
| Rate limiting | **`ulule/limiter`** ou **`tollbooth`** | Global + per-endpoint (login, signup, checkout) |

### 2.2 Architecture style

**Monolito modular DDD-light.**

Razões:
- Single-vendor e-commerce não justifica complexidade de microservices
- Time pequeno (1-2 devs) — overhead de ops distribuída inviabiliza entrega
- Modular bem feito permite extrair serviços depois (Strangler Fig)
- Postgres + Redis bastam até centenas de milhares de pedidos/mês

### 2.3 Authentication

**Session-based em Redis** + cookie httpOnly/Secure/SameSite=Lax.

- Login email/senha (bcrypt cost 12+) — primary
- Login Google OAuth 2.0 (lib `coreos/go-oidc` ou `goth`) — additional desde v1
- Session ID gerado via `crypto/rand` (256 bits)
- TTL 30 dias com sliding expiration
- Rotação de session ID em login privilegiado (mudança de senha, escalonamento)
- Logout = delete da chave Redis (revogação imediata)
- Password reset via token short-lived (1h) enviado por email

Razões da escolha (vs JWT):
- E-commerce exige revogação instantânea (fraude, refund, bloqueio)
- Redis já presente para cache/queue — sem custo adicional
- Same-origin (frontend único) — cookie funciona perfeitamente
- httpOnly cookie protege contra XSS, SameSite contra CSRF
- Implementação mais simples, menor surface area de bug

### 2.4 Payment provider

**v1: Pagar.me** com abstração `PaymentProvider`.

```go
type PaymentProvider interface {
    CreateCharge(ctx context.Context, req ChargeRequest) (Charge, error)
    CapturePix(ctx context.Context, chargeID string) (PixCharge, error)
    HandleWebhook(ctx context.Context, payload []byte, sig string) (Event, error)
    Refund(ctx context.Context, chargeID string, amount Money) error
}
```

Implementação `Pagar.meProvider` deixada **por último na fase 3**. Antes: mock provider permite desenvolver fluxo de pedido/checkout sem dependência externa. Decisão final do gateway pode mudar (MercadoPago, Stripe) com troca isolada de adapter.

### 2.5 Shipping provider

**v1: Melhor Envio** com abstração `ShippingProvider`.

```go
type ShippingProvider interface {
    Quote(ctx context.Context, req QuoteRequest) ([]Quote, error)
    CreateLabel(ctx context.Context, req LabelRequest) (Label, error)
    Track(ctx context.Context, code string) (TrackingStatus, error)
    HandleWebhook(ctx context.Context, payload []byte) (Event, error)
}
```

Cobre Correios + transportadoras (Jadlog, Latam, etc) numa única integração.

### 2.6 Storage / images

**Cloudflare R2** (S3-compatible) + Cloudflare CDN free tier.

- Upload original via signed URL (cliente upload direto, app não bottleneck)
- Geração de variantes (thumb 200px, médio 600px, grande 1200px, webp) **server-side** com libvips no momento do upload
- Variantes salvas em R2 com convenção de path: `products/{productID}/{variantID}/{size}.webp`
- CDN cacheia automaticamente
- Cleanup de imagens órfãs via job river agendado

### 2.7 Search

**Postgres FTS + `pg_trgm`** com abstração `SearchProvider`.

- Coluna `tsvector` em `products` atualizada via trigger
- Configuração `portuguese` (stemming, stop words PT)
- Fuzzy via `pg_trgm` (similaridade) para typo tolerance básica
- Faceted filtering (categoria, tamanho, cor, preço) via SQL normal
- Migrar para Meilisearch quando: catálogo > 50k SKUs, ou UX exigir typo tolerance avançada/instant search

### 2.8 Job queue

**river** (Postgres-backed) com abstração `JobQueue`.

- Jobs enfileirados na **mesma transaction** do dado de negócio (ex: criar pedido + enfileirar email = atomic)
- Retries exponenciais, dead letter queue, scheduling
- river-ui para inspeção
- Worker process separado (`cmd/worker/main.go`)

Job types iniciais:
- `SendTransactionalEmail`
- `ProcessPaymentWebhook`
- `GenerateProductImageVariants`
- `SyncShippingTracking`
- `CleanupExpiredCart`

### 2.9 Observability

Instrumentação **vendor-neutral** desde início:

- **Logs:** `slog` JSON estruturado para stdout. Captura por plataforma de deploy.
- **Metrics:** Prometheus client + `/metrics` (RED metrics + business metrics como pedidos/min, conversão checkout)
- **Traces:** OpenTelemetry SDK + OTLP exporter (config via `OTEL_EXPORTER_OTLP_ENDPOINT`)
- **Errors:** Sentry SDK (`sentry-go`) com DSN via env
- **Health:** `/health` (liveness, retorna 200), `/ready` (readiness, checa Postgres + Redis)

Backend (Grafana Cloud, Datadog, self-hosted) decidido com deploy.

### 2.10 Internacionalização — preparação

Embora v1 seja Brasil only, código adota práticas que destravam expansão futura **sem refactor profundo**:

- Todas mensagens user-facing via `i18n` (lib `nicksnyder/go-i18n`) — chaves traduzidas, default `pt-BR`
- Money como value object com currency code (ex.: `Money{Amount: 9990, Currency: "BRL"}` representando R$99,90 em centavos) — nunca float
- Date/time sempre UTC no DB, conversão na borda
- Endereço como struct extensível (campos opcionais para US/EU formats)
- Identidade fiscal abstraída (CPF/CNPJ v1, extensível para SSN/VAT)
- Gateway de pagamento via interface (já decidido)

---

## 3. Project Structure

```
marketplace-golang/
├── cmd/                          # entry points (binários)
│   ├── api/main.go               # HTTP server
│   ├── worker/main.go            # river job worker
│   └── tools/                    # one-off (seed, admin scripts)
├── internal/                     # tudo privado
│   ├── modules/                  # bounded contexts
│   │   ├── catalog/              # Product, Category, Variant, Image
│   │   ├── identity/             # User, Session, Auth (email+senha+Google)
│   │   ├── cart/                 # Cart, CartItem (autenticado + guest)
│   │   ├── checkout/             # orquestra cart→order→payment
│   │   ├── ordering/             # Order, OrderItem, status machine
│   │   ├── payment/              # PaymentProvider port + Pagar.me adapter
│   │   ├── shipping/             # ShippingProvider port + Melhor Envio adapter
│   │   ├── inventory/            # Stock, reserva, decrement
│   │   ├── notification/         # email events, push (futuro)
│   │   └── review/               # ratings (fase 4)
│   ├── core/                     # cross-cutting
│   │   ├── httpx/                # middleware (auth, logger, recover, cors, ratelimit)
│   │   ├── errors/               # tipos custom + handler central
│   │   ├── observability/        # slog, OTel, Prom, Sentry setup
│   │   └── validator/            # validator config + custom rules
│   ├── platform/                 # adapters infra genéricos
│   │   ├── postgres/             # pgx pool, tx manager
│   │   ├── redis/                # client + session store
│   │   ├── storage/              # R2 client (S3 SDK)
│   │   ├── queue/                # river setup
│   │   └── email/                # SES adapter
│   ├── config/                   # env-based config struct
│   └── testutil/                 # test helpers (NewTestDB, fixtures)
├── db/
│   ├── migrations/               # Atlas .sql versionadas (organizadas por módulo)
│   ├── queries/                  # sqlc .sql files (organizadas por módulo)
│   └── sqlc.yaml
├── api/
│   └── openapi.yaml              # OpenAPI 3.1 spec (source of truth)
├── docs/
│   ├── superpowers/specs/        # specs deste flow
│   └── adr/                      # architecture decision records
├── deployments/
│   ├── Dockerfile
│   └── docker-compose.yml        # dev local (Postgres + Redis + app)
├── scripts/                      # helpers (seed, dev setup)
├── .github/workflows/            # CI (configurado quando código existir)
├── go.mod
├── Makefile
├── atlas.hcl
├── .golangci.yml
├── .gitignore
└── README.md
```

### 3.1 Module structure (per bounded context)

```
internal/modules/<context>/
├── domain/           # entities, value objects, domain errors. Sem dep externa.
├── application/      # use cases / services. Orquestra domain + ports.
├── infrastructure/   # adapters (repository impl via sqlc, external clients)
├── transport/        # HTTP handlers, request/response DTOs (gerados de OpenAPI)
└── module.go         # wiring (DI manual)
```

**Dependency rule:** outer → inner. `domain` não importa nada externo. `transport`/`infrastructure` podem importar `application` e `domain`.

### 3.2 Inter-module communication

- **Síncrona via portas (interfaces):** `ordering` não importa `catalog/infrastructure`, importa interface `catalog.ProductReader` definida em `catalog/application`.
- **Sem event bus em v1** — comunicação cross-module é toda síncrona via interfaces. Event bus pode ser introduzido depois (provavelmente fase 4-5) sem reescrever módulos existentes.

### 3.3 DI strategy

Manual constructor injection. `cmd/api/main.go` instancia adapters de infra → instancia services → instancia handlers → registra rotas.

Sem framework DI (wire, fx, samber/do) em v1 — wiring explícito é simples o suficiente para essa escala. Reavaliar se passar de ~20 services.

### 3.4 OpenAPI workflow

Spec-first:
1. Definir endpoint em `api/openapi.yaml`
2. `oapi-codegen` gera Go types + server interface stubs em `internal/modules/<ctx>/transport/generated/`
3. Implementar interface no handler
4. Geração validada em CI (commit do gerado, diff falha build)

---

## 4. Domain Model — high level

### 4.1 Catalog

- **Product:** id, slug, name, description, brand, category_id, base_price, status (draft/published/archived), tsvector
- **Category:** id, slug, name, parent_id (hierárquica)
- **Variant (SKU):** id, product_id, sku, size, color, price (override), stock_id
- **Image:** id, product_id, variant_id (nullable), url, position, alt_text

### 4.2 Identity

- **User:** id (uuid v7), email, password_hash, name, cpf, phone, status, created_at
- **OAuthAccount:** id, user_id, provider (google), provider_user_id, email, created_at
- **Address:** id, user_id, label, postal_code, street, number, complement, district, city, state, country, is_default
- **Session:** session_id (Redis key), user_id, ip, user_agent, expires_at

### 4.3 Cart

- **Cart:** id, user_id (nullable for guest), guest_token (nullable), expires_at
- **CartItem:** cart_id, variant_id, quantity, snapshot_price, snapshot_name

### 4.4 Inventory

- **Stock:** variant_id (PK), available, reserved, version (optimistic lock)
- **StockReservation:** id, variant_id, quantity, order_id (nullable), expires_at

### 4.5 Ordering

- **Order:** id, user_id, status, subtotal, shipping_cost, discount, total, address_snapshot, created_at
- **OrderItem:** order_id, variant_id, quantity, snapshot (full SKU snapshot at purchase time)
- **OrderStatusTransition:** order_id, from_status, to_status, reason, actor, occurred_at
- **States:** `pending → paid → preparing → shipped → delivered`. Branches: `cancelled`, `refunded`.

### 4.6 Payment

- **Charge:** id, order_id, provider (pagarme), provider_charge_id, method (pix/card/boleto), status, amount, raw_payload (JSONB), created_at
- **WebhookEvent:** id, provider, event_type, payload, signature, processed_at, status

### 4.7 Shipping

- **ShippingQuote:** id, cart_id, provider (melhor_envio), service_id, name, price, eta_days, expires_at
- **Shipment:** id, order_id, provider, label_url, tracking_code, status, last_status_at

### 4.8 Notification

- **EmailLog:** id, user_id, template, to, subject, status, provider_message_id, sent_at

### 4.9 Review

- **Review:** id, product_id, user_id, order_id (must have purchased), rating (1-5), title, body, status (pending/published/rejected), created_at

---

## 5. Phased Delivery Plan

### Fase 1 — Foundation + Catálogo read-only

**Goals:**
- Repo, módulos, Docker Compose dev, Makefile, .golangci.yml
- Atlas migrations + sqlc setup
- chi router + middleware (logger, recover, cors, ratelimit)
- Observability skeleton (slog, OTel, Prom, Sentry)
- OpenAPI spec inicial + oapi-codegen pipeline
- Storage R2 + libvips para variantes
- Domínio `catalog`: Product, Category, Variant, Image
- Endpoints públicos: `GET /products`, `GET /products/:slug`, `GET /categories`, `GET /search?q=...`
- Endpoints admin (auth provisória via static API token em env var **apenas para bootstrap da fase 1**; substituída por session-based admin com role-check na fase 2): CRUD produtos
- Postgres FTS + pg_trgm setup, trigger update tsvector

**Definition of done:**
- Docker compose sobe Postgres, Redis, app
- `curl /health` e `/ready` ok
- Cobertura testes ≥ 80% módulo catalog (≥ 90% domain)
- OpenAPI spec render limpo
- Imagens upload + variantes geradas automaticamente
- Seed script popula 50 produtos demo

### Fase 2 — Identity + Cart

**Goals:**
- Domínio `identity`: User, Address, Session
- Auth email/senha (bcrypt) + signup + login + logout + password reset
- Auth Google OAuth 2.0 (link account, primeiro login cria conta)
- Session middleware Redis cookie httpOnly
- Endpoint signup/login/logout/me
- Domínio `cart`: Cart, CartItem (autenticado + guest cart via cookie)
- Endpoints `POST /cart/items`, `PATCH /cart/items/:id`, `DELETE /cart/items/:id`, `GET /cart`
- Cleanup de carts expirados via job river
- CEP → endereço lookup (ViaCEP free)
- Wishlist básica

**Definition of done:**
- Cliente cria conta, faz login, lista produtos, adiciona ao carrinho, persistência
- Google OAuth completo (signup + login)
- Password reset funcional via SES
- Coverage targets atingidos
- Smoke test E2E passando

### Fase 3 — Checkout + Payment

**Goals:**
- Domínio `checkout`, `ordering`, `payment`, `shipping`, `inventory`
- Inventory com reserve/release (optimistic lock por version)
- Cálculo frete via Melhor Envio (cotação multi-transportadora)
- Cupom de desconto (fixed + percentage, expiration, usage limit)
- `POST /checkout/quote` (calcula tudo, retorna preview)
- `POST /checkout/confirm` (cria Order, reserva stock, cria Charge no provider)
- **PaymentProvider interface + mock implementation primeiro** (deixa fluxo destravado)
- Pagar.me adapter implementado **por último** (Pix + cartão + parcelamento)
- Webhooks de pagamento (signature validation + idempotency)
- State machine de Order com transições válidas

**Definition of done:**
- Fluxo completo carrinho → checkout → pagamento Pix/cartão funcional em sandbox
- Webhook de confirmação atualiza order
- Estoque decrementado atomicamente (sem oversell)
- Coverage payment + ordering + inventory ≥ 90%
- E2E test passa: signup → login → cart → checkout → pagamento → confirmação

### Fase 4 — Pós-venda

**Goals:**
- Eventos de envio Melhor Envio (label generation + tracking webhook)
- Order status transitions completas (preparing → shipped → delivered)
- Email transacional via SES + templates (signup, password reset, order confirmed, shipped, delivered)
- Reviews de produto (rating 1-5, comment, validação must-have-purchased)
- Avaliação de review por admin (publish/reject)
- Refund flow (parcial e total)
- Cancelamento de pedido (com regras de janela de tempo e estado)
- NF-e: integração via provider terceiro (NFE.io ou Migrate) — **avaliação em design separado** (pode ser fase própria)

**Definition of done:**
- Cliente recebe email em cada estado relevante
- Tracking visível ao cliente
- Review flow funcional, anti-spam básico
- Refund testado em sandbox

### Fase 5 — Crescimento

**Goals (escopo flexível, prioridade conforme negócio):**
- Painel admin avançado (relatórios, search admin)
- Cupons avançados (BOGO, frete grátis acima de X, primeira compra)
- Programa fidelidade básico (pontos por compra, cashback simples)
- Recomendações ("clientes que compraram também...") via SQL básico
- Analytics events (page view, add to cart, purchase) via job river → DW
- Webhook publishing (cliente externo pode subscrever a eventos)
- Internacionalização v0 (i18n keys já no código, traduções second locale)

---

## 6. Cross-cutting Concerns

### 6.1 Security

- **All input validated at boundary** (DTO + validator) before reaching domain
- **DB user least privilege** — app user sem `CREATE/DROP`, separado de migration user
- **TLS obrigatório** (`sslmode=require`) em todas conexões DB
- **Secrets via env vars** (caarlos0/env), nunca commitados. `.env.example` no repo, `.env` no .gitignore.
- **Password hashing:** bcrypt cost 12 (ajustar conforme hardware)
- **Rate limiting** middleware chi (`tollbooth` ou `ulule/limiter`) — global + per-endpoint critical (login, signup, checkout)
- **CORS** estrito (origin whitelist via env)
- **Security headers:** `unrolled/secure` ou implementação manual (CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy)
- **CSRF:** SameSite=Lax cobre maioria; double-submit cookie pra formulários sensíveis
- **CSP:** strict, nonces para inline (futuro frontend)
- **Audit log** para operações sensíveis (login, password change, refund, order cancel) — tabela `audit_log` append-only
- **govulncheck + gosec** no CI
- **Sem PII em logs** — request body filtrado, password/token nunca logado, CPF mascarado
- **Webhook signature validation** obrigatória (Pagar.me, Melhor Envio)
- **Idempotency** em webhooks via `(provider, event_id)` unique constraint

### 6.2 Performance

- **Connection pool:** `pgx` pool com `MaxOpenConns` configurado por env (default 25)
- **Indexes:** todo FK indexado, indexes compostos em queries críticas (catalog filters, order lookups)
- **Pagination:** cursor-based em listagens grandes, offset only para admin
- **Cache Redis:** product detail, category tree, hot search results (TTL 5-15min)
- **Avoid N+1:** sqlc força queries explícitas; integration tests em hot paths
- **Image delivery:** CDN cache + correct Cache-Control headers
- **Compression:** gzip/brotli middleware em `chi`

### 6.3 Logging standards

Format: `logger.<level>(message, structured-fields)`.

```go
// Sucesso
slog.Info("order created", 
    slog.String("orderID", order.ID),
    slog.String("userID", order.UserID),
    slog.Int64("amountCents", order.Total))

// Erro
slog.Error("failed to capture payment",
    slog.String("error", err.Error()),
    slog.String("orderID", orderID),
    slog.String("provider", "pagarme"))

// Warning
slog.Warn("low stock detected",
    slog.String("variantID", variantID),
    slog.Int("available", available))
```

camelCase nos campos. Nunca PII completo. Erros sempre com `stackTrace` se panic recovery.

### 6.4 Testing

**Pyramid:** ~70% unit, ~25% integration, ~5% E2E.

**Coverage targets:**
- `domain/` ≥ 90%
- `application/` ≥ 80%
- `infrastructure/` ≥ 70% (parte coberta por integration)
- `transport/` ≥ 70% (parte coberta por E2E)
- Críticos (`payment/`, `ordering/`, `inventory/`) ≥ 90%
- Total ≥ 80%

**Conventions:**
- AAA pattern, table-driven onde aplicável
- Naming: `Test<Type>_<Method>_<Scenario>`
- Variables: `inputX`, `mockX`, `actualX`, `expectedX`
- Fixtures: `testdata/` por módulo
- Helpers: `internal/testutil/`
- Integration tests: `testcontainers-go` Postgres+Redis real, tag `integration`
- E2E em `tests/e2e/`, sobe app + mocks de externos
- `goleak` para detectar goroutine leak
- `go test -race -count=1` em CI

### 6.5 Error handling

- Domain errors tipados (`domain.ErrProductNotFound`, `domain.ErrInsufficientStock`)
- Wrap com `fmt.Errorf("...%w", err)` para chain
- Mid-layer translates domain errors → HTTP errors (handler central)
- Never silenciar erros — log + return ou recover panic central
- Custom HTTP error response struct: `{ "error": { "code": "...", "message": "...", "details": {...} } }`

### 6.6 12-factor compliance

- Config via env vars
- Stdout structured logs
- Stateless processes (state em Postgres + Redis)
- Backing services como attached resources (URLs via env)
- Migrations como command separado (`cmd/tools/migrate`)
- Graceful shutdown (signal handling, drain in-flight requests)
- Health checks `/health` `/ready`

---

## 7. Deferred Decisions

Estas decisões não bloqueiam código v1 e ficam para o momento certo:

- **Deployment target** (Hetzner VPS único vs managed) — escolhido quando próximo do primeiro deploy. Código já 12-factor compliant, agnóstico.
- **Observability backend** (Grafana Cloud vs Datadog vs self-hosted) — escolhido com deploy. Instrumentação no código já vendor-neutral.
- **CI/CD pipeline** (GitHub Actions) — configurado quando primeiro código merge. Configuração não impacta código.
- **Final payment gateway** — Pagar.me é v1, mas adapter isolado permite trocar sem refactor (MercadoPago, Stripe).
- **Migrar Postgres FTS → Meilisearch** — quando catálogo ou UX exigir. Abstração `SearchProvider` já permite.
- **Migrar VPS único → DB managed** — quando dor de backup/replicação aparecer.
- **NF-e provider** (NFE.io, Migrate, etc) — design separado em fase 4.
- **Event bus introduction** — quando comunicação síncrona via interface começar a engessar (fase 4-5 provavelmente).

---

## 8. Open Questions / Risks

1. **Latência Brasil de VPS Europa** (se Hetzner escolhido): ~150-200ms RTT. CDN mitiga static, mas API checkout pode parecer lento. Avaliar com testes reais ou considerar VPS BR.
2. **Pagar.me sandbox + production approval** pode levar dias/semanas — adicionar à fase 3 cedo.
3. **AWS SES sandbox release** também demanda ticket + verificação de domínio + reputação inicial — começar fase 1 com domínio já verificado.
4. **Disaster recovery** em VPS único: backup snapshot + dump Postgres para R2 daily, RTO de 1-2h. Documentar runbook.
5. **Anti-fraude** v1: regras simples (CPF blacklist, velocity check) + dependência do antifraude do gateway. Avaliar Clearsale/Konduto se chargeback rate alto.
6. **GDPR/LGPD compliance:** consentimento explícito, direito ao esquecimento, exportação de dados. Endpoints administrativos para isso na fase 4-5.

---

## 9. Approval

Este spec é o resultado consolidado da sessão de brainstorming. Próximo passo (após aprovação do usuário): invocar `superpowers:writing-plans` para gerar plano detalhado de implementação da **Fase 1**.
