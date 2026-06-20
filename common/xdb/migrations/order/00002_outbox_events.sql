-- +goose Up
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

-- +goose Down
DROP TABLE IF EXISTS outbox_events;
