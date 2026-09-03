package policy

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/accesspolicy"
	"librevita.org/internal/database/record/accesspolicyversion"
	"librevita.org/pkg/ident"
)

type policyRepository struct {
	client *record.Client
}

// NewPolicyRepository creates a policy repository adapter.
func NewPolicyRepository(client *record.Client) Repository {
	return &policyRepository{client: client}
}

func (r *policyRepository) SeedDefaults(ctx context.Context, defaults map[string]string) error {
	clinicID, err := clinicctx.MustClinicID(ctx)
	if err != nil {
		return err
	}
	for name, expr := range defaults {
		exists, err := r.client.AccessPolicy.Query().
			Where(accesspolicy.NameEQ(name), accesspolicy.ClinicIDEQ(clinicID)).
			Exist(ctx)
		if err != nil {
			return errors.Wrapf(err, "policy repository: check %q", name)
		}
		if exists {
			continue
		}

		pID := ident.New[ident.PolicyID]()

		tx, err := r.client.Tx(ctx)
		if err != nil {
			return errors.Wrapf(err, "policy repository: tx for %q", name)
		}

		pol, err := tx.AccessPolicy.Create().
			SetID(pID).
			SetClinicID(clinicID).
			SetName(name).
			SetExpression(expr).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrapf(err, "policy repository: seed insert %q", name)
		}

		_, err = tx.AccessPolicyVersion.Create().
			SetPolicyID(pol.ID).
			SetExpression(expr).
			SetOrigin(accesspolicyversion.OriginSeed).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrapf(err, "policy repository: seed version %q", name)
		}

		if err := tx.Commit(); err != nil {
			return errors.Wrapf(err, "policy repository: commit seed %q", name)
		}
	}
	return nil
}

func (r *policyRepository) List(ctx context.Context) ([]PolicyRow, error) {
	rows, err := r.client.AccessPolicy.Query().
		Order(record.Asc(accesspolicy.FieldName)).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "policy repository: list")
	}

	out := make([]PolicyRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, PolicyRow{
			ID:         row.ID.String(),
			Name:       row.Name,
			Expression: row.Expression,
			UpdatedAt:  row.UpdatedAt,
		})
	}
	return out, nil
}

func (r *policyRepository) Set(ctx context.Context, name, expression string, actor Actor, origin string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return errors.Wrap(err, "policy repository: tx")
	}

	existing, err := tx.AccessPolicy.Query().
		Where(accesspolicy.NameEQ(name)).
		Only(ctx)
	if err != nil && !record.IsNotFound(err) {
		_ = tx.Rollback()
		return errors.Wrap(err, "policy repository: lookup")
	}

	var polID ident.PolicyID
	if record.IsNotFound(err) {
		pID := ident.New[ident.PolicyID]()
		create := tx.AccessPolicy.Create().
			SetID(pID).
			SetName(name).
			SetExpression(expression)
		if clinicID, ok := clinicctx.ClinicID(ctx); ok {
			create.SetClinicID(clinicID)
		}
		pol, err := create.Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrap(err, "policy repository: create")
		}
		polID = pol.ID
	} else {
		polID = existing.ID
		_, err = tx.AccessPolicy.UpdateOneID(polID).
			SetExpression(expression).
			SetUpdatedAt(time.Now()).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrap(err, "policy repository: update")
		}
	}

	verCreate := tx.AccessPolicyVersion.Create().
		SetPolicyID(polID).
		SetExpression(expression).
		SetOrigin(accesspolicyversion.Origin(origin))

	if actor.ID != "" {
		verCreate.SetChangedBy(actor.ID)
	}
	if actor.Email != "" {
		verCreate.SetChangedByEmail(actor.Email)
	}

	_, err = verCreate.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return errors.Wrap(err, "policy repository: version insert")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "policy repository: commit")
	}
	return nil
}

func (r *policyRepository) History(ctx context.Context, name string, limit int) ([]PolicyVersionRow, error) {
	pol, err := r.client.AccessPolicy.Query().
		Where(accesspolicy.NameEQ(name)).
		Only(ctx)
	if record.IsNotFound(err) {
		return nil, ErrPolicyNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "policy repository: lookup")
	}

	versions, err := r.client.AccessPolicyVersion.Query().
		Where(accesspolicyversion.PolicyIDEQ(pol.ID)).
		Order(record.Desc(accesspolicyversion.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "policy repository: history")
	}

	out := make([]PolicyVersionRow, 0, len(versions))
	for _, v := range versions {
		row := PolicyVersionRow{
			ID:             int64(v.ID),
			PolicyID:       v.PolicyID.String(),
			Expression:     v.Expression,
			Origin:         string(v.Origin),
			CreatedAt:      v.CreatedAt,
			ChangedBy:      v.ChangedBy,
			ChangedByEmail: v.ChangedByEmail,
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *policyRepository) Count(ctx context.Context) (int64, error) {
	count, err := r.client.AccessPolicy.Query().Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "policy repository: count")
	}
	return int64(count), nil
}
