package repository

import (
	"context"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/role"
	"librevita.org/internal/database/record/user"
	usermodel "librevita.org/internal/domain/user/model"
	"librevita.org/pkg/ident"
)

type roleRepository struct {
	client *record.Client
}

// NewRoleRepository creates a role repository adapter.
func NewRoleRepository(client *record.Client) usermodel.RoleRepository {
	return &roleRepository{client: client}
}

func (r *roleRepository) List(ctx context.Context) ([]usermodel.Role, error) {
	roles, err := r.client.Role.Query().
		Order(record.Desc(role.FieldSystem), record.Asc(role.FieldName)).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "role repository: list")
	}

	rows := make([]usermodel.Role, 0, len(roles))
	for _, rl := range roles {
		rows = append(rows, *toRoleDomain(rl))
	}
	return rows, nil
}

func (r *roleRepository) GetByID(ctx context.Context, id ident.RoleID) (*usermodel.Role, error) {
	rl, err := r.client.Role.Get(ctx, id)
	if err != nil {
		if record.IsNotFound(err) {
			return nil, errors.New("role repository: role not found")
		}
		return nil, errors.Wrap(err, "role repository: get by id")
	}
	return toRoleDomain(rl), nil
}

func (r *roleRepository) GetByName(ctx context.Context, name string) (*usermodel.Role, error) {
	rl, err := r.client.Role.Query().Where(role.NameEQ(name)).Only(ctx)
	if err != nil {
		if record.IsNotFound(err) {
			return nil, errors.Newf("role repository: role not found: %s", name)
		}
		return nil, errors.Wrap(err, "role repository: get by name")
	}
	return toRoleDomain(rl), nil
}

func (r *roleRepository) Create(ctx context.Context, roleModel *usermodel.Role) (*usermodel.Role, error) {
	create := r.client.Role.Create().
		SetID(roleModel.ID).
		SetName(roleModel.Name).
		SetIsClinical(roleModel.IsClinical)
	if clinicID, ok := clinicctx.ClinicID(ctx); ok {
		create.SetClinicID(clinicID)
	}

	saved, err := create.Save(ctx)
	if err != nil {
		if record.IsConstraintError(err) {
			return nil, errors.WithSecondaryError(usermodel.ErrDuplicateRole, err)
		}
		return nil, errors.Wrap(err, "role repository: create")
	}
	return toRoleDomain(saved), nil
}

func (r *roleRepository) Update(ctx context.Context, roleModel *usermodel.Role) (*usermodel.Role, error) {
	update := r.client.Role.UpdateOneID(roleModel.ID).
		SetName(roleModel.Name).
		SetIsClinical(roleModel.IsClinical)

	saved, err := update.Save(ctx)
	if err != nil {
		if record.IsConstraintError(err) {
			return nil, errors.WithSecondaryError(usermodel.ErrDuplicateRole, err)
		}
		return nil, errors.Wrap(err, "role repository: update")
	}
	return toRoleDomain(saved), nil
}

func (r *roleRepository) Delete(ctx context.Context, id ident.RoleID) error {
	err := r.client.Role.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "role repository: delete")
	}
	return nil
}

func (r *roleRepository) CountUsersWithRole(ctx context.Context, roleID ident.RoleID) (int, error) {
	count, err := r.client.User.Query().Where(user.RoleIDEQ(roleID)).Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "role repository: count users")
	}
	return count, nil
}

func (r *roleRepository) SeedDefaults(ctx context.Context) error {
	clinicID, err := clinicctx.MustClinicID(ctx)
	if err != nil {
		return err
	}
	defaultRoles := []struct {
		name       string
		isClinical bool
	}{
		{name: "admin", isClinical: false},
		{name: "physician", isClinical: true},
		{name: "receptionist", isClinical: false},
		{name: "patient", isClinical: false},
	}

	for _, dr := range defaultRoles {
		exists, err := r.client.Role.Query().Where(role.NameEQ(dr.name)).Exist(ctx)
		if err != nil {
			return errors.Wrapf(err, "role repository: seed check %q", dr.name)
		}
		if exists {
			continue
		}
		rID := ident.New[ident.RoleID]()
		if err := r.client.Role.Create().
			SetID(rID).
			SetClinicID(clinicID).
			SetName(dr.name).
			SetSystem(true).
			SetIsClinical(dr.isClinical).
			Exec(ctx); err != nil && !record.IsConstraintError(err) {
			return errors.Wrapf(err, "role repository: seed insert %q", dr.name)
		}
	}
	return nil
}

func toRoleDomain(rl *record.Role) *usermodel.Role {
	if rl == nil {
		return nil
	}
	return &usermodel.Role{
		ID:         rl.ID,
		Name:       rl.Name,
		System:     rl.System,
		IsClinical: rl.IsClinical,
		CreatedAt:  rl.CreatedAt,
	}
}
