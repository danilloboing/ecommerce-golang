// Package main seeds the catalog with demo data for local development.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/danilloboing/marketplace-golang/internal/config"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/infrastructure"
	internalpostgres "github.com/danilloboing/marketplace-golang/internal/platform/postgres"
)

type categoryDef struct {
	slug string
	name string
}

type productDef struct {
	slug        string
	name        string
	description string
	brand       string
	category    string
	priceCents  int64
	imageURL    string
	variants    []application.VariantInput
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := internalpostgres.NewPool(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := infrastructure.New(pool)
	admin := application.NewAdminService(repo)

	cats := map[string]uuid.UUID{}
	for _, c := range categoryDefs() {
		created, err := admin.CreateCategory(ctx, application.CreateCategoryInput{
			Slug: c.slug, Name: c.name,
		})
		if err != nil {
			return fmt.Errorf("create category %s: %w", c.slug, err)
		}
		cats[c.slug] = created.ID()
		slog.Info("seeded category", "slug", c.slug)
	}

	for _, p := range productDefs() {
		catID, ok := cats[p.category]
		if !ok {
			return fmt.Errorf("unknown category for product %s", p.slug)
		}
		created, err := admin.CreateProduct(ctx, application.CreateProductInput{
			Slug:           p.slug,
			Name:           p.name,
			Description:    p.description,
			Brand:          p.brand,
			CategoryID:     catID,
			BasePriceCents: p.priceCents,
			Currency:       "BRL",
			Status:         "published",
			Variants:       p.variants,
			Images: []application.ImageInput{
				{URL: p.imageURL, AltText: p.name, Position: 0},
			},
		})
		if err != nil {
			return fmt.Errorf("create product %s: %w", p.slug, err)
		}
		slog.Info("seeded product", "slug", p.slug)

		for _, v := range created.Variants() {
			if err := seedStock(ctx, pool, v.ID); err != nil {
				return fmt.Errorf("seed stock for variant %s: %w", v.ID, err)
			}
		}
	}

	return nil
}

func seedStock(ctx context.Context, pool *pgxpool.Pool, variantID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO inventory_stock (variant_id, available, reserved, version) VALUES ($1, 100, 0, 0) ON CONFLICT DO NOTHING`,
		variantID,
	)
	return err
}

func categoryDefs() []categoryDef {
	return []categoryDef{
		{"vestidos", "Vestidos"},
		{"blusas", "Blusas"},
		{"calcas", "Calças"},
		{"saias", "Saias"},
		{"acessorios", "Acessórios"},
	}
}

func productDefs() []productDef {
	stdVariants := []application.VariantInput{
		{SKU: "P", Size: "P", Color: "Padrão"},
		{SKU: "M", Size: "M", Color: "Padrão"},
		{SKU: "G", Size: "G", Color: "Padrão"},
	}

	products := make([]productDef, 0, 50)
	for i := 1; i <= 10; i++ {
		products = append(products, productDef{
			slug:        fmt.Sprintf("vestido-modelo-%02d", i),
			name:        fmt.Sprintf("Vestido Modelo %02d", i),
			description: "Vestido midi com tecido leve.",
			brand:       "AcmeFashion",
			category:    "vestidos",
			priceCents:  int64(8990 + i*100),
			imageURL:    fmt.Sprintf("https://placehold.co/600x800?text=Vestido+%02d", i),
			variants:    cloneVariants(stdVariants, fmt.Sprintf("VM-%02d", i)),
		})
	}
	for i := 1; i <= 10; i++ {
		products = append(products, productDef{
			slug:        fmt.Sprintf("blusa-modelo-%02d", i),
			name:        fmt.Sprintf("Blusa Modelo %02d", i),
			description: "Blusa de algodão.",
			brand:       "AcmeFashion",
			category:    "blusas",
			priceCents:  int64(5990 + i*100),
			imageURL:    fmt.Sprintf("https://placehold.co/600x800?text=Blusa+%02d", i),
			variants:    cloneVariants(stdVariants, fmt.Sprintf("BM-%02d", i)),
		})
	}
	for i := 1; i <= 10; i++ {
		products = append(products, productDef{
			slug:        fmt.Sprintf("calca-modelo-%02d", i),
			name:        fmt.Sprintf("Calça Modelo %02d", i),
			description: "Calça jeans.",
			brand:       "AcmeFashion",
			category:    "calcas",
			priceCents:  int64(11990 + i*200),
			imageURL:    fmt.Sprintf("https://placehold.co/600x800?text=Calca+%02d", i),
			variants:    cloneVariants(stdVariants, fmt.Sprintf("CM-%02d", i)),
		})
	}
	for i := 1; i <= 10; i++ {
		products = append(products, productDef{
			slug:        fmt.Sprintf("saia-modelo-%02d", i),
			name:        fmt.Sprintf("Saia Modelo %02d", i),
			description: "Saia midi rodada.",
			brand:       "AcmeFashion",
			category:    "saias",
			priceCents:  int64(7990 + i*150),
			imageURL:    fmt.Sprintf("https://placehold.co/600x800?text=Saia+%02d", i),
			variants:    cloneVariants(stdVariants, fmt.Sprintf("SM-%02d", i)),
		})
	}
	for i := 1; i <= 10; i++ {
		products = append(products, productDef{
			slug:        fmt.Sprintf("acessorio-modelo-%02d", i),
			name:        fmt.Sprintf("Acessório Modelo %02d", i),
			description: "Acessório fashion.",
			brand:       "AcmeFashion",
			category:    "acessorios",
			priceCents:  int64(2990 + i*50),
			imageURL:    fmt.Sprintf("https://placehold.co/400x400?text=Acessorio+%02d", i),
			variants: []application.VariantInput{
				{SKU: fmt.Sprintf("AC-%02d", i), Size: "U", Color: "Padrão"},
			},
		})
	}
	return products
}

func cloneVariants(template []application.VariantInput, prefix string) []application.VariantInput {
	out := make([]application.VariantInput, 0, len(template))
	for _, v := range template {
		out = append(out, application.VariantInput{
			SKU:   prefix + "-" + v.SKU,
			Size:  v.Size,
			Color: v.Color,
		})
	}
	return out
}
