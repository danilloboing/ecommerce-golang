// Package transport contains HTTP handlers for the address module.
package transport

import "github.com/danilloboing/marketplace-golang/internal/modules/address/domain"

// AddressResponse is the JSON shape of an address.
type AddressResponse struct {
	ID            string  `json:"id"`
	RecipientName string  `json:"recipient_name"`
	PostalCode    string  `json:"postal_code"`
	Street        string  `json:"street"`
	Number        string  `json:"number"`
	Complement    *string `json:"complement,omitempty"`
	Neighborhood  string  `json:"neighborhood"`
	City          string  `json:"city"`
	State         string  `json:"state"`
	IsDefault     bool    `json:"is_default"`
}

// CEPResponse is the JSON shape of a ViaCEP lookup.
type CEPResponse struct {
	PostalCode   string `json:"postal_code"`
	Street       string `json:"street"`
	Neighborhood string `json:"neighborhood"`
	City         string `json:"city"`
	State        string `json:"state"`
}

func toAddressResponse(a domain.Address) AddressResponse {
	return AddressResponse{
		ID:            a.ID.String(),
		RecipientName: a.RecipientName,
		PostalCode:    a.PostalCode,
		Street:        a.Street,
		Number:        a.Number,
		Complement:    a.Complement,
		Neighborhood:  a.Neighborhood,
		City:          a.City,
		State:         a.State,
		IsDefault:     a.IsDefault,
	}
}
