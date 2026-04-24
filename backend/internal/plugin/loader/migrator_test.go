//go:build unit

package loader

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/plugin/repository"
	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// memMigrationRepo is an in-memory MigrationRepository used to isolate
// the Migrator from database/sql details.
type memMigrationRepo struct {
	// applied maps pluginID -> migrationID -> checksum.
	applied map[string]map[string]string
	// recordErr/deleteErr inject failures.
	recordErr error
	deleteErr error
}

func newMemRepo() *memMigrationRepo {
	return &memMigrationRepo{applied: map[string]map[string]string{}}
}

func (m *memMigrationRepo) ListApplied(_ context.Context, pluginID string) (map[string]string, error) {
	entries, ok := m.applied[pluginID]
	if !ok {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(entries))
	for k, v := range entries {
		out[k] = v
	}
	return out, nil
}

func (m *memMigrationRepo) RecordApplied(_ context.Context, _ *sql.Tx, pluginID, migrationID, checksum string) error {
	if m.recordErr != nil {
		return m.recordErr
	}
	if _, ok := m.applied[pluginID]; !ok {
		m.applied[pluginID] = map[string]string{}
	}
	m.applied[pluginID][migrationID] = checksum
	return nil
}

func (m *memMigrationRepo) DeleteApplied(_ context.Context, pluginID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.applied, pluginID)
	return nil
}

func (m *memMigrationRepo) DeleteByIDs(_ context.Context, pluginID string, ids []string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if entries, ok := m.applied[pluginID]; ok {
		for _, id := range ids {
			delete(entries, id)
		}
	}
	return nil
}

// migrationStubPlugin is a minimal plugin.Plugin used to feed the migrator.
type migrationStubPlugin struct{ meta plugin.Meta }

func (s *migrationStubPlugin) Meta() plugin.Meta                { return s.meta }
func (s *migrationStubPlugin) Init(_ plugin.CoreAPI) error      { return nil }
func (s *migrationStubPlugin) Start(_ context.Context) error    { return nil }
func (s *migrationStubPlugin) Shutdown(_ context.Context) error { return nil }

// buildPlugin wires a stub plugin with the given migrations.
func buildPlugin(id string, migs ...plugin.Migration) plugin.Plugin {
	return &migrationStubPlugin{meta: plugin.Meta{
		ID:         id,
		Version:    "1.0.0",
		APIVersion: plugin.SDKVersion,
		Migrations: migs,
	}}
}

