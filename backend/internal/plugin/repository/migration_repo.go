package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// MigrationRepository records which plugin-declared migrations have been
// applied against the database. Migrations are tracked separately from the
// plugins table so that uninstalling a plugin without --purge preserves its
// history for a later reinstall.
type MigrationRepository interface {
	// ListApplied returns {migrationID -> checksum} for the given plugin.
	// Missing plugin yields an empty map (not an error) so callers can
	// uniformly treat first-install and upgrade paths.
	ListApplied(ctx context.Context, pluginID string) (map[string]string, error)

	// RecordApplied inserts one applied-migration row inside the caller's
	// transaction. The runner uses the same tx that executed the DDL so
	// the DDL and the tracking row commit (or roll back) atomically.
	RecordApplied(ctx context.Context, tx *sql.Tx, pluginID, migrationID, checksum string) error

	// DeleteApplied removes every tracking row for the plugin. Called on
	// uninstall --purge after the schema has been torn down.
	DeleteApplied(ctx context.Context, pluginID string) error

	// DeleteByIDs removes specific tracking rows. Called when rolling back
	// individual migrations.
	DeleteByIDs(ctx context.Context, pluginID string, migrationIDs []string) error
}

// ErrNoDB is returned by the SQL-backed repo constructor when db is nil.
var ErrNoDB = errors.New("plugin migration repository: nil db handle")

// sqlMigrationRepository persists tracking rows via database/sql. We avoid
// ent here because the composite (plugin_id, migration_id) string primary
// key is not expressible through the project's global --idtype int64 ent
// codegen configuration.
type sqlMigrationRepository struct {
	db *sql.DB
}

// NewMigrationRepository wires the repository to the provided handle.
// Panics (via ErrNoDB) if db is nil, matching the strict-startup policy
// elsewhere in the codebase.
func NewMigrationRepository(db *sql.DB) (MigrationRepository, error) {
	if db == nil {
		return nil, ErrNoDB
	}
	return &sqlMigrationRepository{db: db}, nil
}

// ListApplied implements MigrationRepository.
func (r *sqlMigrationRepository) ListApplied(ctx context.Context, pluginID string) (map[string]string, error) {
	const q = `SELECT migration_id, checksum FROM plugin_migrations WHERE plugin_id = $1`
	rows, err := r.db.QueryContext(ctx, q, pluginID)
	if err != nil {
		return nil, fmt.Errorf("plugin migration repository: list applied: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]string)
	for rows.Next() {
		var id, sum string
		if err := rows.Scan(&id, &sum); err != nil {
			return nil, fmt.Errorf("plugin migration repository: scan: %w", err)
		}
		out[id] = sum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plugin migration repository: rows: %w", err)
	}
	return out, nil
}

// RecordApplied implements MigrationRepository.
func (r *sqlMigrationRepository) RecordApplied(ctx context.Context, tx *sql.Tx, pluginID, migrationID, checksum string) error {
	if tx == nil {
		return fmt.Errorf("plugin migration repository: record applied requires a transaction")
	}
	const stmt = `
INSERT INTO plugin_migrations (plugin_id, migration_id, checksum)
VALUES ($1, $2, $3)
ON CONFLICT (plugin_id, migration_id) DO UPDATE SET checksum = EXCLUDED.checksum`
	if _, err := tx.ExecContext(ctx, stmt, pluginID, migrationID, checksum); err != nil {
		return fmt.Errorf("plugin migration repository: record %s/%s: %w", pluginID, migrationID, err)
	}
	return nil
}

// DeleteApplied implements MigrationRepository.
func (r *sqlMigrationRepository) DeleteApplied(ctx context.Context, pluginID string) error {
	const stmt = `DELETE FROM plugin_migrations WHERE plugin_id = $1`
	if _, err := r.db.ExecContext(ctx, stmt, pluginID); err != nil {
		return fmt.Errorf("plugin migration repository: delete all for %s: %w", pluginID, err)
	}
	return nil
}

// DeleteByIDs implements MigrationRepository.
func (r *sqlMigrationRepository) DeleteByIDs(ctx context.Context, pluginID string, migrationIDs []string) error {
	if len(migrationIDs) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(migrationIDs))
	args := make([]any, 0, len(migrationIDs)+1)
	args = append(args, pluginID)
	for i, id := range migrationIDs {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+2))
		args = append(args, id)
	}
	stmt := fmt.Sprintf(
		`DELETE FROM plugin_migrations WHERE plugin_id = $1 AND migration_id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	if _, err := r.db.ExecContext(ctx, stmt, args...); err != nil {
		return fmt.Errorf("plugin migration repository: delete by ids for %s: %w", pluginID, err)
	}
	return nil
}
