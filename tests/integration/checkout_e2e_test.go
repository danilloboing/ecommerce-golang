//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	invjobs "github.com/danilloboing/marketplace-golang/internal/modules/inventory/jobs"
	"github.com/danilloboing/marketplace-golang/internal/testutil"
)

// ---------------------------------------------------------------------------
// Request helpers (session + CSRF; mirror the Phase 2b style)
// ---------------------------------------------------------------------------

// authedJSONReq builds an authenticated, CSRF-bearing JSON request. The CSRF
// token is echoed from the csrf_token cookie into the X-CSRF-Token header
// (double-submit), matching how the address E2E tests drive protected routes.
func authedJSONReq(t *testing.T, method, url, body string, cookies []*http.Cookie) *http.Request {
	t.Helper()
	var r *http.Request
	var err error
	if body == "" {
		r, err = http.NewRequest(method, url, nil)
	} else {
		r, err = http.NewRequest(method, url, strings.NewReader(body))
	}
	require.NoError(t, err)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		r.AddCookie(c)
		if c.Name == "csrf_token" {
			r.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	return r
}

// addToCart adds qty of variantID to the authenticated user's cart. Because a
// session cookie is present, ResolveCartIdentity binds the cart to the user, so
// the confirm transaction's GetActiveCartByUser finds it.
func addToCart(t *testing.T, env checkoutEnv, cookies []*http.Cookie, variantID string, qty int) {
	t.Helper()
	body := fmt.Sprintf(`{"variant_id":%q,"quantity":%d}`, variantID, qty)
	resp, err := http.DefaultClient.Do(authedJSONReq(t, http.MethodPost, env.srv.URL+"/cart/items", body, cookies))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "add to cart")
}

// createAddress creates a shipping address for the authenticated user and
// returns its id.
func createAddress(t *testing.T, env checkoutEnv, cookies []*http.Cookie) string {
	t.Helper()
	body := `{"recipient_name":"Ana","postal_code":"01001000","street":"Sé","number":"1","neighborhood":"Sé","city":"São Paulo","state":"SP"}`
	resp, err := http.DefaultClient.Do(authedJSONReq(t, http.MethodPost, env.srv.URL+"/me/addresses", body, cookies))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "create address")
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	return created.ID
}

// quoteResult is the slice of the quote response E2E tests assert on.
type quoteResult struct {
	QuoteID string `json:"quote_id"`
	Total   int64  `json:"total_cents"`
}

// quote calls POST /checkout/quote and returns the decoded result. couponCode
// is optional (pass "").
func quote(t *testing.T, env checkoutEnv, cookies []*http.Cookie, addressID, couponCode string) quoteResult {
	t.Helper()
	body := fmt.Sprintf(`{"shipping_address_id":%q,"shipping_service_id":"pac","coupon_code":%q}`, addressID, couponCode)
	resp, err := http.DefaultClient.Do(authedJSONReq(t, http.MethodPost, env.srv.URL+"/checkout/quote", body, cookies))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "quote")
	var q quoteResult
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&q))
	require.NotEmpty(t, q.QuoteID)
	return q
}

// confirmResult is the slice of the confirm response E2E tests assert on.
type confirmResult struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
	Total   int64  `json:"total_cents"`
}

