// Package repository contains persistence adapters backing the plugin
// subsystem: plugin_repo for the plugins table (managed via ent) and
// migration_repo for plugin_migrations (raw SQL because its composite
// string primary key is not expressible through the project's current
// ent codegen settings).
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	entplugin "github.com/Wei-Shaw/sub2api/ent/plugin"
)

// PluginRecord is the repository-level projection of a plugins row. It
// mirrors the ent entity but keeps the repository interface decoupled from
// the generated struct, so consumers (loader, admin handlers) never import
// ent types directly.
type PluginRecord struct {
	ID             string
	Version        string
	APIVersion     string
	State          string
	InstalledAt    time.Time
	LastEnabledAt  time.Time
	DeclaredTables []string
	MetaSnapshot   map[string]any
}

// Valid lifecycle states persisted in the plugins.state column.
// Mirrored from the ent enum so consumers do not need to import entplugin.
const (
	PluginStateInstalled   = "installed"
	PluginStateDisabled    = "disabled"
	PluginStateEnabled     = "enabled"
	PluginStateUninstalled = "uninstalled"
)

// ErrPluginNotFound is returned by Find / UpdateState / Delete when the
// plugin id is not present in the table.
var ErrPluginNotFound = errors.New("plugin repository: plugin not found")

// PluginRepository is the persistence contract for the plugins table.
// Implementations are expected to be safe for concurrent use.
type PluginRepository interface {
	// Upsert creates or updates the plugin row. The state field is taken
	// verbatim from the record so callers stay in control of the
	// lifecycle transition being persisted.
	Upsert(ctx context.Context, record *PluginRecord) error
	// Find returns the current record or ErrPluginNotFound.
	Find(ctx context.Context, id string) (*PluginRecord, error)
	// List returns every registered plugin, ordered by id for deterministic
	// iteration (used at boot time).
	List(ctx context.Context) ([]*PluginRecord, error)
	// UpdateState flips the state column; when transitioning to enabled
	// last_enabled_at is refreshed to now.
	UpdateState(ctx context.Context, id string, state string) error
	// Delete removes the row (hard delete — see schema note).
	Delete(ctx context.Context, id string) error
}

// entPluginRepository is the ent-backed implementation.
type entPluginRepository struct {
	client *ent.Client
}

// NewPluginRepository wires the repository to the ent client.
func NewPluginRepository(client *ent.Client) PluginRepository {
	return &entPluginRepository{client: client}
}

// Upsert implements PluginRepository.
//
// NOTE: We intentionally avoid ent's OnConflict upsert path here. The
// generated PluginUpsert.Set bypasses the field.TypeJSON encoder for
// declared_tables and forwards the raw []string to the pq driver, which
// fails with "unsupported type []string". Find + Create/UpdateOne both
// route through _spec.SetField(..., field.TypeJSON, ...) so the JSON
// marshal happens before binding. See plugin_create.go for the divergence.
func (r *entPluginRepository) Upsert(ctx context.Context, record *PluginRecord) error {
	if record == nil || record.ID == "" {
		return fmt.Errorf("plugin repository: upsert requires non-empty record id")
	}
	state, err := parsePluginState(record.State)
	if err != nil {
		return err
	}
	tables := record.DeclaredTables
	if tables == nil {
		tables = []string{}
	}

	existing, err := r.client.Plugin.Query().Where(entplugin.IDEQ(record.ID)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("plugin repository: upsert lookup %q: %w", record.ID, err)
	}
	if ent.IsNotFound(err) {
		return r.createRecord(ctx, record, state, tables)
	}
	return r.updateRecord(ctx, existing, record, state, tables)
}

