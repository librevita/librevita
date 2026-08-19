package repository

import (
	"context"
	"fmt"

	"librevita.org/ent"
	"librevita.org/ent/meta"
	clinicmodel "librevita.org/internal/domain/clinic/model"
	usermodel "librevita.org/internal/domain/user/model"
)

const setupMetaKey = "setup_completed"

type setupRepository struct {
	client *ent.Client
}

// NewSetupRepository creates a setup/onboarding repository adapter.
func NewSetupRepository(client *ent.Client) usermodel.SetupRepository {
	return &setupRepository{client: client}
}

func (r *setupRepository) IsOnboarded(ctx context.Context) (bool, error) {
	exists, err := r.client.Meta.Query().Where(meta.IDEQ(setupMetaKey)).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("setup repository: read setup marker: %w", err)
	}
	if exists {
		return true, nil
	}
	count, err := r.client.User.Query().Count(ctx)
	if err != nil {
		return false, fmt.Errorf("setup repository: count users: %w", err)
	}
	return count > 0, nil
}

func (r *setupRepository) Onboard(ctx context.Context, admin *usermodel.User, c *clinicmodel.Clinic) (*usermodel.User, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("setup repository: begin onboard: %w", err)
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
			panic(rec)
		}
	}()

	exists, err := tx.Meta.Query().Where(meta.IDEQ(setupMetaKey)).Exist(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setup repository: read setup marker: %w", err)
	}
	if exists {
		_ = tx.Rollback()
		return nil, usermodel.ErrAlreadyOnboarded
	}

	count, err := tx.User.Query().Count(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setup repository: count users: %w", err)
	}
	if count > 0 {
		_ = tx.Rollback()
		return nil, usermodel.ErrAlreadyOnboarded
	}

	clinicCreate := tx.Clinic.Create().
		SetID(c.ID).
		SetName(c.Name).
		SetCountry(c.Country).
		SetTimezone(c.Timezone)
	if c.TaxID != "" {
		clinicCreate.SetTaxID(c.TaxID)
	}
	if _, err := clinicCreate.Save(ctx); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setup repository: create clinic: %w", err)
	}

	createdUser, err := tx.User.Create().
		SetID(admin.ID).
		SetEmail(admin.Email).
		SetPasswordHash(admin.PasswordHash).
		SetDisplayName(admin.DisplayName).
		SetRoleID(admin.RoleID).
		SetActive(admin.Active).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrEmailTaken
		}
		return nil, fmt.Errorf("setup repository: create admin: %w", err)
	}

	if err := tx.Meta.Create().SetID(setupMetaKey).SetValue("1").Exec(ctx); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setup repository: mark setup complete: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("setup repository: commit onboard: %w", err)
	}

	return toUserDomain(createdUser, "admin"), nil
}
