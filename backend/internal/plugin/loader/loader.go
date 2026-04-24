package loader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"entgo.io/ent/dialect"
)

// Errors emitted by the Loader on illegal lifecycle transitions.
var (
	// ErrInvalidState is returned when a transition is attempted from a
	// state that does not permit it (for example Enable from uninstalled).
	ErrInvalidState = errors.New("plugin loader: invalid state for transition")
)

// shutdownDeadline bounds every Shutdown call so a mis-behaving plugin
// cannot wedge the loader.
const shutdownDeadline = 10 * time.Second

// CoreAPIFactory produces a permission-scoped CoreAPI for the plugin being
// initialised. Implemented by internal/plugin/api (Agent C).
type CoreAPIFactory interface {
	For(pluginID string, perms []plugin.Permission) plugin.CoreAPI
}

// SettingsPurger clears the settings namespace owned by the plugin on
// uninstall. Kept as an optional collaborator because the SettingsAPI is
// owned by Agent C and wired later. When nil the loader logs a TODO and
// skips the cleanup.
type SettingsPurger interface {
	Purge(ctx context.Context, pluginID string) error
}

// Loader orchestrates install, enable, disable and uninstall transitions
// for the plugins registered via plugin.Register.
type Loader struct {
	repo        repository.PluginRepository
	mig         *Migrator
	driver      dialect.Driver
	coreFactory CoreAPIFactory
	settings    SettingsPurger
	log         *slog.Logger
}

// NewLoader wires the collaborators.
func NewLoader(
	repo repository.PluginRepository,
	mig *Migrator,
	driver dialect.Driver,
	coreFactory CoreAPIFactory,
) *Loader {
	return &Loader{
		repo:        repo,
		mig:         mig,
		driver:      driver,
		coreFactory: coreFactory,
		log:         slog.Default(),
	}
}

// WithLogger returns a shallow copy with a custom logger.
func (l *Loader) WithLogger(log *slog.Logger) *Loader {
	if log == nil {
		return l
	}
	clone := *l
	clone.log = log
	return &clone
}

// WithSettingsPurger attaches the optional settings cleanup hook.
func (l *Loader) WithSettingsPurger(sp SettingsPurger) *Loader {
	clone := *l
	clone.settings = sp
	return &clone
}

// Install runs version/table validation, applies migrations and upserts
// the plugins row with state=installed. Idempotent on repeat calls.
func (l *Loader) Install(ctx context.Context, p plugin.Plugin) error {
	meta := p.Meta()
	if err := plugin.CheckAPIVersion(meta.APIVersion); err != nil {
		return err
	}
	if err := validateTables(meta.ID, meta.Tables); err != nil {
		return err
	}
	if err := l.mig.Apply(ctx, p); err != nil {
		return err
	}
	record := BuildPluginRecord(meta, repository.PluginStateInstalled)
	return l.repo.Upsert(ctx, &record)
}

// Enable calls the plugin Init + Start and flips the state column.
// The plugin must currently be in installed or disabled.
func (l *Loader) Enable(ctx context.Context, id string) error {
	record, err := l.repo.Find(ctx, id)
	if err != nil {
		return err
	}
	if record.State != repository.PluginStateInstalled &&
		record.State != repository.PluginStateDisabled {
		return fmt.Errorf("%w: %s state=%s", ErrInvalidState, id, record.State)
	}
	p, ok := plugin.Lookup(id)
	if !ok {
		return fmt.Errorf("%w: %s", plugin.ErrPluginNotFound, id)
	}
	core := l.coreFactory.For(id, p.Meta().Permissions)
	if err := p.Init(core); err != nil {
		return fmt.Errorf("plugin loader: init %s: %w", id, err)
	}
	if err := p.Start(ctx); err != nil {
		return fmt.Errorf("plugin loader: start %s: %w", id, err)
	}
	return l.repo.UpdateState(ctx, id, repository.PluginStateEnabled)
}

