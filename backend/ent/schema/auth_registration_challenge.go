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

type AuthRegistrationChallenge struct {
	ent.Schema
}

func (AuthRegistrationChallenge) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_registration_challenges"},
	}
}

func (AuthRegistrationChallenge) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthRegistrationChallenge) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			StorageKey("challenge_id"),
		field.String("provider").
			MaxLen(64).
			NotEmpty(),
		field.String("issuer").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("external_subject").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("email").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("registration_email").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("username").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("redirect_to").
			Default("/").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("expires_at"),
		field.Time("consumed_at").
			Optional().
			Nillable(),
	}
}

func (AuthRegistrationChallenge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expires_at", "consumed_at"),
	}
}
