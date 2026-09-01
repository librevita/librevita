package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/user"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/kv"
)

type sessionPayload struct {
	UserID    uuid.UUID `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type sessionKV struct {
	store  kv.Store
	client *ent.Client
}

func encodeSession(userID uuid.UUID, expiresAt time.Time) ([]byte, error) {
	b, err := json.Marshal(sessionPayload{UserID: userID, ExpiresAt: expiresAt.UTC()})
	if err != nil {
		return nil, errors.Wrap(err, "session: encode")
	}
	return b, nil
}

func decodeSession(raw []byte) (sessionPayload, error) {
	var p sessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return sessionPayload{}, errors.Wrap(err, "session: decode")
	}
	return p, nil
}

func (r *sessionKV) put(ctx context.Context, key string, userID uuid.UUID, expiresAt time.Time) error {
	raw, err := encodeSession(userID, expiresAt)
	if err != nil {
		return err
	}
	return r.store.Put(ctx, key, raw)
}

func (r *sessionKV) getActive(ctx context.Context, key string, now time.Time) (*sessionPayload, error) {
	raw, err := r.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	p, err := decodeSession(raw)
	if err != nil {
		return nil, err
	}
	if !p.ExpiresAt.After(now) {
		return nil, nil
	}
	return &p, nil
}

func (r *sessionKV) delete(ctx context.Context, key string) error {
	return r.store.Delete(ctx, key)
}

func (r *sessionKV) cleanupExpired(ctx context.Context, prefix string, now time.Time) error {
	entries, err := r.store.ListPrefix(ctx, prefix)
	if err != nil {
		return err
	}
	for _, e := range entries {
		p, err := decodeSession(e.Value)
		if err != nil {
			continue
		}
		if p.ExpiresAt.After(now) {
			continue
		}
		if err := r.store.Delete(ctx, e.Key); err != nil {
			return err
		}
	}
	return nil
}

func clinicSessionKey(ctx context.Context, tokenHash string) (string, error) {
	id, ok := clinicctx.ClinicID(ctx)
	if !ok {
		return "", clinicctx.ErrMissingClinic
	}
	return crypto.ClinicSessionURN(id, tokenHash), nil
}

type sessionRepository struct {
	sessionKV
}

// NewSessionRepository stores clinic sessions in kv and loads users from SQL.
func NewSessionRepository(store kv.Store, client *ent.Client) SessionRepository {
	return &sessionRepository{sessionKV: sessionKV{store: store, client: client}}
}

func (r *sessionRepository) Create(ctx context.Context, id string, userID uuid.UUID, expiresAt time.Time) error {
	key, err := clinicSessionKey(ctx, id)
	if err != nil {
		return errors.Wrap(err, "session repository: create")
	}
	if err := r.put(ctx, key, userID, expiresAt); err != nil {
		return errors.Wrap(err, "session repository: create")
	}
	return nil
}

func (r *sessionRepository) GetActive(ctx context.Context, id string, now time.Time) (*SessionRecord, error) {
	key, err := clinicSessionKey(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "session repository: get active")
	}
	p, err := r.getActive(ctx, key, now)
	if err != nil {
		return nil, errors.Wrap(err, "session repository: get active")
	}
	if p == nil {
		return nil, nil
	}
	u, err := r.loadUser(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	return &SessionRecord{
		ID:        id,
		UserID:    p.UserID,
		ExpiresAt: p.ExpiresAt,
		User:      u,
	}, nil
}

func (r *sessionRepository) loadUser(ctx context.Context, userID uuid.UUID) (*SessionUser, error) {
	usr, err := r.client.User.Query().
		Where(user.IDEQ(userID)).
		WithRole().
		WithPortalPatient().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "session repository: load user")
	}
	var roleName Role
	if usr.Edges.Role != nil {
		roleName = Role(usr.Edges.Role.Name)
	}
	u := &SessionUser{
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
	return u, nil
}

func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	key, err := clinicSessionKey(ctx, id)
	if err != nil {
		return errors.Wrap(err, "session repository: delete")
	}
	if err := r.delete(ctx, key); err != nil {
		return errors.Wrap(err, "session repository: delete")
	}
	return nil
}

func (r *sessionRepository) CleanupExpired(ctx context.Context, now time.Time) error {
	if err := r.cleanupExpired(ctx, "urn:librevita:clinic:", now); err != nil {
		return errors.Wrap(err, "session repository: cleanup expired")
	}
	return nil
}
