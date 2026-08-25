package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/accesspolicyversion"
	"librevita.org/ent/identifiersystem"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/policy"
	usermodel "librevita.org/internal/domain/user/model"
)

type setupRepository struct {
	client *ent.Client
}

// NewSetupRepository creates a setup/onboarding repository adapter.
func NewSetupRepository(client *ent.Client) usermodel.SetupRepository {
	return &setupRepository{client: client}
}

func (r *setupRepository) IsOnboarded(ctx context.Context) (bool, error) {
	c, ok := clinicctx.FromContext(ctx)
	if !ok {
		return false, nil
	}
	if c.OnboardedAt != nil && !c.OnboardedAt.IsZero() {
		return true, nil
	}
	row, err := r.client.Clinic.Get(ctx, c.ID)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("setup repository: load clinic: %w", err)
	}
	return row.OnboardedAt != nil && !row.OnboardedAt.IsZero(), nil
}

func (r *setupRepository) Onboard(ctx context.Context, admin *usermodel.User, systemIDs []uuid.UUID) (*usermodel.User, error) {
	clinicID, err := clinicctx.MustClinicID(ctx)
	if err != nil {
		return nil, err
	}

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

	row, err := tx.Clinic.Get(ctx, clinicID)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setup repository: load clinic: %w", err)
	}
	if row.OnboardedAt != nil && !row.OnboardedAt.IsZero() {
		_ = tx.Rollback()
		return nil, usermodel.ErrAlreadyOnboarded
	}

	roles := []struct {
		name     string
		clinical bool
	}{
		{"admin", false},
		{"physician", true},
		{"receptionist", false},
		{"patient", false},
	}
	var adminRoleID uuid.UUID
	for _, rl := range roles {
		id, err := uuid.NewV7()
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		created, err := tx.Role.Create().
			SetID(id).
			SetClinicID(clinicID).
			SetName(rl.name).
			SetSystem(true).
			SetIsClinical(rl.clinical).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("setup repository: seed role %q: %w", rl.name, err)
		}
		if rl.name == "admin" {
			adminRoleID = created.ID
		}
	}

	for name, expr := range policy.DefaultPolicies {
		pID, err := uuid.NewV7()
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		pol, err := tx.AccessPolicy.Create().
			SetID(pID).
			SetClinicID(clinicID).
			SetName(name).
			SetExpression(expr).
			Save(ctx)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("setup repository: seed policy %q: %w", name, err)
		}
		if _, err := tx.AccessPolicyVersion.Create().
			SetPolicyID(pol.ID).
			SetExpression(expr).
			SetOrigin(accesspolicyversion.OriginSeed).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("setup repository: seed policy version %q: %w", name, err)
		}
	}

	if len(systemIDs) == 0 {
		active, err := tx.IdentifierSystem.Query().Where(identifiersystem.ActiveEQ(true)).All(ctx)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("setup repository: list identifier systems: %w", err)
		}
		for _, sys := range active {
			systemIDs = append(systemIDs, sys.ID)
		}
	}
	for _, sysID := range systemIDs {
		optID, err := uuid.NewV7()
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if err := tx.ClinicIdentifierSystem.Create().
			SetID(optID).
			SetClinicID(clinicID).
			SetIdentifierSystemID(sysID).
			Exec(ctx); err != nil && !ent.IsConstraintError(err) {
			_ = tx.Rollback()
			return nil, fmt.Errorf("setup repository: identifier opt-in: %w", err)
		}
	}

	createdUser, err := tx.User.Create().
		SetID(admin.ID).
		SetClinicID(clinicID).
		SetEmail(admin.Email).
		SetPasswordHash(admin.PasswordHash).
		SetDisplayName(admin.DisplayName).
		SetRoleID(adminRoleID).
		SetActive(admin.Active).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrEmailTaken
		}
		return nil, fmt.Errorf("setup repository: create admin: %w", err)
	}

	now := time.Now().UTC()
	if err := tx.Clinic.UpdateOneID(clinicID).SetOnboardedAt(now).Exec(ctx); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setup repository: mark onboarded: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("setup repository: commit onboard: %w", err)
	}

	return toUserDomain(createdUser, "admin"), nil
}
