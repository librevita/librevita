package repository

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"librevita.org/ent"
	"librevita.org/ent/accesspolicyversion"
	"librevita.org/ent/identifiersystem"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/policy"
	usermodel "librevita.org/internal/domain/user/model"
	"librevita.org/pkg/ident"
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
		return false, errors.Wrap(err, "setup repository: load clinic")
	}
	return row.OnboardedAt != nil && !row.OnboardedAt.IsZero(), nil
}

func (r *setupRepository) Onboard(ctx context.Context, admin *usermodel.User, systemIDs []ident.IdentifierSystemID) (*usermodel.User, error) {
	clinicID, err := clinicctx.MustClinicID(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "setup repository: begin onboard")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
			panic(rec)
		}
	}()

	user, err := r.onboardTx(ctx, tx, clinicID, admin, systemIDs)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "setup repository: commit onboard")
	}
	return user, nil
}

func (r *setupRepository) onboardTx(ctx context.Context, tx *ent.Tx, clinicID ident.ClinicID, admin *usermodel.User, systemIDs []ident.IdentifierSystemID) (*usermodel.User, error) {
	row, err := tx.Clinic.Get(ctx, clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "setup repository: load clinic")
	}
	if row.OnboardedAt != nil && !row.OnboardedAt.IsZero() {
		return nil, usermodel.ErrAlreadyOnboarded
	}

	adminRoleID, err := seedRoles(ctx, tx, clinicID)
	if err != nil {
		return nil, err
	}
	if err := seedPolicies(ctx, tx, clinicID); err != nil {
		return nil, err
	}
	if err := optInIdentifierSystems(ctx, tx, clinicID, systemIDs); err != nil {
		return nil, err
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
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrEmailTaken
		}
		return nil, errors.Wrap(err, "setup repository: create admin")
	}

	now := time.Now().UTC()
	if err := tx.Clinic.UpdateOneID(clinicID).SetOnboardedAt(now).Exec(ctx); err != nil {
		return nil, errors.Wrap(err, "setup repository: mark onboarded")
	}
	return toUserDomain(createdUser, "admin"), nil
}

func seedRoles(ctx context.Context, tx *ent.Tx, clinicID ident.ClinicID) (ident.RoleID, error) {
	roles := []struct {
		name     string
		clinical bool
	}{
		{"admin", false},
		{"physician", true},
		{"receptionist", false},
		{"patient", false},
	}
	var adminRoleID ident.RoleID
	for _, rl := range roles {
		rid := ident.New[ident.RoleID]()
		created, err := tx.Role.Create().
			SetID(rid).
			SetClinicID(clinicID).
			SetName(rl.name).
			SetSystem(true).
			SetIsClinical(rl.clinical).
			Save(ctx)
		if err != nil {
			return ident.RoleID{}, errors.Wrapf(err, "setup repository: seed role %q", rl.name)
		}
		if rl.name == "admin" {
			adminRoleID = created.ID
		}
	}
	return adminRoleID, nil
}

func seedPolicies(ctx context.Context, tx *ent.Tx, clinicID ident.ClinicID) error {
	for name, expr := range policy.DefaultPolicies {
		pID := ident.New[ident.PolicyID]()
		pol, err := tx.AccessPolicy.Create().
			SetID(pID).
			SetClinicID(clinicID).
			SetName(name).
			SetExpression(expr).
			Save(ctx)
		if err != nil {
			return errors.Wrapf(err, "setup repository: seed policy %q", name)
		}
		if _, err := tx.AccessPolicyVersion.Create().
			SetPolicyID(pol.ID).
			SetExpression(expr).
			SetOrigin(accesspolicyversion.OriginSeed).
			Save(ctx); err != nil {
			return errors.Wrapf(err, "setup repository: seed policy version %q", name)
		}
	}
	return nil
}

func optInIdentifierSystems(ctx context.Context, tx *ent.Tx, clinicID ident.ClinicID, systemIDs []ident.IdentifierSystemID) error {
	if len(systemIDs) == 0 {
		active, err := tx.IdentifierSystem.Query().Where(identifiersystem.ActiveEQ(true)).All(ctx)
		if err != nil {
			return errors.Wrap(err, "setup repository: list identifier systems")
		}
		for _, sys := range active {
			systemIDs = append(systemIDs, sys.ID)
		}
	}
	for _, sysID := range systemIDs {
		optID := ident.New[ident.ClinicIdentifierSystemID]()
		if err := tx.ClinicIdentifierSystem.Create().
			SetID(optID).
			SetClinicID(clinicID).
			SetIdentifierSystemID(sysID).
			Exec(ctx); err != nil && !ent.IsConstraintError(err) {
			return errors.Wrap(err, "setup repository: identifier opt-in")
		}
	}
	return nil
}
