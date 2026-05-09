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
