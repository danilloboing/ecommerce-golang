package viacep_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danilloboing/marketplace-golang/internal/platform/viacep"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestClient_Lookup_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/01001000/json/", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cep":"01001-000","logradouro":"Praça da Sé","bairro":"Sé","localidade":"São Paulo","uf":"SP"}`))
	}))
	defer srv.Close()

	c := viacep.NewClient(srv.Client(), newTestRedis(t), srv.URL, time.Hour)
	addr, err := c.Lookup(context.Background(), "01001000")
	require.NoError(t, err)
	assert.Equal(t, "01001000", addr.PostalCode)
	assert.Equal(t, "Praça da Sé", addr.Street)
	assert.Equal(t, "Sé", addr.Neighborhood)
	assert.Equal(t, "São Paulo", addr.City)
	assert.Equal(t, "SP", addr.State)
}

func TestClient_Lookup_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"erro": true}`))
	}))
	defer srv.Close()

	c := viacep.NewClient(srv.Client(), newTestRedis(t), srv.URL, time.Hour)
	_, err := c.Lookup(context.Background(), "00000000")
	require.ErrorIs(t, err, viacep.ErrCEPNotFound)
}

func TestClient_Lookup_InvalidCEP(t *testing.T) {
	c := viacep.NewClient(http.DefaultClient, newTestRedis(t), "http://unused", time.Hour)
	_, err := c.Lookup(context.Background(), "123")
	require.ErrorIs(t, err, viacep.ErrInvalidCEP)
}

func TestClient_Lookup_CachesResult(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"cep":"01001-000","logradouro":"X","bairro":"Y","localidade":"Z","uf":"SP"}`))
	}))
	defer srv.Close()

	c := viacep.NewClient(srv.Client(), newTestRedis(t), srv.URL, time.Hour)
	_, err := c.Lookup(context.Background(), "01001000")
	require.NoError(t, err)
	_, err = c.Lookup(context.Background(), "01001000")
	require.NoError(t, err)
	assert.Equal(t, 1, hits, "second lookup must be served from cache")
}
