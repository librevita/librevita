package fle

import (
	"entgo.io/ent/schema"
)

// AnnotationName identifies the Field-Level Encryption metadata annotation in Ent schemas.
const AnnotationName = "FieldLevelEncryption"

// Annotation defines Field-Level Encryption metadata for Ent schemas and fields.
// It serves as declarative metadata for schema introspection, compliance tooling (LGPD/GDPR),
// and Ent code generation extensions.
type Annotation struct {
	Encrypted  bool   `json:"encrypted,omitempty"`
	Searchable bool   `json:"searchable,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Normalizer string `json:"normalizer,omitempty"`
}

// Name implements the ent schema.Annotation interface.
func (Annotation) Name() string {
	return AnnotationName
}

// Encrypted marks a field for transparent encryption before database persistence.
func Encrypted(domain ...string) Annotation {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return Annotation{
		Encrypted: true,
		Domain:    d,
	}
}

// Searchable marks a field to be encrypted and to have an automatic blind index computed for exact-match searches.
// If domain is omitted, it will be automatically inferred based on convention (e.g. "<entity>.<field>").
func Searchable(domain ...string) Annotation {
	return SearchableText(domain...)
}

// SearchableText configures blind indexing with standard case-insensitive text normalization (trim + lowercase).
func SearchableText(domain ...string) Annotation {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return Annotation{
		Encrypted:  true,
		Searchable: true,
		Domain:     d,
		Normalizer: "text",
	}
}

// SearchablePhone configures blind indexing with numeric-only phone normalization (strips all non-digits).
func SearchablePhone(domain ...string) Annotation {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return Annotation{
		Encrypted:  true,
		Searchable: true,
		Domain:     d,
		Normalizer: "phone",
	}
}

// SearchableEmail configures blind indexing with permissive email normalization (trim + lowercase).
func SearchableEmail(domain ...string) Annotation {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return Annotation{
		Encrypted:  true,
		Searchable: true,
		Domain:     d,
		Normalizer: "email",
	}
}

// SearchableDocument configures blind indexing with alphanumeric document normalization (strips punctuation).
func SearchableDocument(domain ...string) Annotation {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return Annotation{
		Encrypted:  true,
		Searchable: true,
		Domain:     d,
		Normalizer: "document",
	}
}

// Ensure Annotation implements schema.Annotation at compile time.
var _ schema.Annotation = (*Annotation)(nil)