// confirm calls POST /checkout/confirm with the given quote and idempotency key
// and returns the raw response so the caller can assert on status + body.
func confirm(t *testing.T, env checkoutEnv, cookies []*http.Cookie, quoteID, idemKey string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"quote_id":%q,"payment_method":"pix"}`, quoteID)
	req := authedJSONReq(t, http.MethodPost, env.srv.URL+"/checkout/confirm", body, cookies)
	req.Header.Set("Idempotency-Key", idemKey)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// confirmOK runs confirm, asserts a 201, and returns the decoded result.
func confirmOK(t *testing.T, env checkoutEnv, cookies []*http.Cookie, quoteID, idemKey string) confirmResult {
	t.Helper()
	resp := confirm(t, env, cookies, quoteID, idemKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "confirm should be 201")
	var c confirmResult
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&c))
	require.NotEmpty(t, c.OrderID)
	return c
}

// postWebhook POSTs a signed payment webhook event and returns the response.
// When sign is true the body is signed with the env secret; otherwise the
// provided rawSig is sent verbatim (use "" for a missing signature).
func postWebhook(t *testing.T, env checkoutEnv, body []byte, sign bool, rawSig string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.srv.URL+"/payments/webhook", strings.NewReader(string(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if sign {
		req.Header.Set("X-Webhook-Signature", testutil.SignWebhook(env.secret, body))
	} else if rawSig != "" {
		req.Header.Set("X-Webhook-Signature", rawSig)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// paidEventBody builds a `paid` webhook payload for an order with a unique
// event id. amountCents must equal the order total for the event to apply.
func paidEventBody(orderID string, amountCents int64) []byte {
	return []byte(fmt.Sprintf(
		`{"id":%q,"type":"paid","provider_charge_id":%q,"amount_cents":%d}`,
		"evt_"+orderID, "mock_"+orderID, amountCents,
	))
}

// getOrderStatus fetches GET /me/orders/{id} for the authenticated user and
// returns (statusCode, orderStatus). orderStatus is "" on a non-200.
func getOrderStatus(t *testing.T, env checkoutEnv, cookies []*http.Cookie, orderID string) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedJSONReq(t, http.MethodGet, env.srv.URL+"/me/orders/"+orderID, "", cookies))
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, ""
	}
	var o struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&o))
	return resp.StatusCode, o.Status
}

// expireReservations forces all held reservations for an order into the past so
// the release job will pick them up.
func expireReservations(t *testing.T, env checkoutEnv, ctx context.Context, orderID string) {
	t.Helper()
	_, err := env.pool.Exec(ctx,
		`UPDATE stock_reservations SET expires_at = now() - interval '1 hour' WHERE order_id = $1`, orderID)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// The nine E2E tests (spec §10)
// ---------------------------------------------------------------------------

// TestCheckoutE2E_HappyPath proves the full money flow: register → login → set
// stock → add to cart → quote → confirm (pending_payment) → signed paid webhook
// → order is paid, available stock decremented, reservation committed.
func TestCheckoutE2E_HappyPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	cookies := registerVerifyLogin(t, env.srv, env.emails, "happy@example.com", "S3cretPass!")
	env.seedStock(t, ctx, env.variantID, 5)
	addToCart(t, env, cookies, env.variantID.String(), 2)
	addressID := createAddress(t, env, cookies)

	q := quote(t, env, cookies, addressID, "")
	order := confirmOK(t, env, cookies, q.QuoteID, "idem-happy-1")
	assert.Equal(t, "pending_payment", order.Status)
	assert.Equal(t, q.Total, order.Total)

	// After confirm: 2 reserved, 3 available.
	afterConfirm := env.readStock(t, ctx, env.variantID)
	assert.Equal(t, 3, afterConfirm.Available, "available reduced by reserve")
	assert.Equal(t, 2, afterConfirm.Reserved, "qty held")

	resp := postWebhook(t, env, paidEventBody(order.OrderID, order.Total), true, "")
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	code, status := getOrderStatus(t, env, cookies, order.OrderID)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "paid", status)

	// Commit reduces reserved to 0 without touching available again → 3 available, 0 reserved.
	afterPaid := env.readStock(t, ctx, env.variantID)
	assert.Equal(t, 3, afterPaid.Available, "available stays decremented after commit")
	assert.Equal(t, 0, afterPaid.Reserved, "reservation committed (reserved drained)")

	var committed int
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT count(*) FROM stock_reservations WHERE order_id = $1 AND status = 'committed'`, order.OrderID).Scan(&committed))
	assert.Equal(t, 1, committed, "reservation row marked committed")
}

