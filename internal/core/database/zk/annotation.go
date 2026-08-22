package zk

import (
	"entgo.io/ent/schema"
)

// AnnotationName identifies the ZeroKnowledge metadata annotation in Ent schemas.
const AnnotationName = "ZeroKnowledge"

// Annotation defines ZeroKnowledge metadata for Ent schemas and fields.
// It serves as declarative metadata for schema introspection, compliance tooling (LGPD/GDPR),
// and Ent code generation extensions.
type Annotation struct {
	Encrypted  bool   `json:"encrypted,omitempty"`
	Searchable bool   `json:"searchable,omitempty"`
	Domain     string `json:"domain,omitempty"`
}

// Name implements the ent schema.Annotation interface.
func (Annotation) Name() string {
	return AnnotationName
}

// Searchable marks a field to have an automatic blind index computed for exact-match searches.
// If domain is omitted, it will be automatically inferred based on convention (e.g. "<entity>.<field>").
func Searchable(domain ...string) Annotation {
	d := ""
	if len(domain) > 0 {
		d = domain[0]
	}
	return Annotation{
		Searchable: true,
		Domain:     d,
	}
}

// Ensure Annotation implements schema.Annotation at compile time.
var _ schema.Annotation = (*Annotation)(nil)
