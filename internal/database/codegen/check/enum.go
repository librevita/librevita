package check

import (
	"fmt"
	"strings"
	"unicode"

	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
)

// InjectEnumChecks inspects all loaded Ent schemas and automatically adds
// database-level CHECK constraints for all field.Enum fields with consistent
// snake_case naming aligned with the table name (e.g., "staff_change_requests_status_check").
func InjectEnumChecks(schemas []*load.Schema) error {
	annKey := entsql.Annotation{}.Name()

	for _, s := range schemas {
		tblName := tableName(s)
		tableChecks := make(map[string]string)

		for _, f := range s.Fields {
			if f.Info == nil || f.Info.Type != field.TypeEnum || len(f.Enums) == 0 {
				continue
			}

			colName := f.StorageKey
			if colName == "" {
				colName = f.Name
			}

			quotedVals := make([]string, 0, len(f.Enums))
			for _, e := range f.Enums {
				val := e.V
				if val == "" {
					val = e.N
				}
				quotedVals = append(quotedVals, fmt.Sprintf("'%s'", val))
			}

			checkExpr := fmt.Sprintf("%s IN (%s)", colName, strings.Join(quotedVals, ", "))
			checkName := fmt.Sprintf("%s_%s_check", tblName, colName)
			tableChecks[checkName] = checkExpr

			// Also set on field level
			if f.Annotations == nil {
				f.Annotations = make(map[string]any)
			}
			f.Annotations[annKey] = entsql.Annotation{
				Check: checkExpr,
			}
		}

		if len(tableChecks) > 0 {
			if s.Annotations == nil {
				s.Annotations = make(map[string]any)
			}
			if existing, ok := s.Annotations[annKey]; ok {
				if ann, ok := existing.(entsql.Annotation); ok {
					if ann.Checks == nil {
						ann.Checks = make(map[string]string)
					}
					for k, v := range tableChecks {
						ann.Checks[k] = v
					}
					s.Annotations[annKey] = ann
				} else if annPtr, ok := existing.(*entsql.Annotation); ok {
					if annPtr.Checks == nil {
						annPtr.Checks = make(map[string]string)
					}
					for k, v := range tableChecks {
						annPtr.Checks[k] = v
					}
				} else if annMap, ok := existing.(map[string]any); ok {
					var checks map[string]any
					if chk, ok := annMap["checks"].(map[string]any); ok {
						checks = chk
					} else {
						checks = make(map[string]any)
						annMap["checks"] = checks
					}
					for k, v := range tableChecks {
						checks[k] = v
					}
				}
			} else {
				s.Annotations[annKey] = entsql.Annotation{
					Checks: tableChecks,
				}
			}
		}
	}
	return nil
}

func tableName(s *load.Schema) string {
	annKey := entsql.Annotation{}.Name()
	if ann, ok := s.Annotations[annKey]; ok {
		if a, ok := ann.(entsql.Annotation); ok && a.Table != "" {
			return a.Table
		}
		if a, ok := ann.(*entsql.Annotation); ok && a.Table != "" {
			return a.Table
		}
		if m, ok := ann.(map[string]any); ok {
			if t, ok := m["table"].(string); ok && t != "" {
				return t
			}
			if t, ok := m["Table"].(string); ok && t != "" {
				return t
			}
		}
	}
	return toSnakeCase(pluralize(s.Name))
}

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func pluralize(name string) string {
	if strings.HasSuffix(name, "y") && !strings.HasSuffix(name, "ay") && !strings.HasSuffix(name, "ey") && !strings.HasSuffix(name, "oy") && !strings.HasSuffix(name, "uy") {
		return strings.TrimSuffix(name, "y") + "ies"
	}
	if strings.HasSuffix(name, "s") || strings.HasSuffix(name, "x") || strings.HasSuffix(name, "z") || strings.HasSuffix(name, "ch") || strings.HasSuffix(name, "sh") {
		return name + "es"
	}
	return name + "s"
}
