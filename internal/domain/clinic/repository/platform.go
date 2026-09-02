package repository

import (
	"context"

	"github.com/cockroachdb/errors"

	"librevita.org/ent"
	"librevita.org/ent/platformuser"
	"librevita.org/pkg/ident"
)

// PlatformUser is an installation operator stored in platform_users.
type PlatformUser struct {
	ID           ident.PlatformUserID
	Email        string
	PasswordHash string
	DisplayName  string
	Active       bool
}

// PlatformUserRepository persists apex operators.
type PlatformUserRepository interface {
	Count(ctx context.Context) (int, error)
	Create(ctx context.Context, u *PlatformUser) (*PlatformUser, error)
	GetByEmail(ctx context.Context, email string) (*PlatformUser, error)
	GetByID(ctx context.Context, id ident.PlatformUserID) (*PlatformUser, error)
}

type platformUserRepository struct {
	client *ent.Client
}

// NewPlatformUserRepository creates a platform user repository adapter.
func NewPlatformUserRepository(client *ent.Client) PlatformUserRepository {
	return &platformUserRepository{client: client}
}

func (r *platformUserRepository) Count(ctx context.Context) (int, error) {
	n, err := r.client.PlatformUser.Query().Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "platform user: count")
	}
	return n, nil
}

func (r *platformUserRepository) Create(ctx context.Context, u *PlatformUser) (*PlatformUser, error) {
	saved, err := r.client.PlatformUser.Create().
		SetID(u.ID).
		SetEmail(u.Email).
		SetPasswordHash(u.PasswordHash).
		SetDisplayName(u.DisplayName).
		SetActive(u.Active).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errors.New("platform user: email taken")
		}
		return nil, errors.Wrap(err, "platform user: create")
	}
	return toPlatformUser(saved), nil
}

func (r *platformUserRepository) GetByEmail(ctx context.Context, email string) (*PlatformUser, error) {
	row, err := r.client.PlatformUser.Query().Where(platformuser.EmailEQ(email)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "platform user: get by email")
	}
	return toPlatformUser(row), nil
}

func (r *platformUserRepository) GetByID(ctx context.Context, id ident.PlatformUserID) (*PlatformUser, error) {
	row, err := r.client.PlatformUser.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "platform user: get by id")
	}
	return toPlatformUser(row), nil
}

func toPlatformUser(row *ent.PlatformUser) *PlatformUser {
	if row == nil {
		return nil
	}
	return &PlatformUser{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Active:       row.Active,
	}
}
