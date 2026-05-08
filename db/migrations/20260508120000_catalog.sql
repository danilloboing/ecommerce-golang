-- 20260508120000_catalog.sql
-- Catalog schema: categories, products, variants, images, stock.

CREATE TABLE catalog_categories (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    parent_id UUID REFERENCES catalog_categories(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_catalog_categories_parent ON catalog_categories(parent_id);

CREATE TABLE catalog_products (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    brand TEXT NOT NULL DEFAULT '',
    category_id UUID NOT NULL REFERENCES catalog_categories(id) ON DELETE RESTRICT,
    base_price_cents BIGINT NOT NULL CHECK (base_price_cents >= 0),
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'published', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_catalog_products_category ON catalog_products(category_id);
CREATE INDEX idx_catalog_products_status ON catalog_products(status);
CREATE INDEX idx_catalog_products_brand ON catalog_products(brand) WHERE brand <> '';
CREATE INDEX idx_catalog_products_created_at ON catalog_products(created_at DESC, id DESC);

CREATE TABLE catalog_variants (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES catalog_products(id) ON DELETE CASCADE,
    sku TEXT NOT NULL,
    size TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    price_cents BIGINT CHECK (price_cents IS NULL OR price_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, sku)
);

CREATE INDEX idx_catalog_variants_product ON catalog_variants(product_id);
CREATE INDEX idx_catalog_variants_size ON catalog_variants(size) WHERE size <> '';
CREATE INDEX idx_catalog_variants_color ON catalog_variants(color) WHERE color <> '';

CREATE TABLE catalog_images (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES catalog_products(id) ON DELETE CASCADE,
    variant_id UUID REFERENCES catalog_variants(id) ON DELETE SET NULL,
    url TEXT NOT NULL,
    alt_text TEXT NOT NULL DEFAULT '',
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_catalog_images_product ON catalog_images(product_id, position);
CREATE INDEX idx_catalog_images_variant ON catalog_images(variant_id) WHERE variant_id IS NOT NULL;

CREATE TABLE catalog_stock (
    variant_id UUID PRIMARY KEY REFERENCES catalog_variants(id) ON DELETE CASCADE,
    available INT NOT NULL DEFAULT 0 CHECK (available >= 0),
    reserved INT NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    version BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
