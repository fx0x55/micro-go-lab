-- +goose Up
-- +goose StatementBegin
CREATE TABLE processed_events (
    event_id     UUID NOT NULL PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS processed_events;
