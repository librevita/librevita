package check

import (
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestInjectEnumChecks(t *testing.T) {
	statusDesc := field.Enum("status").Values("active", "inactive", "pending").Descriptor()
	statusFld, err := load.NewField(statusDesc)
	require.NoError(t, err)

	schema := &load.Schema{
		Name:   "User",
		Fields: []*load.Field{statusFld},
	}

	err = InjectEnumChecks([]*load.Schema{schema})
	require.NoError(t, err)

	require.NotEmpty(t, schema.Annotations)
}
