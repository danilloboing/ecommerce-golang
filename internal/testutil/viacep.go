package testutil

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// ViaCEPFixture is an httptest ViaCEP server with a hit counter.
type ViaCEPFixture struct {
	Server *httptest.Server
	hits   atomic.Int64
}

// Hits returns how many times the fixture was called.
func (f *ViaCEPFixture) Hits() int64 { return f.hits.Load() }

// NewViaCEPFixture serves a fixed payload for any /<cep>/json/ path and counts hits.
func NewViaCEPFixture(t *testing.T) *ViaCEPFixture {
	t.Helper()
	f := &ViaCEPFixture{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cep":"01001-000","logradouro":"Praça da Sé","bairro":"Sé","localidade":"São Paulo","uf":"SP"}`))
	}))
	t.Cleanup(f.Server.Close)
	return f
}
