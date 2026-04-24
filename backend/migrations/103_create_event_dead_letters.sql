-- 103_create_event_dead_letters.sql
-- Stores failed AsyncHook deliveries after the dispatcher exhausts its
-- retry budget. Operators inspect rows via the Admin API and can re-queue
-- individual entries.

CREATE TABLE IF NOT EXISTS event_dead_letters (
    id              BIGSERIAL PRIMARY KEY,
    topic           TEXT NOT NULL,
    payload         JSONB NOT NULL,
    first_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempt_count   INT NOT NULL DEFAULT 1,
    last_error      TEXT,
    subscriber_tag  TEXT,
    correlation_id  TEXT
);

CREATE INDEX IF NOT EXISTS event_dead_letters_topic
    ON event_dead_letters(topic, first_failed_at DESC);
