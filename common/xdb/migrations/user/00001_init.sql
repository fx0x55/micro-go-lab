-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL,
    password VARCHAR(256) NOT NULL,
    email VARCHAR(128) NOT NULL,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd
CREATE UNIQUE INDEX idx_users_username ON users (username);
CREATE UNIQUE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_deleted_at ON users (deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose StatementBegin
CREATE TABLE todos (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    title VARCHAR(256) NOT NULL,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd
CREATE INDEX idx_todos_user_id ON todos (user_id);
CREATE INDEX idx_todos_deleted_at ON todos (deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose StatementBegin
CREATE TABLE outbox_events (
    id           BIGSERIAL PRIMARY KEY,
    event_id     UUID NOT NULL UNIQUE,
    topic        VARCHAR(255) NOT NULL,
    event_key    VARCHAR(255) NOT NULL,
    event_type   VARCHAR(100) NOT NULL,
    version      INT NOT NULL DEFAULT 1,
    payload      JSONB NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'pending',
    retry_count  INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at      TIMESTAMPTZ
);
-- +goose StatementEnd
CREATE INDEX idx_outbox_events_pending ON outbox_events (created_at) WHERE status = 'pending';

-- +goose StatementBegin
CREATE TABLE processed_events (
    event_id     UUID NOT NULL PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS processed_events;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS todos;
DROP TABLE IF EXISTS users;
