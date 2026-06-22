//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentityE2E_RegisterVerifyLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	// 1) Register
	resp := postIdentityJSON(t, srv, "/auth/register", map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!", "name": "Ana",
	}, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Captured verify email
	require.Eventually(t, func() bool { return len(captured.messages()) > 0 }, 5*time.Second, 50*time.Millisecond)
	verifyMsg := captured.messages()[0]
	token := extractTokenFromBody(t, verifyMsg.TextBody, "verify?token=")

	// 2) Verify email
	resp = postIdentityJSON(t, srv, "/auth/verify-email", map[string]string{"token": token}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 3) Login → 200 + cookies
	resp = postIdentityJSON(t, srv, "/auth/login", map[string]string{
		"email": "ana@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cookies := resp.Cookies()
	resp.Body.Close()

	var session, csrfCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "session_id":
			session = c
		case "csrf_token":
			csrfCookie = c
		}
	}
	require.NotNil(t, session)
	require.NotNil(t, csrfCookie)

	// 4) GET /me with cookie → 200 + correct user
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/me", nil)
	require.NoError(t, err)
	req.AddCookie(session)
	mResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, mResp.StatusCode)
	var user struct {
		Email string `json:"email"`
	}
	require.NoError(t, json.NewDecoder(mResp.Body).Decode(&user))
	mResp.Body.Close()
	assert.Equal(t, "ana@example.com", user.Email)
}

func TestIdentityE2E_LoginUnverifiedBlocked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, _ := startAPIForIdentity(t, ctx)
	defer srv.Close()

	resp := postIdentityJSON(t, srv, "/auth/register", map[string]string{
		"email": "noverify@example.com", "password": "S3cretPass!", "name": "X",
	}, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp = postIdentityJSON(t, srv, "/auth/login", map[string]string{
		"email": "noverify@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}

func TestIdentityE2E_PasswordResetFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	registerVerify(t, srv, captured, "rst@example.com", "S3cretPass!")

	resp := postIdentityJSON(t, srv, "/auth/password-reset/request", map[string]string{
		"email": "rst@example.com",
	}, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	require.Eventually(t, func() bool { return len(captured.messages()) >= 2 }, 5*time.Second, 50*time.Millisecond)
	resetMsg := captured.messages()[len(captured.messages())-1]
	token := extractTokenFromBody(t, resetMsg.TextBody, "reset?token=")

	resp = postIdentityJSON(t, srv, "/auth/password-reset/confirm", map[string]string{
		"token": token, "new_password": "NewS3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Old password rejected
	resp = postIdentityJSON(t, srv, "/auth/login", map[string]string{
		"email": "rst@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// New password works
	resp = postIdentityJSON(t, srv, "/auth/login", map[string]string{
		"email": "rst@example.com", "password": "NewS3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestIdentityE2E_CSRFRequired(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	cookies := registerVerifyLogin(t, srv, captured, "csrf@example.com", "S3cretPass!")

	// PATCH /me without CSRF header → 403
	req, err := http.NewRequest(http.MethodPatch, srv.URL+"/me", strings.NewReader(`{"name":"X"}`))
	require.NoError(t, err)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()

	// PATCH /me with valid CSRF header → 200
	var csrfValue string
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			csrfValue = c.Value
		}
	}
	req, err = http.NewRequest(http.MethodPatch, srv.URL+"/me", strings.NewReader(`{"name":"X"}`))
	require.NoError(t, err)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfValue)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestIdentityE2E_LogoutAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	srv, captured := startAPIForIdentity(t, ctx)
	defer srv.Close()

	first := registerVerifyLogin(t, srv, captured, "logoutall@example.com", "S3cretPass!")

	// Second login (different "device").
	loginResp := postIdentityJSON(t, srv, "/auth/login", map[string]string{
		"email": "logoutall@example.com", "password": "S3cretPass!",
	}, nil)
	require.Equal(t, http.StatusOK, loginResp.StatusCode)
	second := loginResp.Cookies()
	loginResp.Body.Close()

	// DELETE /auth/sessions/all from first device.
	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/auth/sessions/all", nil)
	require.NoError(t, err)
	for _, c := range first {
		req.AddCookie(c)
	}
	for _, c := range first {
		if c.Name == "csrf_token" {
			req.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Both cookies should now be invalid.
	for _, group := range [][]*http.Cookie{first, second} {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/me", nil)
		require.NoError(t, err)
		for _, c := range group {
			req.AddCookie(c)
		}
		r, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, r.StatusCode)
		r.Body.Close()
	}
}

// --- helpers ---

// postIdentityJSON sends a JSON POST to the identity API. Named distinctly
// from postJSON in catalog_e2e_test.go to avoid a same-package collision.
func postIdentityJSON(t *testing.T, srv *httptest.Server, path string, body any, cookies []*http.Cookie) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// extractTokenFromBody returns the substring following marker until the next
// whitespace or end of body. Used to fish verify / reset tokens out of email
// bodies rendered by the templates.
func extractTokenFromBody(t *testing.T, body, marker string) string {
	t.Helper()
	idx := strings.Index(body, marker)
	require.GreaterOrEqual(t, idx, 0, "marker %q not found in body", marker)
	rest := body[idx+len(marker):]
	end := strings.IndexAny(rest, "\n \t\r")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

func registerVerify(t *testing.T, srv *httptest.Server, captured emailCapture, addr, password string) {
	t.Helper()
	resp := postIdentityJSON(t, srv, "/auth/register", map[string]string{
		"email": addr, "password": password, "name": "User",
	}, nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()
	require.Eventually(t, func() bool { return len(captured.messages()) > 0 }, 5*time.Second, 50*time.Millisecond)
	last := captured.messages()[len(captured.messages())-1]
	token := extractTokenFromBody(t, last.TextBody, "verify?token=")
	resp = postIdentityJSON(t, srv, "/auth/verify-email", map[string]string{"token": token}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func registerVerifyLogin(t *testing.T, srv *httptest.Server, captured emailCapture, addr, password string) []*http.Cookie {
	t.Helper()
	registerVerify(t, srv, captured, addr, password)
	resp := postIdentityJSON(t, srv, "/auth/login", map[string]string{
		"email": addr, "password": password,
	}, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cookies := resp.Cookies()
	resp.Body.Close()
	return cookies
}
