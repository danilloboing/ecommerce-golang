-- 20260509000001_image_variants.sql
-- Image variants stored alongside the original URL for CDN delivery.
ALTER TABLE catalog_images
    ADD COLUMN url_thumb TEXT,
    ADD COLUMN url_medium TEXT,
    ADD COLUMN url_large TEXT,
    ADD COLUMN storage_key TEXT;

COMMENT ON COLUMN catalog_images.storage_key IS 'R2 object key prefix (e.g. products/{pid}/{iid})';
