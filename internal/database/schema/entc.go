//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"librevita.org/internal/core/database/check"
	"librevita.org/internal/core/database/fle"
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
		return fmt.Errorf("codegen: create storage: %w", err)
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
		return fmt.Errorf("codegen: load schemas: %w", err)
	}
	cfg.Schema = spec.PkgPath

	// 2. Transform schemas: inject blind indexes for fle.Searchable() fields
	if err := fle.TransformSchemas(spec.Schemas); err != nil {
		return fmt.Errorf("codegen fle: %w", err)
	}

	// 3. Transform schemas: inject database-level CHECK constraints for Enums
	if err := check.InjectEnumChecks(spec.Schemas); err != nil {
		return fmt.Errorf("codegen enum checks: %w", err)
	}

	// 4. Build Graph and Generate Ent artifacts
	graph, err := gen.NewGraph(cfg, spec.Schemas...)
	if err != nil {
		return fmt.Errorf("codegen: build graph: %w", err)
	}
	if err := graph.Gen(); err != nil {
		return fmt.Errorf("codegen: generate artifacts: %w", err)
	}

	return nil
}
