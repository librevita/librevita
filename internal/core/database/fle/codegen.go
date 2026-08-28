package fle

import (
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/cockroachdb/errors"
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
			ann, ok := f.Annotations[AnnotationName]
			if !ok {
				continue
			}
			m, ok := ann.(map[string]any)
			if !ok {
				continue
			}
			var isSearchable bool
			if s, ok := m["searchable"].(bool); ok && s {
				isSearchable = true
			} else if s, ok := m["Searchable"].(bool); ok && s {
				isSearchable = true
			}
			if !isSearchable {
				continue
			}

			var normalizer string
			if n, ok := m["normalizer"].(string); ok {
				normalizer = n
			} else if n, ok := m["Normalizer"].(string); ok {
				normalizer = n
			}

			if normalizer == "name" {
				tokenFieldName := f.Name + "_token_index"
				var hasTokens bool
				for _, existing := range schema.Fields {
					if existing.Name == tokenFieldName {
						hasTokens = true
						break
					}
				}
				if !hasTokens {
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
				}
				continue
			}

			blindFieldName := f.Name + "_blind_index"
			var exists bool
			for _, existing := range schema.Fields {
				if existing.Name == blindFieldName {
					exists = true
					break
				}
			}
			if exists {
				continue
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

			idxFields := []string{blindFieldName}
			for _, ef := range schema.Fields {
				if ef.Name == "clinic_id" {
					idxFields = []string{"clinic_id", blindFieldName}
					break
				}
			}
			schema.Indexes = append(schema.Indexes, &load.Index{
				Fields: idxFields,
			})
		}
	}
	return nil
}
