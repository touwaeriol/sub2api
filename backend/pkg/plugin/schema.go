package plugin

import (
	"context"
	"fmt"
	"strings"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
)

// tableNamePrefix is the required prefix for any table a plugin owns.
const tableNamePrefix = "plugin_"

// SchemaProvider produces idempotent DDL for the plugin's owned tables.
//
// Implementations typically wrap an ent-generated migrate.Schema via
// [NewEntSchemaProvider]. The host calls CreateOrUpgrade on install and on
// every process start; it is responsible for creating new tables, new columns
// and new indexes. Destructive changes (drop/rename/retype) must go through a
// [Migration] instead.
type SchemaProvider interface {
	// CreateOrUpgrade runs the idempotent "create or bring up to date"
	// step against the given driver. Implementations should pass
	// schema.WithAtlas(true) / WithGlobalUniqueID as appropriate.
	CreateOrUpgrade(ctx context.Context, drv dialect.Driver) error
}

// TableName builds a fully-qualified plugin table name from the plugin id
// and a local name. Example: TableName("alipay", "webhook_log") →
// "plugin_alipay_webhook_log".
//
// The plugin id must contain only [a-z0-9_]; the local name contributes the
// suffix after the mandatory prefix.
func TableName(pluginID, local string) string {
	return tableNamePrefix + pluginID + "_" + local
}

// AssertTableName verifies that a table name belongs to the given plugin id.
// Returns [ErrInvalidTableName] on mismatch. The host uses this during
// Meta.Tables validation to refuse tables that escape the plugin namespace.
func AssertTableName(pluginID, table string) error {
	want := tableNamePrefix + pluginID + "_"
	if !strings.HasPrefix(table, want) {
		return fmt.Errorf("%w: %q must start with %q", ErrInvalidTableName, table, want)
	}
	if len(table) == len(want) {
		return fmt.Errorf("%w: %q has empty local suffix", ErrInvalidTableName, table)
	}
	return nil
}

// entSchemaCreator is the minimal surface of ent's generated Schema (the
// field on a ent.Client, type *migrate.Schema). Ent's migrate package changes
// rarely, and exposing the method via an interface lets plugins pass their
// generated *migrate.Schema without the SDK linking each plugin's ent.
type entSchemaCreator interface {
	Create(ctx context.Context, opts ...schema.MigrateOption) error
}

// entSchemaProvider adapts an ent-generated migrate.Schema (or anything with
// the same signature) to the [SchemaProvider] interface.
type entSchemaProvider struct {
	creator entSchemaCreator
	opts    []schema.MigrateOption
}

// CreateOrUpgrade implements SchemaProvider by delegating to the wrapped
// ent Schema. The driver is ignored because the ent schema holds its own.
func (p *entSchemaProvider) CreateOrUpgrade(ctx context.Context, _ dialect.Driver) error {
	return p.creator.Create(ctx, p.opts...)
}

// NewEntSchemaProvider wraps an ent-generated migrate.Schema (obtained from
// `<plugin>/ent/migrate.NewSchema(drv)` or `client.Schema`) as a
// [SchemaProvider]. The type parameter is purely cosmetic — it lets plugin
// authors write `NewEntSchemaProvider(client.Schema)` without needing an
// explicit interface cast.
//
// The caller is responsible for constructing the ent Schema with its own
// driver; this wrapper only forwards the Create call.
func NewEntSchemaProvider[S entSchemaCreator](s S, opts ...schema.MigrateOption) SchemaProvider {
	return &entSchemaProvider{creator: s, opts: opts}
}
