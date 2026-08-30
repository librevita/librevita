package fle

import (
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/cockroachdb/errors"
	"librevita.org/internal/core/database/fle"
)

// Generate runs the Ent code generation pipeline with automatic Field-Level Encryption blind index injection.
func Generate(schemaDir, targetDir string) error {
	driver, err := gen.NewStorage("sql")
	if err != nil {
		return errors.Wrap(err, "fle codegen: create storage")
	}

	cfg := &gen.Config{
		Storage: driver,
		Package: "librevita.org/ent",
		Target:  targetDir,
		Features: []gen.Feature{
			{Name: "sql/versioned-migration"},
			{Name: "intercept"},
		},
	}

	// 1. Load raw schemas from the schema directory
	spec, err := (&load.Config{Path: schemaDir, BuildFlags: cfg.BuildFlags}).Load()
	if err != nil {
		return errors.Wrap(err, "fle codegen: load schemas")
	}
	cfg.Schema = spec.PkgPath

	// 2. Transform schemas: inject blind indexes for all fle.Searchable() fields
	if err := TransformSchemas(spec.Schemas); err != nil {
		return err
	}

	// 3. Build Graph and Generate Ent artifacts
	graph, err := gen.NewGraph(cfg, spec.Schemas...)
	if err != nil {
		return errors.Wrap(err, "fle codegen: build graph")
	}
	if err := graph.Gen(); err != nil {
		return errors.Wrap(err, "fle codegen: generate artifacts")
	}

	return nil
}

// TransformSchemas inspects all loaded schemas and injects _blind_index columns and
// clinic-scoped database indexes for fields annotated with fle.Searchable().
func TransformSchemas(schemas []*load.Schema) error {
	for _, schema := range schemas {
		for _, f := range schema.Fields {
			if err := injectSearchableField(schema, f); err != nil {
				return err
			}
		}
	}
	return nil
}

func injectSearchableField(schema *load.Schema, f *load.Field) error {
	m, ok := searchableAnnotation(f)
	if !ok {
		return nil
	}
	if annotationString(m, "normalizer", "Normalizer") == "name" {
		return injectNameTokens(schema, f)
	}
	return injectBlindIndex(schema, f)
}

func searchableAnnotation(f *load.Field) (map[string]any, bool) {
	ann, ok := f.Annotations[fle.AnnotationName]
	if !ok {
		return nil, false
	}
	m, ok := ann.(map[string]any)
	if !ok {
		return nil, false
	}
	if annotationBool(m, "searchable", "Searchable") {
		return m, true
	}
	return nil, false
}

func annotationBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if s, ok := m[k].(bool); ok && s {
			return true
		}
	}
	return false
}

func annotationString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if n, ok := m[k].(string); ok {
			return n
		}
	}
	return ""
}

func schemaHasField(schema *load.Schema, name string) bool {
	for _, existing := range schema.Fields {
		if existing.Name == name {
			return true
		}
	}
	return false
}

func injectNameTokens(schema *load.Schema, f *load.Field) error {
	tokenFieldName := f.Name + "_token_index"
	if schemaHasField(schema, tokenFieldName) {
		return nil
	}
	tokensDesc := field.JSON(tokenFieldName, []string{}).
		Optional().
		Comment("Blind index token hashes of n-grams for fast tokenized search on " + f.Name).
		Descriptor()
	loadTokensFld, err := load.NewField(tokensDesc)
	if err != nil {
		return errors.Wrapf(err, "fle codegen: create %s field", tokenFieldName)
	}
	loadTokensFld.Position = &load.Position{}
	schema.Fields = append(schema.Fields, loadTokensFld)
	return nil
}

func injectBlindIndex(schema *load.Schema, f *load.Field) error {
	blindFieldName := f.Name + "_blind_index"
	if schemaHasField(schema, blindFieldName) {
		return nil
	}
	desc := field.String(blindFieldName).
		Optional().
		Comment("Keyed hash index for exact search on " + f.Name).
		Descriptor()
	loadFld, err := load.NewField(desc)
	if err != nil {
		return errors.Wrapf(err, "fle codegen: create blind index field %s", blindFieldName)
	}
	loadFld.Position = &load.Position{}
	schema.Fields = append(schema.Fields, loadFld)
	schema.Indexes = append(schema.Indexes, &load.Index{
		Fields: blindIndexFields(schema, blindFieldName),
	})
	return nil
}

func blindIndexFields(schema *load.Schema, blindFieldName string) []string {
	if schemaHasField(schema, "clinic_id") {
		return []string{"clinic_id", blindFieldName}
	}
	return []string{blindFieldName}
}
