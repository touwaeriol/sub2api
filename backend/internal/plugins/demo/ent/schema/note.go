// Package schema defines the ent schemas owned by the demo plugin.
// Table name is prefixed with `plugin_demo_` per the plugin SDK
// contract (see pkg/plugin.AssertTableName).
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Note is an audit note the demo plugin writes when it observes a new
// account being created. It exists to exercise every piece of the plugin
// SDK (schema provider, event subscription, HTTP handler, exports).
type Note struct {
	ent.Schema
}

// Annotations binds the schema to the `plugin_demo_notes` table.
func (Note) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "plugin_demo_notes"},
	}
}

// Fields of the Note entity.
func (Note) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.String("content").
			Default(""),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}
