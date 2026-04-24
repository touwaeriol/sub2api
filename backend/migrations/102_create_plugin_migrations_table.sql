-- 102_create_plugin_migrations_table.sql
-- Tracks which plugin-declared migrations have been applied, with a
-- checksum guard against silent edits to a committed migration.

CREATE TABLE IF NOT EXISTS plugin_migrations (
    plugin_id     TEXT NOT NULL,
    migration_id  TEXT NOT NULL,
    applied_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    checksum      TEXT NOT NULL,
    PRIMARY KEY (plugin_id, migration_id)
);
