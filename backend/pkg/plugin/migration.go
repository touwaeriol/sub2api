package plugin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Migration is one explicit schema or data change applied exactly once for a
// plugin, tracked in the `plugin_migrations` table. Use it for DDL that a
// SchemaProvider cannot express idempotently (column drops, type changes) or
// for any data transform.
//
// The host runs migrations after [SchemaProvider.CreateOrUpgrade] and before
// the plugin is made available to consumers.
type Migration struct {
	// ID identifies the migration; stored in plugin_migrations.migration_id.
	// Convention: "YYYYMMDDHHMMSS_snake_case_description".
	ID string
	// Description is a human-readable summary.
	Description string
	// Up is the forward migration step. Exactly one of Up.SQL or Up.Fn must
	// be non-zero.
	Up MigrationStep
	// Down is the reverse step applied by Migrator.Rollback. Leave zero-
	// valued (IsEmpty()==true) when the migration is irreversible — the
	// Migrator will surface ErrMigrationNotReversible in that case.
	Down MigrationStep

	// Step is retained temporarily as an alias of Up so existing callers
	// (tests, early plugins) keep compiling. Prefer Up for new code.
	//
	// Deprecated: use Up.
	Step MigrationStep
}

// MigrationStep is the payload of a [Migration]. Either write plain SQL or
// provide a Go function that executes inside the migration transaction.
type MigrationStep struct {
	// SQL is a single statement (or a sequence separated by ";") that the
	// runner executes inside a transaction. Takes precedence over Fn when
	// non-empty.
	SQL string
	// Fn is a programmatic step receiving the open tx. Use for data
	// backfills that cannot be expressed cleanly in SQL, e.g. encryption
	// migrations. The runner commits on nil error, rolls back on error.
	Fn func(ctx context.Context, tx *sql.Tx) error
}

// IsEmpty reports whether the step carries no work. Used by the Migrator to
// decide whether a Down step is provided.
func (s MigrationStep) IsEmpty() bool {
	return s.SQL == "" && s.Fn == nil
}

// UpStep returns the forward step, preferring Up and falling back to the
// deprecated Step alias. Consumers should use this helper instead of reading
// Up directly while Step is still in use.
func (m Migration) UpStep() MigrationStep {
	if !m.Up.IsEmpty() {
		return m.Up
	}
	return m.Step
}

// DownStep returns the reverse step if present.
func (m Migration) DownStep() MigrationStep {
	return m.Down
}

// Checksum computes a stable sha256 fingerprint of the forward migration
// step. The runner records it in plugin_migrations.checksum and refuses to
// re-apply a migration whose current checksum differs — that catches edits
// to an already applied migration.
//
// Only the Up step contributes to the checksum: Down-only changes do not
// invalidate history. Fn-based migrations hash "fn:<ID>" since Go functions
// have no portable representation; tampering protection for Fn migrations
// falls back to the plugin's build version.
func (m Migration) Checksum() string {
	up := m.UpStep()
	h := sha256.New()
	if up.SQL != "" {
		_, _ = h.Write([]byte("sql:"))
		_, _ = h.Write([]byte(up.SQL))
	} else {
		_, _ = h.Write([]byte("fn:"))
		_, _ = h.Write([]byte(m.ID))
	}
	return hex.EncodeToString(h.Sum(nil))
}

const (
	upSuffix    = ".up.sql"
	downSuffix  = ".down.sql"
	plainSuffix = ".sql"
)

// MigrationsFromFS loads SQL migrations embedded in a fs.FS (typically an
// embed.FS from the plugin package). It supports two file layouts:
//
//  1. Paired `XXX_desc.up.sql` + optional `XXX_desc.down.sql` — the up
//     content becomes [Migration.Up], the down content becomes
//     [Migration.Down] (empty Down means irreversible).
//  2. Single `XXX_desc.sql` — back-compat, treated as Up only.
//
// Pairing keys on the ID (the filename stripped of .up.sql/.down.sql/.sql).
// Files are returned sorted by ID so a lexical scheme like
// "20260101120000_add_column" yields the desired order.
func MigrationsFromFS(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("plugin: read migration dir %q: %w", dir, err)
	}

	migs := map[string]*Migration{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), plainSuffix) {
			continue
		}
		id, kind := parseMigrationFileName(e.Name())
		if id == "" {
			continue
		}
		path := dir + "/" + e.Name()
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("plugin: read migration %q: %w", path, err)
		}
		m, ok := migs[id]
		if !ok {
			m = &Migration{ID: id}
			migs[id] = m
		}
		applyMigrationFile(m, kind, string(body))
	}

	out := make([]Migration, 0, len(migs))
	for _, m := range migs {
		if m.UpStep().IsEmpty() {
			// A down-only file has no forward counterpart the host can
			// apply; skipping matches the pre-existing "ignore non-sql"
			// behaviour rather than surfacing a half-formed migration.
			continue
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// fileKind distinguishes paired files from plain single-file migrations.
type fileKind int

const (
	fileKindUp fileKind = iota
	fileKindDown
	fileKindPlain
)

// parseMigrationFileName returns the migration ID and the kind of file.
// Returns ("", fileKindPlain) for names that don't match any supported shape.
func parseMigrationFileName(name string) (string, fileKind) {
	switch {
	case strings.HasSuffix(name, upSuffix):
		return strings.TrimSuffix(name, upSuffix), fileKindUp
	case strings.HasSuffix(name, downSuffix):
		return strings.TrimSuffix(name, downSuffix), fileKindDown
	case strings.HasSuffix(name, plainSuffix):
		return strings.TrimSuffix(name, plainSuffix), fileKindPlain
	}
	return "", fileKindPlain
}

// applyMigrationFile stores one file's body on the migration struct.
func applyMigrationFile(m *Migration, kind fileKind, body string) {
	switch kind {
	case fileKindUp, fileKindPlain:
		m.Up = MigrationStep{SQL: body}
		if m.Description == "" {
			m.Description = firstCommentLine(body)
		}
	case fileKindDown:
		m.Down = MigrationStep{SQL: body}
	}
}

// firstCommentLine returns the text of the first "-- ..." comment in a SQL
// file, trimmed, for use as a default description.
func firstCommentLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			return strings.TrimSpace(strings.TrimPrefix(trim, "--"))
		}
		if trim != "" {
			break
		}
	}
	return ""
}
