package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/platformsession"
)

// PlatformSessionRepository stores apex sessions bound to platform_users.
type PlatformSessionRepository interface {
	CleanupExpired(ctx context.Context, now time.Time) error
	Create(ctx context.Context, id string, userID uuid.UUID, expiresAt time.Time) error
	GetActive(ctx context.Context, id string, now time.Time) (*SessionRecord, error)
	Delete(ctx context.Context, id string) error
}

type platformSessionRepository struct {
	client *ent.Client
}

// NewPlatformSessionRepository creates a platform session repository adapter.
func NewPlatformSessionRepository(client *ent.Client) PlatformSessionRepository {
	return &platformSessionRepository{client: client}
}

func (r *platformSessionRepository) Create(ctx context.Context, id string, userID uuid.UUID, expiresAt time.Time) error {
	_, err := r.client.PlatformSession.Create().
		SetID(id).
		SetPlatformUserID(userID).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("platform session repository: create: %w", err)
	}
	return nil
}

func (r *platformSessionRepository) GetActive(ctx context.Context, id string, now time.Time) (*SessionRecord, error) {
	s, err := r.client.PlatformSession.Query().
		Where(
			platformsession.IDEQ(id),
			platformsession.ExpiresAtGT(now),
		).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("platform session repository: get active: %w", err)
	}

	var u *SessionUser
	if s.Edges.User != nil {
		usr := s.Edges.User
		u = &SessionUser{
			ID:     usr.ID,
			Email:  usr.Email,
			Name:   usr.DisplayName,
			Active: usr.Active,
		}
	}

	return &SessionRecord{
		ID:        s.ID,
		UserID:    s.PlatformUserID,
		ExpiresAt: s.ExpiresAt,
		User:      u,
	}, nil
}

func (r *platformSessionRepository) Delete(ctx context.Context, id string) error {
	err := r.client.PlatformSession.DeleteOneID(id).Exec(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("platform session repository: delete: %w", err)
	}
	return nil
}

func (r *platformSessionRepository) CleanupExpired(ctx context.Context, now time.Time) error {
	_, err := r.client.PlatformSession.Delete().
		Where(platformsession.ExpiresAtLTE(now)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("platform session repository: cleanup expired: %w", err)
	}
	return nil
}
