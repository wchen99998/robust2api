package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/google/uuid"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type AuthPasswordCredential struct {
	ent.Schema
}

func (AuthPasswordCredential) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_password_credentials"},
	}
}

func (AuthPasswordCredential) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthPasswordCredential) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			StorageKey("subject_id"),
		field.String("password_hash").
			NotEmpty().
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.Time("changed_at"),
	}
}
