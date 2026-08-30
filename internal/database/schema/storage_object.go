package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"librevita.org/internal/database/schema/mixin"
)

// StorageObject holds the schema definition for the StorageObject metadata entity.
type StorageObject struct {
	ent.Schema
}

// Mixin of the StorageObject.
func (StorageObject) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.UUID{},
		mixin.Clinic{},
		mixin.CreatedAt{},
	}
}

// Fields of the StorageObject.
func (StorageObject) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			NotEmpty().
			Unique().
			Comment("Content-addressed or path key in storage backend"),
		field.String("domain").
			NotEmpty().
			Comment("Domain context: patient, user, etc."),
		field.String("resource_id").
			NotEmpty().
			Comment("Associated resource ID"),
		field.String("original_name").
			NotEmpty().
			Comment("Original filename"),
		field.String("content_type").
			NotEmpty().
			Comment("MIME content type"),
		field.Int64("size").
			NonNegative().
			Comment("File size in bytes"),
		field.String("etag").
			NotEmpty().
			Comment("ETag returned by the storage backend"),
		field.String("checksum").
			NotEmpty().
			Comment("Canonical BLAKE2b-256 digest of the payload"),
		field.UUID("created_by", uuid.UUID{}).
			Comment("User ID who uploaded the file"),
	}
}

// Edges of the StorageObject.
func (StorageObject) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("clinic", Clinic.Type).
			Ref("storage_objects").
			Field("clinic_id").
			Unique().
			Required(),
		edge.To("creator", User.Type).
			Field("created_by").
			Unique().
			Required(),
	}
}

// Indexes of the StorageObject.
func (StorageObject) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("clinic_id"),
		index.Fields("domain", "resource_id", "created_at"),
	}
}
