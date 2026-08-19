package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/accesspolicy"
	"librevita.org/ent/accesspolicyversion"
)

type policyRepository struct {
	client *ent.Client
}

// NewPolicyRepository creates a policy repository adapter.
func NewPolicyRepository(client *ent.Client) Repository {
	return &policyRepository{client: client}
}

func (r *policyRepository) SeedDefaults(ctx context.Context, defaults map[string]string) error {
	for name, expr := range defaults {
		exists, err := r.client.AccessPolicy.Query().
			Where(accesspolicy.NameEQ(name)).
			Exist(ctx)
		if err != nil {
			return fmt.Errorf("policy repository: check %q: %w", name, err)
		}
		if exists {
			continue
		}

		pID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("policy repository: uuid for %q: %w", name, err)
		}

		tx, err := r.client.Tx(ctx)
		if err != nil {
			return fmt.Errorf("policy repository: tx for %q: %w", name, err)
		}

		pol, err := tx.AccessPolicy.Create().
			SetID(pID).
			SetName(name).
			SetExpression(expr).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("policy repository: seed insert %q: %w", name, err)
		}

		_, err = tx.AccessPolicyVersion.Create().
			SetPolicyID(pol.ID).
			SetExpression(expr).
			SetOrigin("seed").
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("policy repository: seed version %q: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("policy repository: commit seed %q: %w", name, err)
		}
	}
	return nil
}

func (r *policyRepository) List(ctx context.Context) ([]PolicyRow, error) {
	rows, err := r.client.AccessPolicy.Query().
		Order(ent.Asc(accesspolicy.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("policy repository: list: %w", err)
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
		return fmt.Errorf("policy repository: tx: %w", err)
	}

	existing, err := tx.AccessPolicy.Query().
		Where(accesspolicy.NameEQ(name)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		_ = tx.Rollback()
		return fmt.Errorf("policy repository: lookup: %w", err)
	}

	var polID uuid.UUID
	if ent.IsNotFound(err) {
		pID, err := uuid.NewV7()
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("policy repository: uuid: %w", err)
		}
		pol, err := tx.AccessPolicy.Create().
			SetID(pID).
			SetName(name).
			SetExpression(expression).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("policy repository: create: %w", err)
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
			return fmt.Errorf("policy repository: update: %w", err)
		}
	}

	verCreate := tx.AccessPolicyVersion.Create().
		SetPolicyID(polID).
		SetExpression(expression).
		SetOrigin(origin)

	if actor.ID != "" {
		verCreate.SetChangedBy(actor.ID)
	}
	if actor.Email != "" {
		verCreate.SetChangedByEmail(actor.Email)
	}

	_, err = verCreate.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("policy repository: version insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("policy repository: commit: %w", err)
	}
	return nil
}

func (r *policyRepository) History(ctx context.Context, name string, limit int) ([]PolicyVersionRow, error) {
	pol, err := r.client.AccessPolicy.Query().
		Where(accesspolicy.NameEQ(name)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrPolicyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("policy repository: lookup: %w", err)
	}

	versions, err := r.client.AccessPolicyVersion.Query().
		Where(accesspolicyversion.PolicyIDEQ(pol.ID)).
		Order(ent.Desc(accesspolicyversion.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("policy repository: history: %w", err)
	}

	out := make([]PolicyVersionRow, 0, len(versions))
	for _, v := range versions {
		row := PolicyVersionRow{
			ID:             int64(v.ID),
			PolicyID:       v.PolicyID.String(),
			Expression:     v.Expression,
			Origin:         v.Origin,
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
		return 0, fmt.Errorf("policy repository: count: %w", err)
	}
	return int64(count), nil
}
