// Package isolation enforces clinic_id filters on Ent queries and mutations.
package isolation

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	entgen "librevita.org/ent"
	"librevita.org/ent/intercept"
	"librevita.org/internal/core/clinicctx"
)

var clinicScopedTypes = map[string]struct{}{
	entgen.TypeUser:                   {},
	entgen.TypeRole:                   {},
	entgen.TypeAccessPolicy:           {},
	entgen.TypePatient:                {},
	entgen.TypeSpecialty:              {},
	entgen.TypeAppointment:            {},
	entgen.TypeEpisode:                {},
	entgen.TypeFinding:                {},
	entgen.TypeProblem:                {},
	entgen.TypePlanItem:               {},
	entgen.TypePatientIdentifier:      {},
	entgen.TypeStaffChangeRequest:     {},
	entgen.TypeStorageObject:          {},
	entgen.TypeClinicIdentifierSystem: {},
}

type clinicIDMutation interface {
	SetClinicID(uuid.UUID)
	ClinicID() (uuid.UUID, bool)
}

type wherePMutation interface {
	WhereP(...func(*sql.Selector))
}

// QueryInterceptor filters clinic-scoped queries by clinic_id from context.
// Apex and skip-isolation contexts do not filter User via a privacy skip:
// apex simply has no clinic, so User queries fail (use platform_users).
func QueryInterceptor() ent.Interceptor {
	return intercept.TraverseFunc(func(ctx context.Context, q intercept.Query) error {
		if clinicctx.IsolationSkipped(ctx) {
			return nil
		}
		if _, ok := clinicScopedTypes[q.Type()]; !ok {
			return nil
		}
		id, ok := clinicctx.ClinicID(ctx)
		if !ok {
			return clinicctx.ErrMissingClinic
		}
		q.WhereP(func(s *sql.Selector) {
			s.Where(sql.EQ(s.C("clinic_id"), id))
		})
		return nil
	})
}

// MutationHook sets clinic_id on create and restricts updates/deletes to the
// clinic in context.
func MutationHook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if clinicctx.IsolationSkipped(ctx) {
				return next.Mutate(ctx, m)
			}
			if _, ok := clinicScopedTypes[m.Type()]; !ok {
				return next.Mutate(ctx, m)
			}
			id, ok := clinicctx.ClinicID(ctx)
			if !ok {
				return nil, clinicctx.ErrMissingClinic
			}
			if m.Op().Is(ent.OpCreate) {
				if setter, ok := m.(clinicIDMutation); ok {
					if existing, set := setter.ClinicID(); set && existing != uuid.Nil && existing != id {
						return nil, errors.New("isolation: clinic_id mismatch")
					}
					setter.SetClinicID(id)
				}
			} else if wp, ok := m.(wherePMutation); ok {
				wp.WhereP(func(s *sql.Selector) {
					s.Where(sql.EQ(s.C("clinic_id"), id))
				})
			}
			return next.Mutate(ctx, m)
		})
	}
}
