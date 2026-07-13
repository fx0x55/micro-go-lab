-- +goose Up

CREATE TABLE `known_users` (
    `user_id`  BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    `username` VARCHAR(64) NOT NULL,
    `seen_at`  DATETIME(3) NOT NULL DEFAULT NOW(3),
    INDEX `idx_known_users_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS `known_users`;
