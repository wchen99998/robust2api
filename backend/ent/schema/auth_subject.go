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

type AuthSubject struct {
	ent.Schema
}

func (AuthSubject) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_subjects"},
	}
}

func (AuthSubject) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthSubject) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			StorageKey("subject_id"),
		field.Int64("legacy_user_id").
			Unique(),
		field.String("email").
			NotEmpty().
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.String("status").
			MaxLen(32).
			Default("active"),
		field.Int64("auth_version").
			Default(1),
	}
}

func (AuthSubject) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email"),
	}
}
