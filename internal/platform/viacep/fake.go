package viacep

import "context"

// FakeClient is a test double for Lookuper.
type FakeClient struct {
	Responses map[string]Address
	Err       error
	Calls     map[string]int
}

var _ Lookuper = (*FakeClient)(nil)

// NewFakeClient builds an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{Responses: map[string]Address{}, Calls: map[string]int{}}
}

// Lookup returns a canned Address or Err.
func (f *FakeClient) Lookup(_ context.Context, cep string) (Address, error) {
	if f.Calls == nil {
		f.Calls = map[string]int{}
	}
	f.Calls[cep]++
	if f.Err != nil {
		return Address{}, f.Err
	}
	addr, ok := f.Responses[cep]
	if !ok {
		return Address{}, ErrCEPNotFound
	}
	return addr, nil
}
