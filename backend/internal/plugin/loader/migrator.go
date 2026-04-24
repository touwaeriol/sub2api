package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"entgo.io/ent/dialect"
)

// Errors returned by the migrator.
var (
	// ErrChecksumMismatch is returned when an already-applied migration
	// has a different checksum on disk. Editing a committed migration is
	// treated as a programming error.
	ErrChecksumMismatch = errors.New("plugin loader: migration checksum mismatch")

	// ErrMigrationNotReversible is returned by Rollback when a migration
	// lacks a Down counterpart.
	ErrMigrationNotReversible = errors.New("plugin loader: migration not reversible")

	// ErrEmptyStep is returned when a Migration has neither SQL nor Fn.
	ErrEmptyStep = errors.New("plugin loader: migration has empty step")
)

// Migrator drives plugin-declared migrations: SchemaProvider upgrades plus
// ordered, checksum-verified Migration entries. It owns the transaction
// boundary and delegates persistence to a MigrationRepository.
type Migrator struct {
	repo   repository.MigrationRepository
	db     *sql.DB
	driver dialect.Driver
	log    *slog.Logger
}

// NewMigrator wires the migrator with its persistence and driver.
func NewMigrator(
	repo repository.MigrationRepository,
	db *sql.DB,
	driver dialect.Driver,
	log *slog.Logger,
) *Migrator {
	if log == nil {
		log = slog.Default()
	}
	return &Migrator{repo: repo, db: db, driver: driver, log: log}
}

// Apply runs every Meta.Migrations entry that is not already recorded,
// in ascending migration ID order, and finishes with Meta.Schema.
//
// Each migration executes in its own transaction so partial progress is
// rolled back on error. Checksum drift is detected before any Up runs.
func (m *Migrator) Apply(ctx context.Context, p plugin.Plugin) error {
	meta := p.Meta()
	applied, err := m.repo.ListApplied(ctx, meta.ID)
	if err != nil {
		return err
	}
	sorted := sortedMigrations(meta.Migrations)
	if err := verifyChecksums(meta.ID, sorted, applied); err != nil {
		return err
	}
	for _, mig := range sorted {
		if _, done := applied[mig.ID]; done {
			continue
		}
		if err := m.applyOne(ctx, meta.ID, mig); err != nil {
			return err
		}
		m.log.Info("plugin migration applied", "plugin", meta.ID, "migration", mig.ID)
	}
	if meta.Schema != nil {
		if err := meta.Schema.CreateOrUpgrade(ctx, m.driver); err != nil {
			return fmt.Errorf("plugin loader: schema upgrade %q: %w", meta.ID, err)
		}
	}
	return nil
}

// applyOne runs a single migration inside a transaction and records it.
func (m *Migrator) applyOne(ctx context.Context, pluginID string, mig plugin.Migration) error {
	up := mig.UpStep()
	if up.IsEmpty() {
		return fmt.Errorf("%w: %s/%s", ErrEmptyStep, pluginID, mig.ID)
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("plugin loader: begin tx for %s/%s: %w", pluginID, mig.ID, err)
	}
	if err := runStep(ctx, tx, up); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("plugin loader: apply %s/%s: %w", pluginID, mig.ID, err)
	}
	if err := m.repo.RecordApplied(ctx, tx, pluginID, mig.ID, mig.Checksum()); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("plugin loader: commit %s/%s: %w", pluginID, mig.ID, err)
	}
	return nil
}

// Rollback replays Down steps in reverse ID order for every applied
// migration whose ID is strictly greater than toMigrationID. An empty
// toMigrationID rolls back every applied migration.
//
// The function refuses to start if any candidate migration is missing a
// Down step (ErrMigrationNotReversible), so partial rollbacks don't leave
// the database in an in-between state. Each Down runs in its own tx and
// deletes the corresponding plugin_migrations row on commit.
func (m *Migrator) Rollback(ctx context.Context, p plugin.Plugin, toMigrationID string) error {
	meta := p.Meta()
	applied, err := m.repo.ListApplied(ctx, meta.ID)
	if err != nil {
		return err
	}
	candidates := collectRollbackCandidates(meta.Migrations, applied, toMigrationID)
	if err := ensureReversible(meta.ID, candidates); err != nil {
		return err
	}
	for _, mig := range candidates {
		if err := m.rollbackOne(ctx, meta.ID, mig.ID, mig.DownStep()); err != nil {
			return err
		}
		m.log.Info("plugin migration rolled back", "plugin", meta.ID, "migration", mig.ID)
	}
	return nil
}

