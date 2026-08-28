package repository

import (
	"context"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/patient"
	"librevita.org/ent/role"
	"librevita.org/ent/specialty"
	"librevita.org/ent/staffchangerequest"
	"librevita.org/ent/user"
	"librevita.org/internal/core/clinicctx"
	usermodel "librevita.org/internal/domain/user/model"
)

type userRepository struct {
	client *ent.Client
}

// NewUserRepository creates a user repository adapter.
func NewUserRepository(client *ent.Client) usermodel.UserRepository {
	return &userRepository{client: client}
}

func (r *userRepository) Create(ctx context.Context, u *usermodel.User) (*usermodel.User, error) {
	clinicID := u.ClinicID
	if clinicID == uuid.Nil {
		var err error
		clinicID, err = clinicctx.MustClinicID(ctx)
		if err != nil {
			return nil, err
		}
	}
	create := r.client.User.Create().
		SetID(u.ID).
		SetClinicID(clinicID).
		SetEmail(u.Email).
		SetPasswordHash(u.PasswordHash).
		SetDisplayName(u.DisplayName).
		SetRoleID(u.RoleID).
		SetActive(u.Active)

	saved, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrEmailTaken
		}
		return nil, errors.Wrap(err, "user repository: create")
	}

	roleName := u.RoleName
	if roleName == "" {
		if rRow, err := r.client.Role.Get(ctx, saved.RoleID); err == nil && rRow != nil {
			roleName = rRow.Name
		}
	}

	return toUserDomain(saved, roleName), nil
}

func (r *userRepository) BindPortalPatient(ctx context.Context, userID, patientID uuid.UUID) error {
	n, err := r.client.Patient.Update().
		Where(patient.IDEQ(patientID), patient.UserIDIsNil()).
		SetUserID(userID).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return usermodel.ErrEmailTaken
		}
		return errors.Wrap(err, "user repository: bind portal patient")
	}
	if n == 0 {
		return usermodel.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*usermodel.GetUserByIDRow, error) {
	u, err := r.client.User.Query().
		Where(user.IDEQ(id)).
		WithRole().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, usermodel.ErrUserNotFound
		}
		return nil, errors.Wrap(err, "user repository: get by id")
	}

	roleName := ""
	roleIsClinical := false
	if u.Edges.Role != nil {
		roleName = u.Edges.Role.Name
		roleIsClinical = u.Edges.Role.IsClinical
	}

	return &usermodel.GetUserByIDRow{
		ID:             u.ID,
		Email:          u.Email,
		PasswordHash:   u.PasswordHash,
		DisplayName:    u.DisplayName,
		RoleID:         u.RoleID,
		RoleName:       roleName,
		RoleIsClinical: roleIsClinical,
		Active:         u.Active,
		Timezone:       u.Timezone,
		UITheme:        string(u.UITheme),
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
	}, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*usermodel.GetUserByIDRow, error) {
	u, err := r.client.User.Query().
		Where(user.EmailEQ(email)).
		WithRole().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, usermodel.ErrUserNotFound
		}
		return nil, errors.Wrap(err, "user repository: get by email")
	}

	roleName := ""
	roleIsClinical := false
	if u.Edges.Role != nil {
		roleName = u.Edges.Role.Name
		roleIsClinical = u.Edges.Role.IsClinical
	}

	return &usermodel.GetUserByIDRow{
		ID:             u.ID,
		Email:          u.Email,
		PasswordHash:   u.PasswordHash,
		DisplayName:    u.DisplayName,
		RoleID:         u.RoleID,
		RoleName:       roleName,
		RoleIsClinical: roleIsClinical,
		Active:         u.Active,
		Timezone:       u.Timezone,
		UITheme:        string(u.UITheme),
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
	}, nil
}

func (r *userRepository) Update(ctx context.Context, u *usermodel.User) (*usermodel.User, error) {
	update := r.client.User.UpdateOneID(u.ID).
		SetEmail(u.Email).
		SetDisplayName(u.DisplayName).
		SetRoleID(u.RoleID).
		SetActive(u.Active).
		SetUpdatedAt(time.Now())

	saved, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, usermodel.ErrUserNotFound
		}
		if ent.IsConstraintError(err) {
			return nil, usermodel.ErrEmailTaken
		}
		return nil, errors.Wrap(err, "user repository: update")
	}

	return toUserDomain(saved, u.RoleName), nil
}

func (r *userRepository) UpdatePreferences(ctx context.Context, id uuid.UUID, timezone, theme string) error {
	err := r.client.User.UpdateOneID(id).
		SetTimezone(timezone).
		SetUITheme(user.UITheme(theme)).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return usermodel.ErrUserNotFound
		}
		return errors.Wrap(err, "user repository: update preferences")
	}
	return nil
}

func (r *userRepository) Count(ctx context.Context) (int64, error) {
	count, err := r.client.User.Query().Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "user repository: count")
	}
	return int64(count), nil
}

func (r *userRepository) CountByRole(ctx context.Context, roleName string) (int64, error) {
	count, err := r.client.User.Query().
		Where(user.HasRoleWith(role.NameEQ(roleName))).
		Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "user repository: count by role")
	}
	return int64(count), nil
}

func (r *userRepository) CountStaff(ctx context.Context, roleNames []string) (int64, error) {
	count, err := r.client.User.Query().
		Where(user.HasRoleWith(role.NameIn(roleNames...))).
		Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "user repository: count staff")
	}
	return int64(count), nil
}