func TestMigrator_ApplyNewMigrations(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mig := plugin.Migration{
		ID: "20260101000000_init",
		Up: plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
	}
	p := buildPlugin("x", mig)

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE plugin_x_foo(id int)").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	repo := newMemRepo()
	m := NewMigrator(repo, db, nil, slog.Default())
	require.NoError(t, m.Apply(context.Background(), p))

	require.Equal(t, map[string]string{mig.ID: mig.Checksum()}, repo.applied["x"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_ChecksumMismatch(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mig := plugin.Migration{
		ID: "20260101000000_init",
		Up: plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
	}
	p := buildPlugin("x", mig)

	repo := newMemRepo()
	repo.applied["x"] = map[string]string{mig.ID: "stale-checksum"}

	m := NewMigrator(repo, db, nil, slog.Default())
	err = m.Apply(context.Background(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrChecksumMismatch)
}

func TestMigrator_SkipsAlreadyApplied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mig := plugin.Migration{
		ID: "20260101000000_init",
		Up: plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
	}
	p := buildPlugin("x", mig)

	repo := newMemRepo()
	repo.applied["x"] = map[string]string{mig.ID: mig.Checksum()}

	m := NewMigrator(repo, db, nil, slog.Default())
	require.NoError(t, m.Apply(context.Background(), p))
	// No Begin/Commit expected.
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_FnStep(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	called := false
	mig := plugin.Migration{
		ID: "20260101000000_fn",
		Up: plugin.MigrationStep{
			Fn: func(_ context.Context, tx *sql.Tx) error {
				called = true
				return nil
			},
		},
	}
	p := buildPlugin("x", mig)

	mock.ExpectBegin()
	mock.ExpectCommit()

	repo := newMemRepo()
	m := NewMigrator(repo, db, nil, slog.Default())
	require.NoError(t, m.Apply(context.Background(), p))
	require.True(t, called)
	require.Equal(t, map[string]string{mig.ID: mig.Checksum()}, repo.applied["x"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_EmptyStepErrors(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	bad := plugin.Migration{ID: "20260101000000_bad"}
	p := buildPlugin("x", bad)

	m := NewMigrator(newMemRepo(), db, nil, slog.Default())
	err = m.Apply(context.Background(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEmptyStep)
}

func TestMigrator_RollbackNotReversible(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mig := plugin.Migration{
		ID: "20260101000000_init",
		Up: plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
	}
	p := buildPlugin("x", mig)

	repo := newMemRepo()
	repo.applied["x"] = map[string]string{mig.ID: mig.Checksum()}

	m := NewMigrator(repo, db, nil, slog.Default())
	err = m.Rollback(context.Background(), p, "")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMigrationNotReversible)
}

func TestMigrator_RollbackAll(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	first := plugin.Migration{
		ID:   "20260101000000_init",
		Up:   plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
		Down: plugin.MigrationStep{SQL: "DROP TABLE plugin_x_foo"},
	}
	second := plugin.Migration{
		ID:   "20260102000000_add_col",
		Up:   plugin.MigrationStep{SQL: "ALTER TABLE plugin_x_foo ADD COLUMN name TEXT"},
		Down: plugin.MigrationStep{SQL: "ALTER TABLE plugin_x_foo DROP COLUMN name"},
	}
	p := buildPlugin("x", first, second)

	repo := newMemRepo()
	repo.applied["x"] = map[string]string{
		first.ID:  first.Checksum(),
		second.ID: second.Checksum(),
	}

	// Newest migration rolls back first.
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE plugin_x_foo DROP COLUMN name").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("DROP TABLE plugin_x_foo").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	m := NewMigrator(repo, db, nil, slog.Default())
	require.NoError(t, m.Rollback(context.Background(), p, ""))
	require.Empty(t, repo.applied["x"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_RollbackPartial(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	first := plugin.Migration{
		ID:   "20260101000000_init",
		Up:   plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
		Down: plugin.MigrationStep{SQL: "DROP TABLE plugin_x_foo"},
	}
	second := plugin.Migration{
		ID:   "20260102000000_add_col",
		Up:   plugin.MigrationStep{SQL: "ALTER TABLE plugin_x_foo ADD COLUMN name TEXT"},
		Down: plugin.MigrationStep{SQL: "ALTER TABLE plugin_x_foo DROP COLUMN name"},
	}
	p := buildPlugin("x", first, second)

	repo := newMemRepo()
	repo.applied["x"] = map[string]string{
		first.ID:  first.Checksum(),
		second.ID: second.Checksum(),
	}

	// Rolling back "to" first.ID undoes every migration whose ID > first.ID
	// (i.e. only second) — first stays applied.
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE plugin_x_foo DROP COLUMN name").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	m := NewMigrator(repo, db, nil, slog.Default())
	require.NoError(t, m.Rollback(context.Background(), p, first.ID))
	require.Equal(t, map[string]string{first.ID: first.Checksum()}, repo.applied["x"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_RollbackMixedReversibilityFailsFast(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// first lacks a Down — even though second has one, Rollback must
	// refuse before executing anything so the DB never enters a half-
	// rolled-back state.
	first := plugin.Migration{
		ID: "20260101000000_init",
		Up: plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
	}
	second := plugin.Migration{
		ID:   "20260102000000_add_col",
		Up:   plugin.MigrationStep{SQL: "ALTER TABLE plugin_x_foo ADD COLUMN name TEXT"},
		Down: plugin.MigrationStep{SQL: "ALTER TABLE plugin_x_foo DROP COLUMN name"},
	}
	p := buildPlugin("x", first, second)

	repo := newMemRepo()
	repo.applied["x"] = map[string]string{
		first.ID:  first.Checksum(),
		second.ID: second.Checksum(),
	}

	m := NewMigrator(repo, db, nil, slog.Default())
	err = m.Rollback(context.Background(), p, "")
	require.ErrorIs(t, err, ErrMigrationNotReversible)
	// Nothing should have been executed; the DB still thinks both are applied.
	require.Equal(t, map[string]string{
		first.ID:  first.Checksum(),
		second.ID: second.Checksum(),
	}, repo.applied["x"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_DeprecatedStepStillWorks(t *testing.T) {
	// Backward-compat guard: the deprecated Migration.Step field must
	// still feed Apply via UpStep() until downstream plugins migrate.
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mig := plugin.Migration{
		ID:   "20260101000000_legacy",
		Step: plugin.MigrationStep{SQL: "CREATE TABLE legacy(id int)"},
	}
	p := buildPlugin("x", mig)

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE legacy(id int)").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	repo := newMemRepo()
	m := NewMigrator(repo, db, nil, slog.Default())
	require.NoError(t, m.Apply(context.Background(), p))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_ApplyFailureRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mig := plugin.Migration{
		ID: "20260101000000_init",
		Up: plugin.MigrationStep{SQL: "CREATE TABLE plugin_x_foo(id int)"},
	}
	p := buildPlugin("x", mig)

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE plugin_x_foo(id int)").WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	repo := newMemRepo()
	m := NewMigrator(repo, db, nil, slog.Default())
	err = m.Apply(context.Background(), p)
	require.Error(t, err)
	require.Empty(t, repo.applied["x"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrator_Drop(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	p := &migrationStubPlugin{meta: plugin.Meta{
		ID:         "x",
		Version:    "1.0.0",
		APIVersion: plugin.SDKVersion,
		Tables:     []string{"plugin_x_foo", "plugin_x_bar"},
	}}

	// Drop runs in reverse order: bar first, then foo.
	mock.ExpectExec("DROP TABLE IF EXISTS plugin_x_bar CASCADE").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DROP TABLE IF EXISTS plugin_x_foo CASCADE").WillReturnResult(sqlmock.NewResult(0, 0))

	repo := newMemRepo()
	repo.applied["x"] = map[string]string{"m1": "cs"}

	m := NewMigrator(repo, db, nil, slog.Default())
	require.NoError(t, m.Drop(context.Background(), p))
	require.Empty(t, repo.applied["x"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Compile-time check that memMigrationRepo satisfies the interface.
var _ repository.MigrationRepository = (*memMigrationRepo)(nil)