// TestCheckoutE2E_Oversell proves the conditional-decrement guard: with stock=1,
// two users confirming the same variant concurrently yield exactly one 201 and
// one 422 insufficient_stock.
func TestCheckoutE2E_Oversell(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	env.seedStock(t, ctx, env.variantID, 1)

	type prepared struct {
		cookies []*http.Cookie
		quoteID string
	}
	prep := func(email string) prepared {
		cookies := registerVerifyLogin(t, env.srv, env.emails, email, "S3cretPass!")
		addToCart(t, env, cookies, env.variantID.String(), 1)
		addressID := createAddress(t, env, cookies)
		q := quote(t, env, cookies, addressID, "")
		return prepared{cookies: cookies, quoteID: q.QuoteID}
	}
	a := prep("oversell-a@example.com")
	b := prep("oversell-b@example.com")

	var wg sync.WaitGroup
	statuses := make([]int, 2)
	start := make(chan struct{})
	run := func(idx int, p prepared, key string) {
		defer wg.Done()
		<-start
		resp := confirm(t, env, p.cookies, p.quoteID, key)
		statuses[idx] = resp.StatusCode
		resp.Body.Close()
	}
	wg.Add(2)
	go run(0, a, "idem-oversell-a")
	go run(1, b, "idem-oversell-b")
	close(start)
	wg.Wait()

	created, conflict := 0, 0
	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusUnprocessableEntity:
			conflict++
		}
	}
	assert.Equal(t, 1, created, "exactly one confirm succeeds (statuses=%v)", statuses)
	assert.Equal(t, 1, conflict, "exactly one confirm is 422 insufficient_stock (statuses=%v)", statuses)

	// Net stock effect: one unit reserved, none left available.
	s := env.readStock(t, ctx, env.variantID)
	assert.Equal(t, 0, s.Available)
	assert.Equal(t, 1, s.Reserved)
}

// TestCheckoutE2E_PaidAfterExpiry_AwaitingStock proves C2: a paid webhook that
// lands after the reservation expired (and the freed stock was sold to a 2nd
// order) never drops the payment — the order becomes paid_awaiting_stock.
func TestCheckoutE2E_PaidAfterExpiry_AwaitingStock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	env.seedStock(t, ctx, env.variantID, 1)

	// First user confirms, consuming the only unit.
	cookiesA := registerVerifyLogin(t, env.srv, env.emails, "expiry-a@example.com", "S3cretPass!")
	addToCart(t, env, cookiesA, env.variantID.String(), 1)
	addrA := createAddress(t, env, cookiesA)
	qA := quote(t, env, cookiesA, addrA, "")
	orderA := confirmOK(t, env, cookiesA, qA.QuoteID, "idem-expiry-a")

	// Force the reservation expired and run the release job → order A expired,
	// stock freed back to available.
	expireReservations(t, env, ctx, orderA.OrderID)
	expired, err := invjobs.RunReleaseExpiredOnce(ctx, env.pool, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(1), expired, "one order expired by the release job")

	_, statusA := getOrderStatus(t, env, cookiesA, orderA.OrderID)
	require.Equal(t, "expired", statusA)
	freed := env.readStock(t, ctx, env.variantID)
	require.Equal(t, 1, freed.Available, "stock returned to available")
	require.Equal(t, 0, freed.Reserved)

	// Second user consumes the freed unit.
	cookiesB := registerVerifyLogin(t, env.srv, env.emails, "expiry-b@example.com", "S3cretPass!")
	addToCart(t, env, cookiesB, env.variantID.String(), 1)
	addrB := createAddress(t, env, cookiesB)
	qB := quote(t, env, cookiesB, addrB, "")
	orderB := confirmOK(t, env, cookiesB, qB.QuoteID, "idem-expiry-b")
	require.Equal(t, "pending_payment", orderB.Status)

	// Now the delayed paid webhook for order A arrives. No stock left → the
	// order must become paid_awaiting_stock (payment never dropped).
	resp := postWebhook(t, env, paidEventBody(orderA.OrderID, orderA.Total), true, "")
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, statusAfter := getOrderStatus(t, env, cookiesA, orderA.OrderID)
	assert.Equal(t, "paid_awaiting_stock", statusAfter)
}

// TestCheckoutE2E_WebhookForgeryRejected proves C1: a bad/missing signature is
// rejected with 401 and the order is left untouched.
func TestCheckoutE2E_WebhookForgeryRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	cookies := registerVerifyLogin(t, env.srv, env.emails, "forgery@example.com", "S3cretPass!")
	env.seedStock(t, ctx, env.variantID, 3)
	addToCart(t, env, cookies, env.variantID.String(), 1)
	addressID := createAddress(t, env, cookies)
	q := quote(t, env, cookies, addressID, "")
	order := confirmOK(t, env, cookies, q.QuoteID, "idem-forgery")

	body := paidEventBody(order.OrderID, order.Total)

	// Bad signature.
	bad := postWebhook(t, env, body, false, "deadbeef")
	assert.Equal(t, http.StatusUnauthorized, bad.StatusCode)
	bad.Body.Close()

	// Missing signature.
	missing := postWebhook(t, env, body, false, "")
	assert.Equal(t, http.StatusUnauthorized, missing.StatusCode)
	missing.Body.Close()

	_, status := getOrderStatus(t, env, cookies, order.OrderID)
	assert.Equal(t, "pending_payment", status, "forged webhook must not advance the order")
}

