//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCartE2E_AnonAddRemove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 2,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var anon *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "cart_anon" {
			anon = c
		}
	}
	var cart struct {
		Items         []struct{ ID string `json:"id"` } `json:"items"`
		SubtotalCents int64                              `json:"subtotal_cents"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cart))
	resp.Body.Close()
	require.NotNil(t, anon, "cart_anon cookie must be set")
	require.Len(t, cart.Items, 1)
	assert.Equal(t, int64(19800), cart.SubtotalCents)

	// remove the item using the anon cookie
	itemID := cart.Items[0].ID
	req, err := http.NewRequest(http.MethodDelete, env.srv.URL+"/cart/items/"+itemID, nil)
	require.NoError(t, err)
	req.AddCookie(anon)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, delResp.StatusCode)
	delResp.Body.Close()
}

func TestCartE2E_QtyClamp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 200,
	}, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	resp.Body.Close()
}

func TestCartE2E_MergeOnLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	// 1) anon adds an item
	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 3,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var anon *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "cart_anon" {
			anon = c
		}
	}
	resp.Body.Close()
	require.NotNil(t, anon)

	// 2) register + verify
	registerVerify(t, env.srv, env.emails, "merge@example.com", "S3cretPass!")

	// 3) login WITH the anon cart cookie → merge fires
	loginResp := postIdentityJSON(t, env.srv, "/auth/login", map[string]string{
		"email": "merge@example.com", "password": "S3cretPass!",
	}, []*http.Cookie{anon})
	require.Equal(t, http.StatusOK, loginResp.StatusCode)
	var session, clearedAnon *http.Cookie
	for _, c := range loginResp.Cookies() {
		switch c.Name {
		case "session_id":
			session = c
		case "cart_anon":
			clearedAnon = c
		}
	}
	loginResp.Body.Close()
	require.NotNil(t, session)
	if clearedAnon != nil {
		assert.True(t, clearedAnon.MaxAge < 0, "cart_anon should be cleared on login")
	}

	// 4) user cart has the merged item
	req, err := http.NewRequest(http.MethodGet, env.srv.URL+"/cart", nil)
	require.NoError(t, err)
	req.AddCookie(session)
	getResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var cart struct {
		Items []struct{ Quantity int `json:"quantity"` } `json:"items"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&cart))
	getResp.Body.Close()
	require.Len(t, cart.Items, 1)
	assert.Equal(t, 3, cart.Items[0].Quantity)
}

func TestCartE2E_VariantDeleteBlockedByFK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)

	resp := postIdentityJSON(t, env.srv, "/cart/items", map[string]any{
		"variant_id": env.variantID.String(), "quantity": 1,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Hard-deleting a referenced variant must fail with FK violation (23503).
	_, err := env.pool.Exec(ctx, `DELETE FROM catalog_variants WHERE id = $1`, env.variantID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "23503")
}
