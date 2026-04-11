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

type AuthRefreshToken struct {
	ent.Schema
}

func (AuthRefreshToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_refresh_tokens"},
	}
}

func (AuthRefreshToken) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthRefreshToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("token_hash").
			NotEmpty().
			Sensitive().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.UUID("sid", uuid.UUID{}),
		field.UUID("subject_id", uuid.UUID{}),
		field.Int64("legacy_user_id"),
		field.Time("idle_expires_at"),
		field.Time("absolute_expires_at"),
		field.Time("rotated_at").
			Optional().
			Nillable(),
		field.Time("revoked_at").
			Optional().
			Nillable(),
		field.String("replaced_by_token_hash").
			Optional().
			Nillable().
			Sensitive().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (AuthRefreshToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sid"),
		index.Fields("subject_id"),
	}
}
