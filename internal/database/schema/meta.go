package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Meta holds the schema definition for system key-value metadata.
type Meta struct {
	ent.Schema
}

// Annotations of the Meta schema.
func (Meta) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "meta"},
	}
}

// Fields of the Meta.
func (Meta) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			StorageKey("key").
			NotEmpty().
			Immutable().
			Comment("Metadata key"),
		field.String("value").
			NotEmpty().
			Comment("Metadata value"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}
