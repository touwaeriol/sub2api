package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Plugin holds the schema definition for the Plugin entity.
//
// Each row tracks one registered plugin: its version, API contract version,
// lifecycle state, declared tables (for purge-on-uninstall) and a snapshot
// of the Meta last seen at install time.
//
// 删除策略：硬删除
// 插件被卸载时相应记录直接删除；purge 模式下附带删除 declared_tables 列出的表。
//
// Run `go generate ./ent` after editing this file.
type Plugin struct {
	ent.Schema
}

// Annotations binds this schema to the `plugins` table.
func (Plugin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "plugins"},
	}
}

// Fields of the Plugin entity. The "id" field is a string primary key
// (overrides the project-wide --idtype int64 for this single table).
func (Plugin) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			MaxLen(64).
			NotEmpty().
			Immutable(),
		field.String("version").
			MaxLen(32).
			NotEmpty(),
		field.String("api_version").
			MaxLen(32).
			NotEmpty(),
		field.Enum("state").
			Values("installed", "disabled", "enabled", "uninstalled").
			Default("installed"),
		field.Time("installed_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_enabled_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("declared_tables", []string{}).
			Default([]string{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("meta_snapshot", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

// Indexes of the Plugin entity.
func (Plugin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("state"),
	}
}
