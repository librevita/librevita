package auth

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/internal/core/kv"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/platformuser"
	"librevita.org/pkg/ident"
	"librevita.org/pkg/urn"
)

// PlatformSessionRepository stores apex sessions bound to platform_users.
type PlatformSessionRepository interface {
	CleanupExpired(ctx context.Context, now time.Time) error
	Create(ctx context.Context, id string, userID uuid.UUID, expiresAt time.Time) error
	GetActive(ctx context.Context, id string, now time.Time) (*SessionRecord, error)
	Delete(ctx context.Context, id string) error
}

type platformSessionRepository struct {
	sessionKV
}

// NewPlatformSessionRepository stores apex sessions in the same kv store as clinic sessions.
func NewPlatformSessionRepository(store kv.Store, client *record.Client) PlatformSessionRepository {
	return &platformSessionRepository{sessionKV: sessionKV{store: store, client: client}}
}

func (r *platformSessionRepository) Create(ctx context.Context, id string, userID uuid.UUID, expiresAt time.Time) error {
	if err := r.put(ctx, urn.PlatformSession(id), userID, expiresAt); err != nil {
		return errors.Wrap(err, "platform session repository: create")
	}
	return nil
}

func (r *platformSessionRepository) GetActive(ctx context.Context, sessionID string, now time.Time) (*SessionRecord, error) {
	p, err := r.getActive(ctx, urn.PlatformSession(sessionID), now)
	if err != nil {
		return nil, errors.Wrap(err, "platform session repository: get active")
	}
	if p == nil {
		return nil, nil
	}
	usr, err := r.client.PlatformUser.Query().
		Where(platformuser.IDEQ(ident.PlatformUserID(p.UserID))).
		Only(ctx)
	if err != nil {
		if record.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "platform session repository: load user")
	}
	return &SessionRecord{
		ID:        sessionID,
		UserID:    p.UserID,
		ExpiresAt: p.ExpiresAt,
		User: &SessionUser{
			ID:     usr.ID.UUID(),
			Email:  usr.Email,
			Name:   usr.DisplayName,
			Active: usr.Active,
		},
	}, nil
}

func (r *platformSessionRepository) Delete(ctx context.Context, id string) error {
	if err := r.delete(ctx, urn.PlatformSession(id)); err != nil {
		return errors.Wrap(err, "platform session repository: delete")
	}
	return nil
}

func (r *platformSessionRepository) CleanupExpired(ctx context.Context, now time.Time) error {
	if err := r.cleanupExpired(ctx, urn.PlatformSessionPrefix, now); err != nil {
		return errors.Wrap(err, "platform session repository: cleanup expired")
	}
	return nil
}
