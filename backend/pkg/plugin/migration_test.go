//go:build unit

package plugin_test

import (
	"testing"
	"testing/fstest"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"

	"github.com/stretchr/testify/require"
)

func TestMigrationsFromFS_PairedUpDown(t *testing.T) {
	fsys := fstest.MapFS{
		"m/20260101120000_init.up.sql": &fstest.MapFile{
			Data: []byte("-- create foo\nCREATE TABLE foo(id int)"),
		},
		"m/20260101120000_init.down.sql": &fstest.MapFile{
			Data: []byte("DROP TABLE foo"),
		},
	}

	migs, err := plugin.MigrationsFromFS(fsys, "m")
	require.NoError(t, err)
	require.Len(t, migs, 1)
	require.Equal(t, "20260101120000_init", migs[0].ID)
	require.Equal(t, "create foo", migs[0].Description)
	require.Equal(t, "-- create foo\nCREATE TABLE foo(id int)", migs[0].Up.SQL)
	require.Equal(t, "DROP TABLE foo", migs[0].Down.SQL)
	require.False(t, migs[0].Up.IsEmpty())
	require.False(t, migs[0].Down.IsEmpty())
}

func TestMigrationsFromFS_UpOnly(t *testing.T) {
	fsys := fstest.MapFS{
		"m/20260101120000_init.up.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE foo(id int)"),
		},
	}

	migs, err := plugin.MigrationsFromFS(fsys, "m")
	require.NoError(t, err)
	require.Len(t, migs, 1)
	require.Equal(t, "CREATE TABLE foo(id int)", migs[0].Up.SQL)
	require.True(t, migs[0].Down.IsEmpty(), "Down step should be empty when no .down.sql exists")
}

func TestMigrationsFromFS_BackwardCompatPlainSQL(t *testing.T) {
	// Legacy single-file form (no .up.sql / .down.sql suffix) still loads
	// as Up only.
	fsys := fstest.MapFS{
		"m/20260101120000_init.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE legacy(id int)"),
		},
	}

	migs, err := plugin.MigrationsFromFS(fsys, "m")
	require.NoError(t, err)
	require.Len(t, migs, 1)
	require.Equal(t, "20260101120000_init", migs[0].ID)
	require.Equal(t, "CREATE TABLE legacy(id int)", migs[0].Up.SQL)
	require.True(t, migs[0].Down.IsEmpty())
}

func TestMigrationsFromFS_DownOnlyIsSkipped(t *testing.T) {
	// A .down.sql without its Up pair has no forward step the host could
	// apply. MigrationsFromFS drops it so callers don't see a phantom
	// migration.
	fsys := fstest.MapFS{
		"m/20260101120000_init.down.sql": &fstest.MapFile{
			Data: []byte("DROP TABLE foo"),
		},
	}

	migs, err := plugin.MigrationsFromFS(fsys, "m")
	require.NoError(t, err)
	require.Empty(t, migs)
}

func TestMigrationsFromFS_SortedByID(t *testing.T) {
	fsys := fstest.MapFS{
		"m/20260102000000_b.up.sql": &fstest.MapFile{Data: []byte("-- b")},
		"m/20260101000000_a.up.sql": &fstest.MapFile{Data: []byte("-- a")},
	}

	migs, err := plugin.MigrationsFromFS(fsys, "m")
	require.NoError(t, err)
	require.Len(t, migs, 2)
	require.Equal(t, "20260101000000_a", migs[0].ID)
	require.Equal(t, "20260102000000_b", migs[1].ID)
}

func TestMigration_ChecksumIgnoresDown(t *testing.T) {
	// Down changes must not alter the checksum — otherwise adding a Down
	// to a historical migration would trigger ErrChecksumMismatch on the
	// next boot.
	base := plugin.Migration{
		ID: "20260101000000_init",
		Up: plugin.MigrationStep{SQL: "CREATE TABLE x(id int)"},
	}
	withDown := base
	withDown.Down = plugin.MigrationStep{SQL: "DROP TABLE x"}

	require.Equal(t, base.Checksum(), withDown.Checksum())
}

func TestMigration_UpStepFallsBackToDeprecatedStep(t *testing.T) {
	mig := plugin.Migration{
		ID:   "legacy",
		Step: plugin.MigrationStep{SQL: "CREATE TABLE legacy(id int)"},
	}
	require.Equal(t, "CREATE TABLE legacy(id int)", mig.UpStep().SQL)
}
