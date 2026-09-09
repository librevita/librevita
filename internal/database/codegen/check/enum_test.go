package check

import (
	"testing"

	"entgo.io/ent/dialect/entsql"
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

func TestInjectEnumChecks_MergeAnnotations(t *testing.T) {
	annKey := entsql.Annotation{}.Name()

	// 1. Existing entsql.Annotation
	statusDesc := field.Enum("status").Values("active", "inactive").Descriptor()
	statusFld, err := load.NewField(statusDesc)
	require.NoError(t, err)

	schemaVal := &load.Schema{
		Name:   "User",
		Fields: []*load.Field{statusFld},
		Annotations: map[string]any{
			annKey: entsql.Annotation{
				Checks: map[string]string{"custom_check": "col > 0"},
			},
		},
	}
	require.NoError(t, InjectEnumChecks([]*load.Schema{schemaVal}))
	valAnn := schemaVal.Annotations[annKey].(entsql.Annotation)
	require.Contains(t, valAnn.Checks, "custom_check")
	require.Contains(t, valAnn.Checks, "users_status_check")

	// 2. Existing *entsql.Annotation
	schemaPtr := &load.Schema{
		Name:   "User",
		Fields: []*load.Field{statusFld},
		Annotations: map[string]any{
			annKey: &entsql.Annotation{
				Table: "custom_users",
			},
		},
	}
	require.NoError(t, InjectEnumChecks([]*load.Schema{schemaPtr}))
	ptrAnn := schemaPtr.Annotations[annKey].(*entsql.Annotation)
	require.Contains(t, ptrAnn.Checks, "custom_users_status_check")

	// 3. Existing map[string]any with checks
	schemaMap := &load.Schema{
		Name:   "User",
		Fields: []*load.Field{statusFld},
		Annotations: map[string]any{
			annKey: map[string]any{
				"table":  "tbl_users",
				"checks": map[string]any{"prev": "val"},
			},
		},
	}
	require.NoError(t, InjectEnumChecks([]*load.Schema{schemaMap}))
	mapAnn := schemaMap.Annotations[annKey].(map[string]any)
	checks := mapAnn["checks"].(map[string]any)
	require.Contains(t, checks, "tbl_users_status_check")

	// 4. Existing map[string]any with capitalized Table and without checks
	schemaMapCap := &load.Schema{
		Name:   "User",
		Fields: []*load.Field{statusFld},
		Annotations: map[string]any{
			annKey: map[string]any{
				"Table": "Cap_Users",
			},
		},
	}
	require.NoError(t, InjectEnumChecks([]*load.Schema{schemaMapCap}))
}

func TestInjectEnumChecks_PluralizeAndSnakeCase(t *testing.T) {
	cases := []struct {
		name     string
		expected string
	}{
		{"Category", "categories"},
		{"Day", "days"},
		{"Box", "boxes"},
		{"Batch", "batches"},
		{"Bus", "buses"},
		{"Dish", "dishes"},
		{"Buzz", "buzzes"},
		{"UserProfile", "user_profiles"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statusDesc := field.Enum("type").Values("a", "b").StorageKey("custom_type").Descriptor()
			statusFld, err := load.NewField(statusDesc)
			require.NoError(t, err)

			nonEnumDesc := field.String("name").Descriptor()
			nonEnumFld, err := load.NewField(nonEnumDesc)
			require.NoError(t, err)

			schema := &load.Schema{
				Name:   tc.name,
				Fields: []*load.Field{nonEnumFld, statusFld},
			}

			require.NoError(t, InjectEnumChecks([]*load.Schema{schema}))
			annKey := entsql.Annotation{}.Name()
			ann := schema.Annotations[annKey].(entsql.Annotation)
			expectedKey := tc.expected + "_custom_type_check"
			require.Contains(t, ann.Checks, expectedKey)
		})
	}
}