// TestCheckoutE2E_WebhookAmountMismatch proves C5 amount integrity: a validly
// signed paid event whose amount differs from the order total is accepted (200)
// but applies nothing — the order stays pending_payment.
func TestCheckoutE2E_WebhookAmountMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	cookies := registerVerifyLogin(t, env.srv, env.emails, "mismatch@example.com", "S3cretPass!")
	env.seedStock(t, ctx, env.variantID, 3)
	addToCart(t, env, cookies, env.variantID.String(), 1)
	addressID := createAddress(t, env, cookies)
	q := quote(t, env, cookies, addressID, "")
	order := confirmOK(t, env, cookies, q.QuoteID, "idem-mismatch")

	// Signed but with a wrong amount.
	resp := postWebhook(t, env, paidEventBody(order.OrderID, order.Total+100), true, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode, "mismatch is recorded (anti-replay), so 200")
	resp.Body.Close()

	_, status := getOrderStatus(t, env, cookies, order.OrderID)
	assert.Equal(t, "pending_payment", status, "amount mismatch must not advance the order")
}

// TestCheckoutE2E_Idempotent proves the idempotency contract: the same key +
// body returns the same order (one order created); the same key + a different
// body is a 409 conflict.
func TestCheckoutE2E_Idempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	cookies := registerVerifyLogin(t, env.srv, env.emails, "idem@example.com", "S3cretPass!")
	env.seedStock(t, ctx, env.variantID, 5)
	addToCart(t, env, cookies, env.variantID.String(), 1)
	addressID := createAddress(t, env, cookies)

	// Two quotes from the same (unchanged) cart. Their request hashes differ
	// because the hash is over the quote id, so they share a key but conflict.
	q1 := quote(t, env, cookies, addressID, "")
	q2 := quote(t, env, cookies, addressID, "")

	const sharedKey = "idem-key-shared"
	first := confirmOK(t, env, cookies, q1.QuoteID, sharedKey)
	// Replay: same key + same body → the stored result (same order id), no new order.
	second := confirmOK(t, env, cookies, q1.QuoteID, sharedKey)
	assert.Equal(t, first.OrderID, second.OrderID, "replay returns the same order id")

	// Exactly one order exists for this user.
	var orderCount int
	require.NoError(t, env.pool.QueryRow(ctx,
		`SELECT count(*) FROM orders o JOIN idempotency_keys k ON k.order_id = o.id WHERE k.key = $1`,
		sharedKey).Scan(&orderCount))
	assert.Equal(t, 1, orderCount, "only one order persisted for the reused key")

	// Same key, different body (different quote_id) → 409 conflict. The
	// idempotency lookup runs first, so this rejects before the (now converted)
	// cart is even consulted.
	conflictResp := confirm(t, env, cookies, q2.QuoteID, sharedKey)
	assert.Equal(t, http.StatusConflict, conflictResp.StatusCode, "same key + different body → 409")
	conflictResp.Body.Close()
}

// TestCheckoutE2E_QuoteStale proves both staleness guards: confirming an expired
// quote → 409, and confirming after the cart changed → 409 cart_changed.
func TestCheckoutE2E_QuoteStale(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	cookies := registerVerifyLogin(t, env.srv, env.emails, "stale@example.com", "S3cretPass!")
	env.seedStock(t, ctx, env.variantID, 5)
	addToCart(t, env, cookies, env.variantID.String(), 1)
	addressID := createAddress(t, env, cookies)

	// (a) expired quote → 409.
	qExpired := quote(t, env, cookies, addressID, "")
	_, err := env.pool.Exec(ctx,
		`UPDATE checkout_quotes SET expires_at = now() - interval '1 hour' WHERE id = $1`, qExpired.QuoteID)
	require.NoError(t, err)
	expiredResp := confirm(t, env, cookies, qExpired.QuoteID, "idem-stale-expired")
	assert.Equal(t, http.StatusConflict, expiredResp.StatusCode, "expired quote → 409")
	expiredResp.Body.Close()

	// (b) cart mutated after quote → 409 cart_changed.
	qFresh := quote(t, env, cookies, addressID, "")
	addToCart(t, env, cookies, env.variantID.String(), 1) // bump qty → fingerprint changes
	changedResp := confirm(t, env, cookies, qFresh.QuoteID, "idem-stale-changed")
	assert.Equal(t, http.StatusConflict, changedResp.StatusCode, "cart changed → 409")
	bodyBytes, _ := io.ReadAll(changedResp.Body)
	changedResp.Body.Close()
	assert.Contains(t, string(bodyBytes), "cart_changed", "error code should be cart_changed")
}

