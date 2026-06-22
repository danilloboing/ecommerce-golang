-- checkout: freeze the recipient address at quote time
-- Without this column the quote's address snapshot was dropped on persist, so
-- orders.address_snapshot (JSONB NOT NULL) was always written as '{}'.
ALTER TABLE checkout_quotes
  ADD COLUMN address_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
