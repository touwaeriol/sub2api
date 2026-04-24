-- 101_create_plugins_table.sql
-- Registry of every plugin ever installed on this deployment.
-- Lifecycle: NotInstalled -> Installed -> Disabled <-> Enabled.

CREATE TABLE IF NOT EXISTS plugins (
    id               TEXT PRIMARY KEY,
    version          TEXT NOT NULL,
    api_version      TEXT NOT NULL,
    state            TEXT NOT NULL DEFAULT 'installed',
    installed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_enabled_at  TIMESTAMPTZ,
    declared_tables  JSONB NOT NULL DEFAULT '[]'::JSONB,
    meta_snapshot    JSONB
);

CREATE INDEX IF NOT EXISTS plugins_state ON plugins(state);
