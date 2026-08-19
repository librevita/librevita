package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/role"
	"librevita.org/ent/user"
	usermodel "librevita.org/internal/domain/user/model"
)

type roleRepository struct {
	client *ent.Client
}

// NewRoleRepository creates a role repository adapter.
func NewRoleRepository(client *ent.Client) usermodel.RoleRepository {
	return &roleRepository{client: client}
}

func (r *roleRepository) List(ctx context.Context) ([]usermodel.Role, error) {
	roles, err := r.client.Role.Query().
		Order(ent.Desc(role.FieldSystem), ent.Asc(role.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("role repository: list: %w", err)
	}

	rows := make([]usermodel.Role, 0, len(roles))
	for _, rl := range roles {
		rows = append(rows, *toRoleDomain(rl))
	}
	return rows, nil
}

func (r *roleRepository) GetByID(ctx context.Context, id uuid.UUID) (*usermodel.Role, error) {
	rl, err := r.client.Role.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("role repository: role not found")
		}
		return nil, fmt.Errorf("role repository: get by id: %w", err)
	}
	return toRoleDomain(rl), nil
}

func (r *roleRepository) GetByName(ctx context.Context, name string) (*usermodel.Role, error) {
	rl, err := r.client.Role.Query().Where(role.NameEQ(name)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("role repository: role not found: %s", name)
		}
		return nil, fmt.Errorf("role repository: get by name: %w", err)
	}
	return toRoleDomain(rl), nil
}

func (r *roleRepository) Create(ctx context.Context, roleModel *usermodel.Role) (*usermodel.Role, error) {
	create := r.client.Role.Create().
		SetID(roleModel.ID).
		SetName(roleModel.Name).
		SetIsClinical(roleModel.IsClinical)

	saved, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrDuplicateRole
		}
		return nil, fmt.Errorf("role repository: create: %w", err)
	}
	return toRoleDomain(saved), nil
}

func (r *roleRepository) Update(ctx context.Context, roleModel *usermodel.Role) (*usermodel.Role, error) {
	update := r.client.Role.UpdateOneID(roleModel.ID).
		SetName(roleModel.Name).
		SetIsClinical(roleModel.IsClinical)

	saved, err := update.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrDuplicateRole
		}
		return nil, fmt.Errorf("role repository: update: %w", err)
	}
	return toRoleDomain(saved), nil
}

func (r *roleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.client.Role.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("role repository: delete: %w", err)
	}
	return nil
}

func (r *roleRepository) CountUsersWithRole(ctx context.Context, roleID uuid.UUID) (int, error) {
	count, err := r.client.User.Query().Where(user.RoleIDEQ(roleID)).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("role repository: count users: %w", err)
	}
	return count, nil
}

func (r *roleRepository) SeedDefaults(ctx context.Context) error {
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
			return fmt.Errorf("role repository: seed check %q: %w", dr.name, err)
		}
		if exists {
			continue
		}
		rID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if err := r.client.Role.Create().
			SetID(rID).
			SetName(dr.name).
			SetSystem(true).
			SetIsClinical(dr.isClinical).
			Exec(ctx); err != nil && !ent.IsConstraintError(err) {
			return fmt.Errorf("role repository: seed insert %q: %w", dr.name, err)
		}
	}
	return nil
}

func toRoleDomain(rl *ent.Role) *usermodel.Role {
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
