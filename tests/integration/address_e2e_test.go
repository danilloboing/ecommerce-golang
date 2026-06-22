//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authedAddressReq builds an authenticated, CSRF-bearing request.
func authedAddressReq(t *testing.T, method, url, body string, cookies []*http.Cookie) *http.Request {
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

func TestAddressE2E_CRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)
	cookies := registerVerifyLogin(t, env.srv, env.emails, "addr@example.com", "S3cretPass!")

	// create
	body := `{"recipient_name":"Ana","postal_code":"01001000","street":"Sé","number":"1","neighborhood":"Sé","city":"São Paulo","state":"SP"}`
	resp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodPost, env.srv.URL+"/me/addresses", body, cookies))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created struct{ ID string `json:"id"` }
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()

	// list
	listResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodGet, env.srv.URL+"/me/addresses", "", cookies))
	require.NoError(t, err)
	var list []map[string]any
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	listResp.Body.Close()
	assert.Len(t, list, 1)

	// patch
	patchResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodPatch, env.srv.URL+"/me/addresses/"+created.ID, `{"number":"222"}`, cookies))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, patchResp.StatusCode)
	patchResp.Body.Close()

	// delete
	delResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodDelete, env.srv.URL+"/me/addresses/"+created.ID, "", cookies))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()
}

func TestAddressE2E_DefaultUnique(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	env := startCommerceAPI(t, ctx)
	cookies := registerVerifyLogin(t, env.srv, env.emails, "def@example.com", "S3cretPass!")

	mk := func(def bool) {
		body := `{"recipient_name":"Ana","postal_code":"01001000","street":"Sé","number":"1","neighborhood":"Sé","city":"SP","state":"SP","is_default":` + boolStr(def) + `}`
		resp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodPost, env.srv.URL+"/me/addresses", body, cookies))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}
	mk(true)
	mk(true)

	listResp, err := http.DefaultClient.Do(authedAddressReq(t, http.MethodGet, env.srv.URL+"/me/addresses", "", cookies))
	require.NoError(t, err)
	var list []struct {
		IsDefault bool `json:"is_default"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	listResp.Body.Close()

	defaults := 0
	for _, a := range list {
		if a.IsDefault {
			defaults++
		}
	}
	assert.Equal(t, 1, defaults, "exactly one default address")
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
