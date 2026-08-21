//go:build ignore

package main

import (
	"log"
	"os"

	"librevita.org/internal/core/database/zk"
)

func main() {
	schemaDir := "."
	targetDir := "../../../ent"
	if _, err := os.Stat("internal/database/schema"); err == nil {
		schemaDir = "./internal/database/schema"
		targetDir = "./ent"
	}

	if err := zk.Generate(schemaDir, targetDir); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
