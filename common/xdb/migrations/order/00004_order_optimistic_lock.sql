-- +goose Up
ALTER TABLE orders ADD COLUMN version INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE orders DROP COLUMN version;