// collectRollbackCandidates returns applied migrations with ID > toMigrationID
// in descending ID order so the newest migration is undone first.
func collectRollbackCandidates(
	all []plugin.Migration,
	applied map[string]string,
	toMigrationID string,
) []plugin.Migration {
	sorted := sortedMigrations(all)
	out := make([]plugin.Migration, 0, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		mig := sorted[i]
		if _, done := applied[mig.ID]; !done {
			continue
		}
		if toMigrationID != "" && mig.ID <= toMigrationID {
			continue
		}
		out = append(out, mig)
	}
	return out
}

// ensureReversible pre-validates every candidate has a Down step so we fail
// fast before mutating the database.
func ensureReversible(pluginID string, migs []plugin.Migration) error {
	for _, mig := range migs {
		if mig.DownStep().IsEmpty() {
			return fmt.Errorf("%w: %s/%s", ErrMigrationNotReversible, pluginID, mig.ID)
		}
	}
	return nil
}

// rollbackOne executes the Down step in a transaction then deletes the
// tracking row.
func (m *Migrator) rollbackOne(ctx context.Context, pluginID, migrationID string, step plugin.MigrationStep) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("plugin loader: begin tx for rollback %s/%s: %w",
			pluginID, migrationID, err)
	}
	if err := runStep(ctx, tx, step); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("plugin loader: rollback %s/%s: %w", pluginID, migrationID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("plugin loader: commit rollback %s/%s: %w",
			pluginID, migrationID, err)
	}
	if err := m.repo.DeleteByIDs(ctx, pluginID, []string{migrationID}); err != nil {
		return err
	}
	return nil
}

// Drop tears down every table owned by the plugin (reverse declaration
// order, CASCADE) and clears the plugin_migrations history. Destructive —
// only called from Loader.Uninstall when purge=true.
func (m *Migrator) Drop(ctx context.Context, p plugin.Plugin) error {
	meta := p.Meta()
	for i := len(meta.Tables) - 1; i >= 0; i-- {
		table := meta.Tables[i]
		stmt := fmt.Sprintf(`DROP TABLE IF EXISTS %s CASCADE`, table)
		if _, err := m.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("plugin loader: drop %s: %w", table, err)
		}
		m.log.Info("plugin table dropped", "plugin", meta.ID, "table", table)
	}
	if err := m.repo.DeleteApplied(ctx, meta.ID); err != nil {
		return err
	}
	return nil
}

// runStep executes either the SQL or the Fn in the step, within tx.
// SQL takes precedence when both are set (matches plugin.MigrationStep).
func runStep(ctx context.Context, tx *sql.Tx, step plugin.MigrationStep) error {
	if step.SQL != "" {
		_, err := tx.ExecContext(ctx, step.SQL)
		return err
	}
	if step.Fn != nil {
		return step.Fn(ctx, tx)
	}
	return ErrEmptyStep
}

// sortedMigrations returns a copy of the slice sorted by ID.
func sortedMigrations(migs []plugin.Migration) []plugin.Migration {
	out := append([]plugin.Migration(nil), migs...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// verifyChecksums confirms every previously-applied migration still
// matches its on-disk checksum.
func verifyChecksums(pluginID string, sorted []plugin.Migration, applied map[string]string) error {
	for _, mig := range sorted {
		recorded, done := applied[mig.ID]
		if !done {
			continue
		}
		if recorded != mig.Checksum() {
			return fmt.Errorf("%w: %s/%s (recorded=%s current=%s)",
				ErrChecksumMismatch, pluginID, mig.ID, recorded, mig.Checksum())
		}
	}
	return nil
}
