-- Enable required extensions for the marketplace.
-- pg_trgm: trigram similarity for fuzzy product search (Phase 1b).
-- citext:  case-insensitive text type for emails and similar fields.
-- pgcrypto: gen_random_uuid + crypto helpers (used for legacy uuid v4 if needed).
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
