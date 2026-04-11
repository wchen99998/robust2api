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

type AuthEmailVerification struct {
	ent.Schema
}

func (AuthEmailVerification) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_email_verifications"},
	}
}

func (AuthEmailVerification) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthEmailVerification) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			StorageKey("verification_id"),
		field.UUID("subject_id", uuid.UUID{}).
			Optional().
			Nillable(),
		field.String("purpose").
			MaxLen(64).
			NotEmpty(),
		field.String("email").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("code_hash").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("expires_at"),
		field.Time("consumed_at").
			Optional().
			Nillable(),
	}
}

func (AuthEmailVerification) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email", "purpose", "consumed_at"),
	}
}
