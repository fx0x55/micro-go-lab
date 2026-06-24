-- +goose Up
-- +goose StatementBegin
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    product_name VARCHAR(256) NOT NULL,
    amount BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    version INTEGER NOT NULL DEFAULT 0,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd
CREATE INDEX idx_orders_user_id ON orders (user_id);
CREATE INDEX idx_orders_deleted_at ON orders (deleted_at) WHERE deleted_at IS NOT NULL;

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
DROP TABLE IF EXISTS orders;
