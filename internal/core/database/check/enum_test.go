package check

import (
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/entc/load"
	"github.com/stretchr/testify/require"
)

func TestEntSQLAnnotation(t *testing.T) {
	spec, err := (&load.Config{Path: "../../../database/schema"}).Load()
	require.NoError(t, err)

	err = InjectEnumChecks(spec.Schemas)
	require.NoError(t, err)

	cfg := &gen.Config{
		Package: "librevita.org/ent",
		Target:  t.TempDir(),
	}
	graph, err := gen.NewGraph(cfg, spec.Schemas...)
	require.NoError(t, err)

	for _, typ := range graph.Nodes {
		ant := typ.EntSQL()
		t.Logf("Node %s -> typ.EntSQL(): %+v", typ.Name, ant)
	}
}
