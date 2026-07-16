-- +goose Up

ALTER TABLE `orders`
    ADD COLUMN `sku`           VARCHAR(32) NULL AFTER `amount`,
    ADD COLUMN `quantity`      INT NOT NULL DEFAULT 1 AFTER `sku`,
    ADD COLUMN `cancel_reason` VARCHAR(32) NULL AFTER `status`;

-- +goose Down

ALTER TABLE `orders`
    DROP COLUMN `cancel_reason`,
    DROP COLUMN `quantity`,
    DROP COLUMN `sku`;
