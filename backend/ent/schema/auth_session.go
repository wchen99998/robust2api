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

type AuthSession struct {
	ent.Schema
}

func (AuthSession) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_sessions"},
	}
}

func (AuthSession) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			StorageKey("sid"),
		field.UUID("subject_id", uuid.UUID{}),
		field.Int64("legacy_user_id"),
		field.String("status").
			MaxLen(32).
			Default("active"),
		field.String("amr").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("last_seen_at"),
		field.Time("expires_at"),
		field.Time("absolute_expires_at"),
		field.Time("revoked_at").
			Optional().
			Nillable(),
		field.String("current_refresh_token_hash").
			NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("auth_version"),
	}
}

func (AuthSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subject_id", "status"),
		index.Fields("legacy_user_id"),
	}
}
