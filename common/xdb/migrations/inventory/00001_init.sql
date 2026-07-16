-- +goose Up

CREATE TABLE `products` (
    `sku`        VARCHAR(64) NOT NULL PRIMARY KEY,
    `total`      INT NOT NULL,
    `available`  INT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `inventory_reservations` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `order_id`   BIGINT UNSIGNED NOT NULL,
    `sku`        VARCHAR(64) NOT NULL,
    `quantity`   INT NOT NULL,
    `status`     VARCHAR(32) NOT NULL DEFAULT 'reserved',
    `created_at` DATETIME(3) NOT NULL DEFAULT NOW(3),
    UNIQUE INDEX `idx_reservations_order_id` (`order_id`),
    INDEX `idx_reservations_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `outbox_events` (
    `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `event_id`    CHAR(36) NOT NULL,
    `topic`       VARCHAR(255) NOT NULL,
    `event_key`   VARCHAR(255) NOT NULL,
    `event_type`  VARCHAR(100) NOT NULL,
    `version`     INT NOT NULL DEFAULT 1,
    `payload`     JSON NOT NULL,
    `status`      VARCHAR(20) NOT NULL DEFAULT 'pending',
    `retry_count` INT NOT NULL DEFAULT 0,
    `last_error`  TEXT,
    `created_at`  DATETIME(3) NOT NULL DEFAULT NOW(3),
    `sent_at`     DATETIME(3) DEFAULT NULL,
    UNIQUE INDEX `idx_outbox_events_event_id` (`event_id`),
    INDEX `idx_outbox_events_pending` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `processed_events` (
    `event_id`     CHAR(36) NOT NULL PRIMARY KEY,
    `processed_at` DATETIME(3) NOT NULL DEFAULT NOW(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 初始化示例商品（教学用）
INSERT INTO `products` (`sku`, `total`, `available`) VALUES
    ('SKU-001', 100, 100),
    ('SKU-002', 50, 50),
    ('SKU-003', 10, 10);

-- +goose Down
DROP TABLE IF EXISTS `processed_events`;
DROP TABLE IF EXISTS `outbox_events`;
DROP TABLE IF EXISTS `inventory_reservations`;
DROP TABLE IF EXISTS `products`;
