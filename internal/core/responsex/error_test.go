package responsex_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/responsex"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteError_NotFoundReturns404(t *testing.T) {
	rec := httptest.NewRecorder()
	responsex.WriteError(rec, domain.ErrNotFound)

	require.Equal(t, http.StatusNotFound, rec.Code)
	var body map[string]map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "not_found", body["error"]["code"])
}

func TestWriteError_InvalidProductReturns400(t *testing.T) {
	rec := httptest.NewRecorder()
	responsex.WriteError(rec, domain.ErrInvalidProduct)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWriteError_BlankSearchQueryReturns400(t *testing.T) {
	rec := httptest.NewRecorder()
	responsex.WriteError(rec, application.ErrBlankSearchQuery)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWriteError_UnknownErrorReturns500(t *testing.T) {
	rec := httptest.NewRecorder()
	responsex.WriteError(rec, errors.New("boom"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestError_WritesJSONWithCodeAndMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)

	responsex.Error(rec, r, http.StatusForbidden, "csrf_invalid", "csrf token invalid")

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body struct {
		Error struct {
			Code, Message string
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "csrf_invalid", body.Error.Code)
	assert.Equal(t, "csrf token invalid", body.Error.Message)
}

func TestJSON_WritesPayloadWithStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	responsex.JSON(rec, http.StatusCreated, map[string]string{"id": "abc"})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "abc", body["id"])
}