// Disable calls Shutdown (with a deadline) and flips the state column.
func (l *Loader) Disable(ctx context.Context, id string) error {
	p, ok := plugin.Lookup(id)
	if !ok {
		return fmt.Errorf("%w: %s", plugin.ErrPluginNotFound, id)
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownDeadline)
	defer cancel()
	if err := p.Shutdown(shutdownCtx); err != nil {
		l.log.Error("plugin shutdown failed", "plugin", id, "error", err)
		// Still demote the state so the plugin is not re-entered.
	}
	return l.repo.UpdateState(ctx, id, repository.PluginStateDisabled)
}

// Uninstall takes the plugin offline. When purge=true the declared tables
// are dropped and the plugins row is removed; otherwise the row is marked
// uninstalled so reinstall preserves data.
func (l *Loader) Uninstall(ctx context.Context, id string, purge bool) error {
	if err := l.safeDisable(ctx, id); err != nil {
		return err
	}
	if err := l.purgeSettings(ctx, id); err != nil {
		return err
	}
	if !purge {
		return l.repo.UpdateState(ctx, id, repository.PluginStateUninstalled)
	}
	return l.purgeTablesAndRow(ctx, id)
}

// safeDisable demotes the plugin only if currently enabled.
func (l *Loader) safeDisable(ctx context.Context, id string) error {
	record, err := l.repo.Find(ctx, id)
	if err != nil {
		return err
	}
	if record.State == repository.PluginStateEnabled {
		if err := l.Disable(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// purgeSettings clears plugin-scoped settings when a purger is wired.
func (l *Loader) purgeSettings(ctx context.Context, id string) error {
	if l.settings == nil {
		l.log.Debug("plugin settings purge skipped (no purger wired)", "plugin", id)
		return nil
	}
	if err := l.settings.Purge(ctx, id); err != nil {
		return fmt.Errorf("plugin loader: purge settings %s: %w", id, err)
	}
	return nil
}

// purgeTablesAndRow drops plugin tables, migration history and the
// plugins row.
func (l *Loader) purgeTablesAndRow(ctx context.Context, id string) error {
	p, ok := plugin.Lookup(id)
	if !ok {
		// Plugin no longer in code — fall back to just deleting the row
		// because we cannot discover its tables.
		l.log.Warn("plugin purge without code; row only", "plugin", id)
		return l.repo.Delete(ctx, id)
	}
	if err := l.mig.Drop(ctx, p); err != nil {
		return err
	}
	return l.repo.Delete(ctx, id)
}

// PluginState is the read-only projection of a plugins row exposed to
// admin handlers. It mirrors repository.PluginRecord but is declared in the
// loader package so callers do not need to import the repository package
// just to consume lifecycle state.
type PluginState struct {
	ID             string
	Version        string
	APIVersion     string
	State          string
	InstalledAt    time.Time
	LastEnabledAt  time.Time
	DeclaredTables []string
	MetaSnapshot   map[string]any
}

// ListStates returns every row in the plugins table. Used by the admin
// HTTP handlers to render the plugin list alongside the compiled-in
// registry; read-only, no transitions performed.
func (l *Loader) ListStates(ctx context.Context) ([]PluginState, error) {
	rows, err := l.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PluginState, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, toPluginState(row))
	}
	return out, nil
}

// FindState returns the state row for a single plugin, or
// repository.ErrPluginNotFound when missing.
func (l *Loader) FindState(ctx context.Context, id string) (*PluginState, error) {
	row, err := l.repo.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	state := toPluginState(row)
	return &state, nil
}

// toPluginState projects a repository record into the public PluginState
// value type used by admin handlers.
func toPluginState(row *repository.PluginRecord) PluginState {
	return PluginState{
		ID:             row.ID,
		Version:        row.Version,
		APIVersion:     row.APIVersion,
		State:          row.State,
		InstalledAt:    row.InstalledAt,
		LastEnabledAt:  row.LastEnabledAt,
		DeclaredTables: append([]string(nil), row.DeclaredTables...),
		MetaSnapshot:   row.MetaSnapshot,
	}
}

// BootstrapAll is invoked at host startup. It iterates the registered
// plugins in dependency order and replays the lifecycle implied by each
// row state.
func (l *Loader) BootstrapAll(ctx context.Context) error {
	registered := plugin.Registered()
	ordered, err := Sort(registered)
	if err != nil {
		return err
	}
	for _, p := range ordered {
		if err := l.bootstrapOne(ctx, p); err != nil {
			return fmt.Errorf("plugin loader: bootstrap %s: %w", p.Meta().ID, err)
		}
	}
	return nil
}

// bootstrapOne replays the state transition for one plugin.
func (l *Loader) bootstrapOne(ctx context.Context, p plugin.Plugin) error {
	id := p.Meta().ID
	record, err := l.repo.Find(ctx, id)
	if err != nil && !errors.Is(err, repository.ErrPluginNotFound) {
		return err
	}
	if errors.Is(err, repository.ErrPluginNotFound) {
		return l.Install(ctx, p)
	}
	switch record.State {
	case repository.PluginStateEnabled:
		if err := l.Install(ctx, p); err != nil {
			return err
		}
		return l.Enable(ctx, id)
	case repository.PluginStateDisabled, repository.PluginStateInstalled:
		return l.Install(ctx, p)
	case repository.PluginStateUninstalled:
		l.log.Info("plugin remains uninstalled, skipping", "plugin", id)
		return nil
	default:
		return fmt.Errorf("%w: %s state=%s", ErrInvalidState, id, record.State)
	}
}

// DisableAll iterates every plugin currently in state=enabled and invokes
// Disable. Used by main.go during graceful shutdown so plugin Shutdown
// hooks run before infra resources (Redis, ent client) are closed.
// Errors are logged per-plugin and do not abort the sweep.
func (l *Loader) DisableAll(ctx context.Context) {
	states, err := l.ListStates(ctx)
	if err != nil {
		l.log.Error("plugin loader: list states for shutdown failed", "error", err)
		return
	}
	for _, st := range states {
		if st.State != repository.PluginStateEnabled {
			continue
		}
		if err := l.Disable(ctx, st.ID); err != nil {
			l.log.Error("plugin loader: disable during shutdown failed",
				"plugin", st.ID, "error", err)
		}
	}
}

// validateTables ensures every declared table name obeys the
// plugin_<id>_ prefix rule.
func validateTables(pluginID string, tables []string) error {
	for _, t := range tables {
		if err := plugin.AssertTableName(pluginID, t); err != nil {
			return err
		}
	}
	return nil
}

// BuildPluginRecord projects a Meta into a repository PluginRecord with
// the requested state. Exported as a plain function so tests and the
// bootstrap helper can build records without constructing a Loader.
func BuildPluginRecord(meta plugin.Meta, state string) repository.PluginRecord {
	return repository.PluginRecord{
		ID:             meta.ID,
		Version:        meta.Version,
		APIVersion:     meta.APIVersion,
		State:          state,
		DeclaredTables: append([]string(nil), meta.Tables...),
		MetaSnapshot:   metaSnapshot(meta),
	}
}

// metaSnapshot projects a Meta into a stable JSON-friendly map. We only
// persist fields that survive a process restart (no function pointers).
func metaSnapshot(meta plugin.Meta) map[string]any {
	return map[string]any{
		"id":           meta.ID,
		"name":         meta.Name,
		"description":  meta.Description,
		"version":      meta.Version,
		"api_version":  meta.APIVersion,
		"permissions":  meta.Permissions,
		"tables":       meta.Tables,
		"dependencies": depsSnapshot(meta.Dependencies),
	}
}

// depsSnapshot renders Dep slices in the meta_snapshot JSON form.
func depsSnapshot(deps []plugin.Dep) []map[string]any {
	out := make([]map[string]any, 0, len(deps))
	for _, d := range deps {
		out = append(out, map[string]any{
			"id":            d.ID,
			"version_range": d.VersionRange,
			"optional":      d.Optional,
		})
	}
	return out
}