// TestCheckoutE2E_CouponLimit proves C4 coupon redemption: a usage_limit=1
// coupon succeeds once; a second confirm with the same coupon → 422
// coupon_unavailable.
func TestCheckoutE2E_CouponLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	// Admin creates a single-use fixed coupon.
	createCoupon(t, env, `{"code":"SAVE10","type":"fixed","value":1000,"usage_limit":1}`)

	env.seedStock(t, ctx, env.variantID, 5)

	// Both users quote with the coupon BEFORE either confirms, so used_count is
	// still 0 at quote time and the single-use guard is exercised at the atomic
	// redeem step inside confirm (C4), not at validation time.
	cookiesA := registerVerifyLogin(t, env.srv, env.emails, "coupon-a@example.com", "S3cretPass!")
	addToCart(t, env, cookiesA, env.variantID.String(), 1)
	addrA := createAddress(t, env, cookiesA)
	qA := quote(t, env, cookiesA, addrA, "SAVE10")

	cookiesB := registerVerifyLogin(t, env.srv, env.emails, "coupon-b@example.com", "S3cretPass!")
	addToCart(t, env, cookiesB, env.variantID.String(), 1)
	addrB := createAddress(t, env, cookiesB)
	qB := quote(t, env, cookiesB, addrB, "SAVE10")

	// First user redeems the coupon successfully (used_count → 1 = limit).
	orderA := confirmOK(t, env, cookiesA, qA.QuoteID, "idem-coupon-a")
	assert.Equal(t, "pending_payment", orderA.Status)

	// Second user: the atomic conditional redeem finds the coupon at its limit
	// and returns 0 rows → ErrCouponUnavailable → 422.
	secondResp := confirm(t, env, cookiesB, qB.QuoteID, "idem-coupon-b")
	assert.Equal(t, http.StatusUnprocessableEntity, secondResp.StatusCode, "coupon at limit → 422")
	bodyBytes, _ := io.ReadAll(secondResp.Body)
	secondResp.Body.Close()
	assert.Contains(t, string(bodyBytes), "coupon_unavailable")
}

// TestCheckoutE2E_OrderIDOR proves the per-user scoping: user B cannot read user
// A's order — GET /me/orders/{A's id} returns 404 (not 403, to avoid leaking
// existence).
func TestCheckoutE2E_OrderIDOR(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	env := startCheckoutAPI(t, ctx)

	env.seedStock(t, ctx, env.variantID, 5)

	cookiesA := registerVerifyLogin(t, env.srv, env.emails, "idor-a@example.com", "S3cretPass!")
	addToCart(t, env, cookiesA, env.variantID.String(), 1)
	addrA := createAddress(t, env, cookiesA)
	qA := quote(t, env, cookiesA, addrA, "")
	orderA := confirmOK(t, env, cookiesA, qA.QuoteID, "idem-idor-a")

	// User A can read their own order.
	codeOwn, statusOwn := getOrderStatus(t, env, cookiesA, orderA.OrderID)
	require.Equal(t, http.StatusOK, codeOwn)
	require.Equal(t, "pending_payment", statusOwn)

	// User B cannot.
	cookiesB := registerVerifyLogin(t, env.srv, env.emails, "idor-b@example.com", "S3cretPass!")
	codeOther, _ := getOrderStatus(t, env, cookiesB, orderA.OrderID)
	assert.Equal(t, http.StatusNotFound, codeOther, "cross-user order read → 404")
}

// ---------------------------------------------------------------------------
// Admin helper
// ---------------------------------------------------------------------------

// createCoupon POSTs to the admin coupon endpoint with the admin bearer token.
func createCoupon(t *testing.T, env checkoutEnv, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.srv.URL+"/admin/coupons", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "create coupon")
}
