-- +goose Up

CREATE TABLE `orders` (
    `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `user_id`      BIGINT UNSIGNED NOT NULL,
    `product_name` VARCHAR(256) NOT NULL,
    `amount`       BIGINT NOT NULL,
    `status`       VARCHAR(32) NOT NULL DEFAULT 'pending',
    `version`      INT NOT NULL DEFAULT 0,
    `deleted_at`   DATETIME(3) DEFAULT NULL,
    `created_at`   DATETIME(3) NOT NULL DEFAULT NOW(3),
    `updated_at`   DATETIME(3) NOT NULL DEFAULT NOW(3),
    INDEX `idx_orders_user_id` (`user_id`),
    INDEX `idx_orders_deleted_at` (`deleted_at`)
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

-- +goose Down
DROP TABLE IF EXISTS `processed_events`;
DROP TABLE IF EXISTS `outbox_events`;
DROP TABLE IF EXISTS `orders`;
