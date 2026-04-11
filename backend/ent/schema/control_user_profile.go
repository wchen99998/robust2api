package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/google/uuid"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ControlUserProfile struct {
	ent.Schema
}

func (ControlUserProfile) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "control_user_profiles"},
	}
}

func (ControlUserProfile) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ControlUserProfile) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			StorageKey("subject_id"),
		field.Int64("legacy_user_id").
			Unique(),
		field.String("email").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("username").
			MaxLen(100).
			Default(""),
		field.String("notes").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (ControlUserProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email"),
	}
}
