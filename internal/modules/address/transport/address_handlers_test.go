package transport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/transport"
	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

func TestCEPHandler_Success(t *testing.T) {
	fake := viacep.NewFakeClient()
	fake.Responses["01001000"] = viacep.Address{
		PostalCode:   "01001000",
		Street:       "Sé",
		Neighborhood: "Sé",
		City:         "São Paulo",
		State:        "SP",
	}

	h := transport.NewCEPHandler(fake)
	r := chi.NewRouter()
	h.RegisterCEPRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/address/cep/01001000")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCEPHandler_NotFound(t *testing.T) {
	fake := viacep.NewFakeClient()
	h := transport.NewCEPHandler(fake)
	r := chi.NewRouter()
	h.RegisterCEPRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/address/cep/99999999")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
