package zk

import (
	"fmt"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
)

// Generate runs the Ent code generation pipeline with automatic ZeroKnowledge blind index injection.
func Generate(schemaDir, targetDir string) error {
	driver, err := gen.NewStorage("sql")
	if err != nil {
		return fmt.Errorf("zk codegen: create storage: %w", err)
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
		return fmt.Errorf("zk codegen: load schemas: %w", err)
	}
	cfg.Schema = spec.PkgPath

	// 2. Transform schemas: inject blind indexes for all zk.Searchable() fields
	if err := TransformSchemas(spec.Schemas); err != nil {
		return err
	}

	// 3. Build Graph and Generate Ent artifacts
	graph, err := gen.NewGraph(cfg, spec.Schemas...)
	if err != nil {
		return fmt.Errorf("zk codegen: build graph: %w", err)
	}
	if err := graph.Gen(); err != nil {
		return fmt.Errorf("zk codegen: generate artifacts: %w", err)
	}

	return nil
}

// TransformSchemas inspects all loaded schemas and injects _blind_index columns and
// clinic-scoped database indexes for fields annotated with zk.Searchable().
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
				return fmt.Errorf("zk codegen: create blind index field %s: %w", blindFieldName, err)
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
