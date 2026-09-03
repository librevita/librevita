// Package isolation enforces clinic_id filters on Ent queries and mutations.
package isolation

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/intercept"
	"librevita.org/pkg/ident"
)

var clinicScopedTypes = map[string]struct{}{
	record.TypeUser:                   {},
	record.TypeRole:                   {},
	record.TypeAccessPolicy:           {},
	record.TypePatient:                {},
	record.TypeSpecialty:              {},
	record.TypeAppointment:            {},
	record.TypeEpisode:                {},
	record.TypeFinding:                {},
	record.TypeProblem:                {},
	record.TypePlanItem:               {},
	record.TypePatientIdentifier:      {},
	record.TypeStaffChangeRequest:     {},
	record.TypeStorageObject:          {},
	record.TypeClinicIdentifierSystem: {},
}

type clinicIDMutation interface {
	SetClinicID(ident.ClinicID)
	ClinicID() (ident.ClinicID, bool)
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
			return isolateMutation(ctx, next, m)
		})
	}
}

func isolateMutation(ctx context.Context, next ent.Mutator, m ent.Mutation) (ent.Value, error) {
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
		if err := applyCreateClinicID(m, id); err != nil {
			return nil, err
		}
	} else {
		restrictMutationToClinic(m, id)
	}
	return next.Mutate(ctx, m)
}

func applyCreateClinicID(m ent.Mutation, clinicID ident.ClinicID) error {
	setter, ok := m.(clinicIDMutation)
	if !ok {
		return nil
	}
	if existing, set := setter.ClinicID(); set && !existing.IsZero() && existing != clinicID {
		return errors.New("isolation: clinic_id mismatch")
	}
	setter.SetClinicID(clinicID)
	return nil
}

func restrictMutationToClinic(m ent.Mutation, clinicID ident.ClinicID) {
	wp, ok := m.(wherePMutation)
	if !ok {
		return
	}
	wp.WhereP(func(s *sql.Selector) {
		s.Where(sql.EQ(s.C("clinic_id"), clinicID))
	})
}
