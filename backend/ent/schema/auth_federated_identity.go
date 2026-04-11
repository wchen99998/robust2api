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

type AuthFederatedIdentity struct {
	ent.Schema
}

func (AuthFederatedIdentity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "auth_federated_identities"},
	}
}

func (AuthFederatedIdentity) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AuthFederatedIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.UUID("subject_id", uuid.UUID{}),
		field.String("provider").
			MaxLen(64).
			NotEmpty(),
		field.String("issuer").
			NotEmpty().
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.String("external_subject").
			NotEmpty().
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.String("email").
			Default("").
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.String("username").
			Default("").
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
	}
}

func (AuthFederatedIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "issuer", "external_subject").Unique(),
		index.Fields("subject_id"),
	}
}
