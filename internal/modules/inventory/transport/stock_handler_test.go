package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/core/adminauth"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/transport"
)

const testAdminToken = "test-admin-secret"

// stubStockSetter is a minimal fake that satisfies StockSetter.
type stubStockSetter struct {
	returnStock domain.Stock
	returnErr   error
}

func (s *stubStockSetter) SetStock(_ context.Context, variantID uuid.UUID, available, version int) (domain.Stock, error) {
	if s.returnErr != nil {
		return domain.Stock{}, s.returnErr
	}
	return domain.Stock{
		VariantID: variantID,
		Available: available,
		Reserved:  0,
		Version:   version + 1,
	}, nil
}

func newTestRouter(svc transport.StockSetter) *chi.Mux {
	r := chi.NewRouter()
	r.Group(func(admin chi.Router) {
		admin.Use(adminauth.RequireToken(testAdminToken))
		h := transport.NewStockHandler(svc)
		h.RegisterStockRoutes(admin)
	})
	return r
}

func TestStockHandler_SetStock_200(t *testing.T) {
	svc := &stubStockSetter{}
	r := newTestRouter(svc)

	variantID := uuid.New()
	body := map[string]any{"available": 50, "version": 0}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/admin/variants/"+variantID.String()+"/stock", bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, variantID.String(), resp["variantId"])
	assert.Equal(t, float64(50), resp["available"])
}

func TestStockHandler_SetStock_409_VersionConflict(t *testing.T) {
	svc := &stubStockSetter{returnErr: domain.ErrStockConflict}
	r := newTestRouter(svc)

	variantID := uuid.New()
	body := map[string]any{"available": 50, "version": 0}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/admin/variants/"+variantID.String()+"/stock", bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok, "expected error object in response")
	assert.Equal(t, "stock_conflict", errObj["code"])
}

func TestStockHandler_SetStock_401_MissingToken(t *testing.T) {
	svc := &stubStockSetter{}
	r := newTestRouter(svc)

	variantID := uuid.New()
	body := map[string]any{"available": 50, "version": 0}
	buf, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/admin/variants/"+variantID.String()+"/stock", bytes.NewReader(buf))
	// deliberately omit Authorization header
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
