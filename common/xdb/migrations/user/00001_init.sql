-- +goose Up

CREATE TABLE `users` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `username`   VARCHAR(64) NOT NULL,
    `password`   VARCHAR(256) NOT NULL,
    `email`      VARCHAR(128) NOT NULL,
    `deleted_at` DATETIME(3) DEFAULT NULL,
    `created_at` DATETIME(3) NOT NULL DEFAULT NOW(3),
    `updated_at` DATETIME(3) NOT NULL DEFAULT NOW(3),
    UNIQUE INDEX `idx_users_username` (`username`),
    UNIQUE INDEX `idx_users_email` (`email`),
    INDEX `idx_users_deleted_at` (`deleted_at`)
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
DROP TABLE IF EXISTS `users`;
