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

// EventDeadLetter holds the schema definition for the EventDeadLetter entity.
//
// Mirrors the `event_dead_letters` table created by migration 103. Rows are
// inserted by the async event dispatcher when a subscriber still fails after
// the retry budget is exhausted. Admin tooling lists, retries and deletes
// entries.
//
// 删除策略：硬删除（手动重试成功或管理员清理时直接删除）。
//
// Run `go generate ./ent` after editing this file.
type EventDeadLetter struct {
	ent.Schema
}

// Annotations binds this schema to the `event_dead_letters` table.
func (EventDeadLetter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "event_dead_letters"},
	}
}

// Fields of the EventDeadLetter entity.
func (EventDeadLetter) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			StructTag(`json:"id"`).
			Immutable(),
		field.String("topic").
			MaxLen(256).
			NotEmpty(),
		field.JSON("payload", []byte{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("first_failed_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_attempt_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int("attempt_count").
			Default(1).
			NonNegative(),
		field.Text("last_error").
			Optional().
			Nillable(),
		field.String("subscriber_tag").
			MaxLen(256).
			Optional().
			Nillable(),
		field.String("correlation_id").
			MaxLen(128).
			Optional().
			Nillable(),
	}
}

// Indexes of the EventDeadLetter entity.
func (EventDeadLetter) Indexes() []ent.Index {
	return []ent.Index{
		// Mirrors the SQL index for topic-scoped admin listings.
		index.Fields("topic", "first_failed_at"),
	}
}
