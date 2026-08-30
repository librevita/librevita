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
			name, expr, ok := enumCheckForField(f, tblName, annKey)
			if !ok {
				continue
			}
			tableChecks[name] = expr
		}
		if len(tableChecks) > 0 {
			mergeTableChecks(s, annKey, tableChecks)
		}
	}
	return nil
}

func enumCheckForField(f *load.Field, tblName, annKey string) (string, string, bool) {
	if f.Info == nil || f.Info.Type != field.TypeEnum || len(f.Enums) == 0 {
		return "", "", false
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
	if f.Annotations == nil {
		f.Annotations = make(map[string]any)
	}
	f.Annotations[annKey] = entsql.Annotation{Check: checkExpr}
	return fmt.Sprintf("%s_%s_check", tblName, colName), checkExpr, true
}

func mergeTableChecks(s *load.Schema, annKey string, tableChecks map[string]string) {
	if s.Annotations == nil {
		s.Annotations = make(map[string]any)
	}
	existing, ok := s.Annotations[annKey]
	if !ok {
		s.Annotations[annKey] = entsql.Annotation{Checks: tableChecks}
		return
	}
	mergeExistingChecks(s, annKey, existing, tableChecks)
}

func mergeExistingChecks(s *load.Schema, annKey string, existing any, tableChecks map[string]string) {
	if ann, ok := existing.(entsql.Annotation); ok {
		mergeAnnotationChecks(&ann, tableChecks)
		s.Annotations[annKey] = ann
		return
	}
	if annPtr, ok := existing.(*entsql.Annotation); ok {
		mergeAnnotationChecks(annPtr, tableChecks)
		return
	}
	if annMap, ok := existing.(map[string]any); ok {
		mergeMapChecks(annMap, tableChecks)
	}
}

func mergeAnnotationChecks(ann *entsql.Annotation, tableChecks map[string]string) {
	if ann.Checks == nil {
		ann.Checks = make(map[string]string)
	}
	for k, v := range tableChecks {
		ann.Checks[k] = v
	}
}

func mergeMapChecks(annMap map[string]any, tableChecks map[string]string) {
	checks, ok := annMap["checks"].(map[string]any)
	if !ok {
		checks = make(map[string]any)
		annMap["checks"] = checks
	}
	for k, v := range tableChecks {
		checks[k] = v
	}
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
	if consonantY(name) {
		return strings.TrimSuffix(name, "y") + "ies"
	}
	if sibilant(name) {
		return name + "es"
	}
	return name + "s"
}

func consonantY(name string) bool {
	return strings.HasSuffix(name, "y") && !strings.HasSuffix(name, "ay") && !strings.HasSuffix(name, "ey") && !strings.HasSuffix(name, "oy") && !strings.HasSuffix(name, "uy")
}

func sibilant(name string) bool {
	return strings.HasSuffix(name, "s") || strings.HasSuffix(name, "x") || strings.HasSuffix(name, "z") || strings.HasSuffix(name, "ch") || strings.HasSuffix(name, "sh")
}
