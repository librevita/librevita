package fle

import (
	"context"
	"strings"

	"entgo.io/ent"
	"librevita.org/internal/core/crypto"
)

// BlindIndexHook returns an ent.Hook that automatically computes blind indexes
// for any entity with matching _blind_index columns before persisting to database.
func BlindIndexHook(hasher crypto.Hasher) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if !m.Op().Is(ent.OpCreate | ent.OpUpdate | ent.OpUpdateOne) {
				return next.Mutate(ctx, m)
			}

			entityName := strings.ToLower(m.Type())
			if entityName == "" {
				return next.Mutate(ctx, m)
			}

			for _, fieldName := range m.Fields() {
				if strings.HasSuffix(fieldName, "_blind_index") {
					continue
				}

				val, ok := m.Field(fieldName)
				if !ok {
					continue
				}

				blindIndexField := fieldName + "_blind_index"

				// If value is nil or empty string, clear the matching blind index
				if val == nil {
					_ = m.SetField(blindIndexField, "")
					continue
				}

				str, isStr := val.(string)
				if !isStr {
					continue
				}

				if str == "" {
					_ = m.SetField(blindIndexField, "")
					continue
				}

				// Derive cryptographic domain tag (custom context domain or entity.field default)
				domainTag := entityName + "." + fieldName
				if customDomain, _, ok := SearchableFieldFromContext(ctx); ok && customDomain != "" {
					domainTag = customDomain
				}

				// TODO: Canonical normalization for phone, cpf, tax_id (strip spaces, hyphens, punctuation)
				normalized := strings.ToLower(strings.TrimSpace(str))
				if blindHash, err := hasher.BlindIndex(domainTag, normalized); err == nil {
					_ = m.SetField(blindIndexField, blindHash)
				}
			}

			return next.Mutate(ctx, m)
		})
	}
}
