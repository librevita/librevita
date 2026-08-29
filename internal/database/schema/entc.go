//go:build ignore

package main

import (
	"log"
	"os"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"github.com/cockroachdb/errors"
	"librevita.org/internal/database/codegen/check"
	"librevita.org/internal/database/codegen/fle"
)

func main() {
	schemaDir := "."
	targetDir := "../../../ent"
	if _, err := os.Stat("internal/database/schema"); err == nil {
		schemaDir = "./internal/database/schema"
		targetDir = "./ent"
	}

	if err := runCodegen(schemaDir, targetDir); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}

func runCodegen(schemaDir, targetDir string) error {
	driver, err := gen.NewStorage("sql")
	if err != nil {
		return errors.Wrap(err, "codegen: create storage")
	}

	cfg := &gen.Config{
		Storage: driver,
		Package: "librevita.org/ent",
		Target:  targetDir,
		Templates: []*gen.Template{
			fle.Template,
		},
		Features: []gen.Feature{
			{Name: "sql/versioned-migration"},
			{Name: "intercept"},
			{Name: "privacy"},
		},
	}

	// 1. Load raw schemas from the schema directory
	spec, err := (&load.Config{Path: schemaDir, BuildFlags: cfg.BuildFlags}).Load()
	if err != nil {
		return errors.Wrap(err, "codegen: load schemas")
	}
	cfg.Schema = spec.PkgPath

	// 2. Transform schemas: inject blind indexes for fle.Searchable() fields
	if err := fle.TransformSchemas(spec.Schemas); err != nil {
		return errors.Wrap(err, "codegen fle")
	}

	// 3. Transform schemas: inject database-level CHECK constraints for Enums
	if err := check.InjectEnumChecks(spec.Schemas); err != nil {
		return errors.Wrap(err, "codegen enum checks")
	}

	// 4. Build Graph and Generate Ent artifacts
	graph, err := gen.NewGraph(cfg, spec.Schemas...)
	if err != nil {
		return errors.Wrap(err, "codegen: build graph")
	}
	if err := graph.Gen(); err != nil {
		return errors.Wrap(err, "codegen: generate artifacts")
	}

	return nil
}