// createRecord inserts a new plugins row.
func (r *entPluginRepository) createRecord(
	ctx context.Context,
	record *PluginRecord,
	state entplugin.State,
	tables []string,
) error {
	installedAt := record.InstalledAt
	if installedAt.IsZero() {
		installedAt = time.Now()
	}
	create := r.client.Plugin.Create().
		SetID(record.ID).
		SetVersion(record.Version).
		SetAPIVersion(record.APIVersion).
		SetState(state).
		SetInstalledAt(installedAt).
		SetDeclaredTables(tables).
		SetMetaSnapshot(record.MetaSnapshot)
	if !record.LastEnabledAt.IsZero() {
		create = create.SetLastEnabledAt(record.LastEnabledAt)
	}
	if err := create.Exec(ctx); err != nil {
		return fmt.Errorf("plugin repository: upsert create %q: %w", record.ID, err)
	}
	return nil
}

// updateRecord refreshes a pre-existing plugins row (installed_at stays
// immutable by virtue of the schema annotation).
func (r *entPluginRepository) updateRecord(
	ctx context.Context,
	existing *ent.Plugin,
	record *PluginRecord,
	state entplugin.State,
	tables []string,
) error {
	update := existing.Update().
		SetVersion(record.Version).
		SetAPIVersion(record.APIVersion).
		SetState(state).
		SetDeclaredTables(tables).
		SetMetaSnapshot(record.MetaSnapshot)
	if !record.LastEnabledAt.IsZero() {
		update = update.SetLastEnabledAt(record.LastEnabledAt)
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("plugin repository: upsert update %q: %w", record.ID, err)
	}
	return nil
}

// Find implements PluginRepository.
func (r *entPluginRepository) Find(ctx context.Context, id string) (*PluginRecord, error) {
	row, err := r.client.Plugin.Query().Where(entplugin.IDEQ(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPluginNotFound
		}
		return nil, fmt.Errorf("plugin repository: find %q: %w", id, err)
	}
	return toPluginRecord(row), nil
}

// List implements PluginRepository.
func (r *entPluginRepository) List(ctx context.Context) ([]*PluginRecord, error) {
	rows, err := r.client.Plugin.Query().Order(ent.Asc(entplugin.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugin repository: list: %w", err)
	}
	out := make([]*PluginRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPluginRecord(row))
	}
	return out, nil
}

// UpdateState implements PluginRepository.
func (r *entPluginRepository) UpdateState(ctx context.Context, id string, state string) error {
	parsed, err := parsePluginState(state)
	if err != nil {
		return err
	}
	update := r.client.Plugin.UpdateOneID(id).SetState(parsed)
	if parsed == entplugin.StateEnabled {
		update = update.SetLastEnabledAt(time.Now())
	}
	if err := update.Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
		}
		return fmt.Errorf("plugin repository: update state %q: %w", id, err)
	}
	return nil
}

// Delete implements PluginRepository.
func (r *entPluginRepository) Delete(ctx context.Context, id string) error {
	if err := r.client.Plugin.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
		}
		return fmt.Errorf("plugin repository: delete %q: %w", id, err)
	}
	return nil
}

// parsePluginState narrows a repository-level string to the ent enum; an
// empty string defaults to "installed" to simplify first-time inserts.
func parsePluginState(state string) (entplugin.State, error) {
	if state == "" {
		return entplugin.StateInstalled, nil
	}
	s := entplugin.State(state)
	if err := entplugin.StateValidator(s); err != nil {
		return "", fmt.Errorf("plugin repository: invalid state %q: %w", state, err)
	}
	return s, nil
}

// toPluginRecord converts ent.Plugin to the repository-level record.
func toPluginRecord(row *ent.Plugin) *PluginRecord {
	rec := &PluginRecord{
		ID:             row.ID,
		Version:        row.Version,
		APIVersion:     row.APIVersion,
		State:          string(row.State),
		InstalledAt:    row.InstalledAt,
		DeclaredTables: append([]string(nil), row.DeclaredTables...),
		MetaSnapshot:   row.MetaSnapshot,
	}
	if row.LastEnabledAt != nil {
		rec.LastEnabledAt = *row.LastEnabledAt
	}
	return rec
}
