package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Protocol struct {
	ent.Schema
}

func (Protocol) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "protocols"},
	}
}

func (Protocol) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Protocol) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(50).
			NotEmpty().
			Unique(),
		field.String("display_name").
			MaxLen(100).
			NotEmpty(),
		field.String("platform").
			MaxLen(50).
			NotEmpty(),
		field.String("gateway_endpoint").
			MaxLen(200).
			NotEmpty(),
		field.String("icon_svg").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("theme_color").
			MaxLen(20).
			Default(""),
		field.Int("sort_order").
			Default(0),
		field.String("status").
			MaxLen(20).
			Default(domain.StatusActive),
	}
}

func (Protocol) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("groups", Group.Type),
	}
}

func (Protocol) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform"),
		index.Fields("status"),
		index.Fields("sort_order"),
	}
}
