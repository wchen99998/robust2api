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

type AuthPasswordResetToken struct {
	ent.Schema
}

func (AuthPasswordResetToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_password_reset_tokens"},
	}
}

func (AuthPasswordResetToken) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthPasswordResetToken) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			StorageKey("reset_id"),
		field.UUID("subject_id", uuid.UUID{}),
		field.String("email").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("token_hash").
			Unique().
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("expires_at"),
		field.Time("consumed_at").
			Optional().
			Nillable(),
	}
}

func (AuthPasswordResetToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subject_id", "consumed_at"),
	}
}
