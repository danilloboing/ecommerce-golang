//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogE2E_AdminCreateAndPublicList(t *testing.T) {
	dsn := testutil.NewTestPostgresURL(t)
	testutil.ApplyMigrations(t, dsn)
	addr := testutil.NewTestRedisAddr(t)

	port := "18092"

	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bin := testutil.BuildAPIBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"APP_PORT="+port,
		"APP_ENV=test",
		"APP_LOG_LEVEL=warn",
		"DATABASE_URL="+dsn,
		"REDIS_ADDR="+addr,
		"ADMIN_API_TOKEN=secret",
		"CORS_ALLOWED_ORIGINS=http://test.local",
	)
	var logBuf bytes.Buffer
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGINT)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		cancel()
		if t.Failed() {
			t.Logf("=== api logs ===\n%s\n=== end ===", logBuf.String())
		}
	})

	base := "http://127.0.0.1:" + port
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 200*time.Millisecond)

	// Wait for /ready (Postgres + Redis reachable from API).
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/ready")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 15*time.Second, 200*time.Millisecond, "api never became ready")

	// Create category
	resp := postJSON(t, base+"/admin/categories", "secret", map[string]any{
		"slug": "vestidos",
		"name": "Vestidos",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var catBody map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&catBody))
	resp.Body.Close()
	categoryID, err := uuid.Parse(catBody["id"])
	require.NoError(t, err)

	// Create product
	resp = postJSON(t, base+"/admin/products", "secret", map[string]any{
		"slug":           "vestido-azul",
		"name":           "Vestido Azul",
		"description":    "x",
		"brand":          "AcmeFashion",
		"categoryId":     categoryID.String(),
		"basePriceCents": 9990,
		"currency":       "BRL",
		"status":         "published",
		"variants": []map[string]any{
			{"sku": "VA-P", "size": "P", "color": "Azul"},
		},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Public list
	listResp, err := http.Get(base + "/products")
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	body, err := io.ReadAll(listResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"slug":"vestido-azul"`)

	// Public detail
	detail, err := http.Get(base + "/products/vestido-azul")
	require.NoError(t, err)
	defer detail.Body.Close()
	require.Equal(t, http.StatusOK, detail.StatusCode)
	detailBody, err := io.ReadAll(detail.Body)
	require.NoError(t, err)
	assert.Contains(t, string(detailBody), `"name":"Vestido Azul"`)

	// Public search
	search, err := http.Get(base + "/search?q=azul")
	require.NoError(t, err)
	defer search.Body.Close()
	require.Equal(t, http.StatusOK, search.StatusCode)

	// Public categories
	cats, err := http.Get(base + "/categories")
	require.NoError(t, err)
	defer cats.Body.Close()
	require.Equal(t, http.StatusOK, cats.StatusCode)
	catsBody, err := io.ReadAll(cats.Body)
	require.NoError(t, err)
	assert.Contains(t, string(catsBody), `"slug":"vestidos"`)

	// Admin auth check (no token = 401)
	noAuth, err := http.Post(base+"/admin/products", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	noAuth.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, noAuth.StatusCode)
}

func postJSON(t *testing.T, url, token string, payload map[string]any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}
