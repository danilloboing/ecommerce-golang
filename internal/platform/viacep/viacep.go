// Package viacep wraps the ViaCEP postal-code API behind a Redis cache.
package viacep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

// Sentinel errors.
var (
	ErrCEPNotFound = errors.New("viacep: cep not found")
	ErrInvalidCEP  = errors.New("viacep: invalid cep")
)

var cepPattern = regexp.MustCompile(`^[0-9]{8}$`)

// Address is a resolved postal address (subset of the ViaCEP payload).
type Address struct {
	PostalCode   string
	Street       string
	Neighborhood string
	City         string
	State        string
}

// Lookuper resolves a CEP to an Address. Implemented by *Client and *FakeClient.
type Lookuper interface {
	Lookup(ctx context.Context, cep string) (Address, error)
}

// Client is the cached ViaCEP HTTP client.
type Client struct {
	httpClient *http.Client
	cache      *redis.Client
	baseURL    string
	cacheTTL   time.Duration
}

var _ Lookuper = (*Client)(nil)

// NewClient builds a Client. baseURL has no trailing slash (e.g. https://viacep.com.br/ws).
func NewClient(httpClient *http.Client, cache *redis.Client, baseURL string, cacheTTL time.Duration) *Client {
	return &Client{httpClient: httpClient, cache: cache, baseURL: baseURL, cacheTTL: cacheTTL}
}

type viacepResponse struct {
	Cep         string `json:"cep"`
	Logradouro  string `json:"logradouro"`
	Bairro      string `json:"bairro"`
	Localidade  string `json:"localidade"`
	UF          string `json:"uf"`
	Erro        any    `json:"erro"`
}

// Lookup resolves cep (8 digits, no mask). Cache hit short-circuits the HTTP call.
func (c *Client) Lookup(ctx context.Context, cep string) (Address, error) {
	if !cepPattern.MatchString(cep) {
		return Address{}, ErrInvalidCEP
	}

	cacheKey := "viacep:" + cep
	if raw, err := c.cache.Get(ctx, cacheKey).Bytes(); err == nil {
		var cached Address
		if json.Unmarshal(raw, &cached) == nil {
			return cached, nil
		}
	}

	url := fmt.Sprintf("%s/%s/json/", c.baseURL, cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Address{}, fmt.Errorf("viacep: build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Address{}, fmt.Errorf("viacep: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Address{}, fmt.Errorf("viacep: unexpected status %d", resp.StatusCode)
	}

	var body viacepResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Address{}, fmt.Errorf("viacep: decode: %w", err)
	}
	if body.Erro != nil && body.Erro != false {
		return Address{}, ErrCEPNotFound
	}

	addr := Address{
		PostalCode:   cep,
		Street:       body.Logradouro,
		Neighborhood: body.Bairro,
		City:         body.Localidade,
		State:        body.UF,
	}

	if encoded, err := json.Marshal(addr); err == nil {
		_ = c.cache.Set(ctx, cacheKey, encoded, c.cacheTTL).Err()
	}

	return addr, nil
}
