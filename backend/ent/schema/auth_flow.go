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

type AuthFlow struct {
	ent.Schema
}

func (AuthFlow) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_flows"},
	}
}

func (AuthFlow) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthFlow) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			StorageKey("flow_id"),
		field.String("provider").
			MaxLen(64).
			NotEmpty(),
		field.String("purpose").
			MaxLen(64).
			NotEmpty(),
		field.String("issuer").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("state_hash").
			Unique().
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("code_verifier").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("nonce").
			Optional().
			Nillable().
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
