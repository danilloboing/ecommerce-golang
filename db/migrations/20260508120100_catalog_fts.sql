-- 20260508120100_catalog_fts.sql
-- Add tsvector for product full-text search and trigram index for fuzzy lookups.

ALTER TABLE catalog_products
    ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('portuguese', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('portuguese', coalesce(brand, '')), 'B') ||
        setweight(to_tsvector('portuguese', coalesce(description, '')), 'C')
    ) STORED;

CREATE INDEX idx_catalog_products_search ON catalog_products USING GIN (search_vector);

-- Trigram index on name for fuzzy/typo-tolerant lookups.
CREATE INDEX idx_catalog_products_name_trgm ON catalog_products USING GIN (name gin_trgm_ops);
