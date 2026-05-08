// Package transport adapts catalog services to HTTP.
package transport

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

// ProductResponse is the public-facing product DTO.
type ProductResponse struct {
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Brand       string            `json:"brand"`
	CategoryID  string            `json:"categoryId"`
	BasePrice   MoneyResponse     `json:"basePrice"`
	Status      string            `json:"status"`
	Variants    []VariantResponse `json:"variants"`
	Images      []ImageResponse   `json:"images"`
}

// MoneyResponse mirrors domain.Money for JSON output.
type MoneyResponse struct {
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
}

// VariantResponse is the public-facing variant DTO.
type VariantResponse struct {
	ID    string         `json:"id"`
	SKU   string         `json:"sku"`
	Size  string         `json:"size"`
	Color string         `json:"color"`
	Price *MoneyResponse `json:"price,omitempty"`
}

// ImageResponse is the public-facing image DTO.
type ImageResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	AltText  string `json:"altText"`
	Position int    `json:"position"`
}

// CategoryResponse is the public-facing category DTO.
type CategoryResponse struct {
	ID       string  `json:"id"`
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	ParentID *string `json:"parentId,omitempty"`
}

func toProductResponse(p domain.Product) ProductResponse {
	r := ProductResponse{
		ID:          p.ID().String(),
		Slug:        p.Slug().String(),
		Name:        p.Name(),
		Description: p.Description(),
		Brand:       p.Brand(),
		CategoryID:  p.CategoryID().String(),
		BasePrice: MoneyResponse{
			AmountCents: p.BasePrice().AmountCents(),
			Currency:    p.BasePrice().Currency(),
		},
		Status:   string(p.Status()),
		Variants: make([]VariantResponse, 0, len(p.Variants())),
		Images:   make([]ImageResponse, 0, len(p.Images())),
	}
	for _, v := range p.Variants() {
		vr := VariantResponse{
			ID:    v.ID.String(),
			SKU:   v.SKU,
			Size:  v.Size,
			Color: v.Color,
		}
		if v.Price != nil {
			vr.Price = &MoneyResponse{
				AmountCents: v.Price.AmountCents(),
				Currency:    v.Price.Currency(),
			}
		}
		r.Variants = append(r.Variants, vr)
	}
	for _, img := range p.Images() {
		r.Images = append(r.Images, ImageResponse{
			ID:       img.ID.String(),
			URL:      img.URL,
			AltText:  img.AltText,
			Position: img.Position,
		})
	}
	return r
}

func toCategoryResponse(c domain.Category) CategoryResponse {
	r := CategoryResponse{
		ID:   c.ID().String(),
		Slug: c.Slug().String(),
		Name: c.Name(),
	}
	if pid := c.ParentID(); pid != nil {
		s := pid.String()
		r.ParentID = &s
	}
	return r
}