func (r *userRepository) CountActiveAdmins(ctx context.Context) (int, error) {
	count, err := r.client.User.Query().
		Where(
			user.HasRoleWith(role.NameEQ("admin")),
			user.ActiveEQ(true),
		).
		Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "user repository: count active admins")
	}
	return count, nil
}

func (r *userRepository) ListRecent(ctx context.Context, limit int) ([]usermodel.ListRecentUsersRow, error) {
	users, err := r.client.User.Query().
		WithRole().
		Order(ent.Desc(user.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "user repository: list recent")
	}

	rows := make([]usermodel.ListRecentUsersRow, 0, len(users))
	for _, u := range users {
		roleName := ""
		if u.Edges.Role != nil {
			roleName = u.Edges.Role.Name
		}
		rows = append(rows, usermodel.ListRecentUsersRow{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			RoleName:    roleName,
			CreatedAt:   u.CreatedAt,
		})
	}
	return rows, nil
}

func (r *userRepository) ListPage(ctx context.Context, q string, limit, offset int) ([]usermodel.ListUsersRow, int64, error) {
	query := r.client.User.Query().WithRole().Order(ent.Desc(user.FieldCreatedAt))
	countQuery := r.client.User.Query()

	trimmed := strings.TrimSpace(q)
	if trimmed != "" {
		pattern := "% " + strings.ToLower(trimmed) + "%"
		predicate := func(s *entsql.Selector) {
			s.Where(entsql.ExprP("lower(' ' || "+s.C(user.FieldEmail)+" || ' ' || "+s.C(user.FieldDisplayName)+") LIKE ?", pattern))
		}
		query = query.Where(predicate)
		countQuery = countQuery.Where(predicate)
	}

	users, err := query.Limit(limit).Offset(offset).All(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "user repository: list page")
	}

	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "user repository: count page")
	}

	rows := make([]usermodel.ListUsersRow, 0, len(users))
	for _, u := range users {
		roleName := ""
		if u.Edges.Role != nil {
			roleName = u.Edges.Role.Name
		}
		rows = append(rows, usermodel.ListUsersRow{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			RoleName:    roleName,
			Active:      u.Active,
			CreatedAt:   u.CreatedAt,
		})
	}
	return rows, int64(total), nil
}

func (r *userRepository) ListPhysiciansPage(ctx context.Context, limit, offset int) ([]usermodel.ListPhysiciansPageRow, int64, error) {
	query := r.client.User.Query().
		Where(user.HasRoleWith(role.IsClinical(true))).
		WithSpecialties(func(sq *ent.SpecialtyQuery) {
			sq.Order(ent.Asc(specialty.FieldName))
		}).
		Order(ent.Asc(user.FieldDisplayName)).
		Limit(limit).
		Offset(offset)

	users, err := query.All(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "user repository: list physicians")
	}

	total, err := r.client.User.Query().Where(user.HasRoleWith(role.IsClinical(true))).Count(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "user repository: count physicians")
	}

	rows := make([]usermodel.ListPhysiciansPageRow, 0, len(users))
	for _, u := range users {
		spNames := make([]string, 0, len(u.Edges.Specialties))
		for _, sp := range u.Edges.Specialties {
			spNames = append(spNames, sp.Name)
		}
		rows = append(rows, usermodel.ListPhysiciansPageRow{
			ID:          u.ID,
			DisplayName: u.DisplayName,
			Email:       u.Email,
			Active:      u.Active,
			Specialties: strings.Join(spNames, ", "),
		})
	}
	return rows, int64(total), nil
}

func (r *userRepository) SetSpecialties(ctx context.Context, userID uuid.UUID, specialtyIDs []uuid.UUID) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return errors.Wrap(err, "user repository: begin specialty tx")
	}

	uUpdate := tx.User.UpdateOneID(userID).ClearSpecialties()
	if len(specialtyIDs) > 0 {
		uUpdate.AddSpecialtyIDs(specialtyIDs...)
	}
	if err := uUpdate.Exec(ctx); err != nil {
		_ = tx.Rollback()
		return errors.Wrap(err, "user repository: set specialties")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "user repository: commit specialty tx")
	}
	return nil
}

func (r *userRepository) ApplyApprovedStaffChange(ctx context.Context, reqID, userID, deciderID uuid.UUID, name, email string, specialtyIDs []uuid.UUID) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return errors.Wrap(err, "user repository: begin staff change tx")
	}

	uUpdate := tx.User.UpdateOneID(userID).
		SetEmail(email).
		SetDisplayName(name).
		ClearSpecialties()
	if len(specialtyIDs) > 0 {
		uUpdate.AddSpecialtyIDs(specialtyIDs...)
	}
	if err := uUpdate.Exec(ctx); err != nil {
		_ = tx.Rollback()
		if ent.IsConstraintError(err) {
			return usermodel.ErrEmailInUse
		}
		return errors.Wrap(err, "user repository: apply staff change")
	}

	decidedAt := time.Now().UTC()
	if err := tx.StaffChangeRequest.UpdateOneID(reqID).
		SetStatus(staffchangerequest.StatusApproved).
		SetDecidedAt(decidedAt).
		SetDecidedBy(deciderID).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return errors.Wrap(err, "user repository: approve request")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "user repository: commit staff change tx")
	}
	return nil
}

func toUserDomain(u *ent.User, roleName string) *usermodel.User {
	if u == nil {
		return nil
	}
	return &usermodel.User{
		ID:           u.ID,
		ClinicID:     u.ClinicID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		DisplayName:  u.DisplayName,
		RoleID:       u.RoleID,
		RoleName:     roleName,
		Active:       u.Active,
		Timezone:     u.Timezone,
		UITheme:      string(u.UITheme),
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}
