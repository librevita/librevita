package auth

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/session"
)

type sessionRepository struct {
	client *ent.Client
}

// NewSessionRepository creates a session repository adapter.
func NewSessionRepository(client *ent.Client) SessionRepository {
	return &sessionRepository{client: client}
}

func (r *sessionRepository) Create(ctx context.Context, id string, userID uuid.UUID, expiresAt time.Time) error {
	_, err := r.client.Session.Create().
		SetID(id).
		SetUserID(userID).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return errors.Wrap(err, "session repository: create")
	}
	return nil
}

func (r *sessionRepository) GetActive(ctx context.Context, id string, now time.Time) (*SessionRecord, error) {
	s, err := r.client.Session.Query().
		Where(
			session.IDEQ(id),
			session.ExpiresAtGT(now),
		).
		WithUser(func(uq *ent.UserQuery) {
			uq.WithRole().WithPortalPatient()
		}).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "session repository: get active")
	}

	var u *SessionUser
	if s.Edges.User != nil {
		usr := s.Edges.User
		var roleName Role
		if usr.Edges.Role != nil {
			roleName = Role(usr.Edges.Role.Name)
		}
		u = &SessionUser{
			ID:       usr.ID,
			Email:    usr.Email,
			Name:     usr.DisplayName,
			Role:     roleName,
			Active:   usr.Active,
			Timezone: usr.Timezone,
			UITheme:  UITheme(usr.UITheme),
			ClinicID: usr.ClinicID,
		}
		if len(usr.Edges.PortalPatient) > 0 && usr.Edges.PortalPatient[0] != nil {
			u.PatientID = usr.Edges.PortalPatient[0].ID
		}
	}

	return &SessionRecord{
		ID:        s.ID,
		UserID:    s.UserID,
		ExpiresAt: s.ExpiresAt,
		User:      u,
	}, nil
}

func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	err := r.client.Session.DeleteOneID(id).Exec(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return errors.Wrap(err, "session repository: delete")
	}
	return nil
}

func (r *sessionRepository) CleanupExpired(ctx context.Context, now time.Time) error {
	_, err := r.client.Session.Delete().
		Where(session.ExpiresAtLTE(now)).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "session repository: cleanup expired")
	}
	return nil
}
