package fle

import (
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/database/fle"
)

func TestTransformSchemas(t *testing.T) {
	nameDesc := field.String("full_name").Descriptor()
	nameFld, err := load.NewField(nameDesc)
	require.NoError(t, err)
	nameFld.Annotations = map[string]any{
		fle.AnnotationName: map[string]any{
			"searchable": true,
			"normalizer": "name",
		},
	}

	emailDesc := field.String("email").Descriptor()
	emailFld, err := load.NewField(emailDesc)
	require.NoError(t, err)
	emailFld.Annotations = map[string]any{
		fle.AnnotationName: map[string]any{
			"Searchable": true,
			"Normalizer": "email",
		},
	}

	clinicIDDesc := field.String("clinic_id").Descriptor()
	clinicIDFld, err := load.NewField(clinicIDDesc)
	require.NoError(t, err)

	unannotatedDesc := field.String("description").Descriptor()
	unannotatedFld, err := load.NewField(unannotatedDesc)
	require.NoError(t, err)

	schema := &load.Schema{
		Name: "Patient",
		Fields: []*load.Field{
			clinicIDFld,
			nameFld,
			emailFld,
			unannotatedFld,
		},
	}

	err = TransformSchemas([]*load.Schema{schema})
	require.NoError(t, err)

	// Check that full_name_token_index and email_blind_index were injected
	assert.True(t, schemaHasField(schema, "full_name_token_index"))
	assert.True(t, schemaHasField(schema, "email_blind_index"))
	assert.NotEmpty(t, schema.Indexes)
	assert.Contains(t, schema.Indexes[0].Fields, "clinic_id")
	assert.Contains(t, schema.Indexes[0].Fields, "email_blind_index")

	// Running TransformSchemas again should be idempotent and not duplicate
	err = TransformSchemas([]*load.Schema{schema})
	require.NoError(t, err)

	// Test schema without clinic_id
	descNoClinic := field.String("tax_id").Descriptor()
	fldNoClinic, err := load.NewField(descNoClinic)
	require.NoError(t, err)
	fldNoClinic.Annotations = map[string]any{
		fle.AnnotationName: map[string]any{
			"searchable": true,
		},
	}
	schemaNoClinic := &load.Schema{
		Name:   "GlobalSetting",
		Fields: []*load.Field{fldNoClinic},
	}
	require.NoError(t, TransformSchemas([]*load.Schema{schemaNoClinic}))
	assert.True(t, schemaHasField(schemaNoClinic, "tax_id_blind_index"))
	assert.NotEmpty(t, schemaNoClinic.Indexes)
	assert.Equal(t, []string{"tax_id_blind_index"}, schemaNoClinic.Indexes[0].Fields)
}

func TestTemplateHelpers(t *testing.T) {
	require.NotNil(t, Template)
	assert.Equal(t, "fle_hooks", Template.Name())

	// isNameField
	fName := &gen.Field{
		Name: "name",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"normalizer": "name"},
		},
	}
	fNameCap := &gen.Field{
		Name: "name_cap",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"Normalizer": "NAME"},
		},
	}
	fOther := &gen.Field{
		Name: "other",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"normalizer": "phone"},
		},
	}
	fNoAnn := &gen.Field{Name: "plain"}

	assert.True(t, isNameField(fName))
	assert.True(t, isNameField(fNameCap))
	assert.False(t, isNameField(fOther))
	assert.False(t, isNameField(fNoAnn))

	// normalizerFunc
	assert.Equal(t, "normalize.Phone", normalizerFunc(fOther))
	assert.Equal(t, "normalize.Text", normalizerFunc(fName))
	fEmail := &gen.Field{
		Name: "email",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"normalizer": "email"},
		},
	}
	assert.Equal(t, "normalize.Email", normalizerFunc(fEmail))
	fDoc := &gen.Field{
		Name: "doc",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"Normalizer": "document"},
		},
	}
	assert.Equal(t, "normalize.Document", normalizerFunc(fDoc))
	fCustom := &gen.Field{
		Name: "custom",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"normalizer": "unknown"},
		},
	}
	assert.Equal(t, "normalize.Text", normalizerFunc(fCustom))
	assert.Equal(t, "normalize.Text", normalizerFunc(fNoAnn))

	// isSearchableField
	fSearchable := &gen.Field{
		Name: "searchable_field",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"searchable": true},
		},
	}
	fSearchableCap := &gen.Field{
		Name: "searchable_cap",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"Searchable": true},
		},
	}
	assert.True(t, isSearchableField(fSearchable))
	assert.True(t, isSearchableField(fSearchableCap))
	assert.False(t, isSearchableField(fNoAnn))

	// hasTokenIndex
	nodeType := &gen.Type{
		Name: "User",
		Fields: []*gen.Field{
			{Name: "full_name"},
			{Name: "full_name_token_index"},
		},
	}
	assert.True(t, hasTokenIndex(nodeType, &gen.Field{Name: "full_name"}))
	assert.False(t, hasTokenIndex(nodeType, &gen.Field{Name: "email"}))

	// isEncryptedField, hasEncryptedFields, encryptedFieldsOf
	fBlind := &gen.Field{Name: "email_blind_index"}
	assert.False(t, isEncryptedField(fBlind))

	fEncrypted := &gen.Field{
		Name: "ssn",
		Annotations: map[string]any{
			fle.AnnotationName: map[string]any{"encrypted": true},
		},
	}
	assert.True(t, isEncryptedField(fEncrypted))
	assert.True(t, isEncryptedField(fSearchable))

	nodeEnc := &gen.Type{
		Name: "SensitiveRecord",
		Fields: []*gen.Field{
			fPlainField(),
			fBlind,
			fEncrypted,
		},
	}
	assert.True(t, hasEncryptedFields(nodeEnc))
	encFields := encryptedFieldsOf(nodeEnc)
	require.Len(t, encFields, 1)
	assert.Equal(t, "ssn", encFields[0].Name)

	nodePlain := &gen.Type{
		Name: "PublicRecord",
		Fields: []*gen.Field{
			fPlainField(),
		},
	}
	assert.False(t, hasEncryptedFields(nodePlain))
	assert.Empty(t, encryptedFieldsOf(nodePlain))
}

func fPlainField() *gen.Field {
	return &gen.Field{Name: "plain"}
}

func TestGenerate_InvalidPath(t *testing.T) {
	err := Generate("/non/existent/schema/dir", "/tmp")
	require.Error(t, err)
}

func TestSearchableAnnotation_EdgeCases(t *testing.T) {
	desc := field.String("notes").Descriptor()
	fld, err := load.NewField(desc)
	require.NoError(t, err)

	// Annotation is not a map
	fld.Annotations = map[string]any{
		fle.AnnotationName: "invalid-type",
	}
	schema := &load.Schema{Name: "Test", Fields: []*load.Field{fld}}
	require.NoError(t, TransformSchemas([]*load.Schema{schema}))

	// Annotation is a map with false or non-bool searchable
	fld.Annotations = map[string]any{
		fle.AnnotationName: map[string]any{
			"searchable": false,
		},
	}
	require.NoError(t, TransformSchemas([]*load.Schema{schema}))
}
