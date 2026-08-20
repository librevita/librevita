// Package auth implements session-based authentication using PASETO v4.local
// tokens with server-side revocation in the database.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/google/uuid"
	"golang.org/x/crypto/blake2b"

	"librevita.org/internal/core/config"
)

// SessionCookieName is the name of the session cookie.
const SessionCookieName = "session"

// sessionTTL is the validity period of a session token.
const sessionTTL = 24 * time.Hour

// Principal is the authenticated user identity carried through the request.
type Principal struct {
	ID       string
	Email    string
	Name     string
	Role     Role
	Timezone string
	UITheme  UITheme
}

// SessionUser holds the user information joined in active session queries.
type SessionUser struct {
	ID       uuid.UUID
	Email    string
	Name     string
	Role     Role
	Active   bool
	Timezone string
	UITheme  UITheme
}

// SessionRecord holds the session storage row.
type SessionRecord struct {
	ID        string
	UserID    uuid.UUID
	ExpiresAt time.Time
	User      *SessionUser
}

// SessionRepository defines the storage contract for sessions.
type SessionRepository interface {
	CleanupExpired(ctx context.Context, now time.Time) error
	Create(ctx context.Context, id string, userID uuid.UUID, expiresAt time.Time) error
	GetActive(ctx context.Context, id string, now time.Time) (*SessionRecord, error)
	Delete(ctx context.Context, id string) error
}

// SessionManager issues PASETO v4.local session tokens and keeps a
// revocation index in the database.
type SessionManager struct {
	repo   SessionRepository
	key    paseto.V4SymmetricKey
	ttl    time.Duration
	secure bool
	log    *slog.Logger
}

// NewSessionManager is the Fx provider.
func NewSessionManager(repo SessionRepository, cfg *config.Config, log *slog.Logger) (*SessionManager, error) {
	if repo == nil {
		return nil, errors.New("auth: session repository is nil")
	}

	raw, err := decodeKey(cfg.PasetoKey)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		if !cfg.IsDevelopment() {
			return nil, errors.New("auth: paseto key is required outside development (LIBREVITA_PASETO_KEY)")
		}
		log.Warn("no paseto key configured; using an ephemeral key (sessions reset on restart)")
		raw = make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("auth: ephemeral paseto key: %w", err)
		}
	}

	key, err := paseto.V4SymmetricKeyFromBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("auth: paseto key: %w", err)
	}

	return &SessionManager{
		repo:   repo,
		key:    key,
		ttl:    sessionTTL,
		secure: !cfg.IsDevelopment(),
		log:    log,
	}, nil
}

// Create starts a session for p and returns the raw PASETO token. The token
// id is stored hashed in the sessions table so that it can be revoked.
func (m *SessionManager) Create(ctx context.Context, p Principal) (string, error) {
	now := time.Now().UTC()
	expires := now.Add(m.ttl)

	// Best-effort cleanup of expired sessions
	_ = m.repo.CleanupExpired(ctx, now)

	jti := make([]byte, 32)
	if _, err := rand.Read(jti); err != nil {
		return "", fmt.Errorf("auth: session id: %w", err)
	}
	jtiHex := hex.EncodeToString(jti)

	token := paseto.NewToken()
	token.SetSubject(p.ID)
	token.SetJti(jtiHex)
	token.SetIssuedAt(now)
	token.SetExpiration(expires)
	rawToken := token.V4Encrypt(m.key, nil)

	userUUID, err := uuid.Parse(p.ID)
	if err != nil {
		return "", fmt.Errorf("auth: invalid user id: %w", err)
	}

	if err := m.repo.Create(ctx, m.hashToken(jtiHex), userUUID, expires); err != nil {
		return "", fmt.Errorf("auth: store session: %w", err)
	}
	return rawToken, nil
}

// Authenticate validates the token cryptographically (signature and
// expiration), then checks that it was not revoked and that the account is
// still active. The principal is loaded fresh from the database.
func (m *SessionManager) Authenticate(ctx context.Context, token string) (*Principal, error) {
	parsed, err := paseto.NewParser().ParseV4Local(m.key, token, nil)
	if err != nil {
		return nil, ErrNoSession
	}
	jti, err := parsed.GetJti()
	if err != nil || jti == "" {
		return nil, ErrNoSession
	}

	sess, err := m.repo.GetActive(ctx, m.hashToken(jti), time.Now().UTC())
	if err != nil || sess == nil || sess.User == nil || !sess.User.Active {
		return nil, ErrNoSession
	}

	u := sess.User
	return &Principal{
		ID:       u.ID.String(),
		Email:    u.Email,
		Name:     u.Name,
		Role:     u.Role,
		Timezone: u.Timezone,
		UITheme:  u.UITheme,
	}, nil
}

// Destroy revokes the session behind token.
func (m *SessionManager) Destroy(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	parsed, err := paseto.NewParser().ParseV4Local(m.key, token, nil)
	if err != nil {
		return nil
	}
	jti, err := parsed.GetJti()
	if err != nil {
		return nil
	}
	return m.repo.Delete(ctx, m.hashToken(jti))
}

// CleanupExpired removes expired sessions from the database.
func (m *SessionManager) CleanupExpired(ctx context.Context) error {
	return m.repo.CleanupExpired(ctx, time.Now().UTC())
}

// Cookie builds the session cookie for token.
// #nosec G124 -- Secure is conditional on the environment (dev runs
// without TLS); HttpOnly and SameSite are set.
func (m *SessionManager) Cookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(m.ttl.Seconds()),
	}
}

// ClearCookie returns the cookie that expires the session cookie.
// #nosec G124 -- Secure is conditional on the environment (dev runs
// without TLS); HttpOnly and SameSite are set.
func (m *SessionManager) ClearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

// PrincipalFromContext retrieves the authenticated Principal from ctx.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(*Principal)
	return p, ok && p != nil
}

// ContextWithPrincipal returns a child context carrying p.
func ContextWithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

type principalContextKey struct{}

// hashToken computes the keyed BLAKE2b-256 fingerprint of the session
// token id (jti) stored in the sessions table for revocation.
func (m *SessionManager) hashToken(token string) string {
	h, err := blake2b.New256(m.key.ExportBytes())
	if err != nil {
		panic(fmt.Sprintf("auth: blake2b init: %v", err))
	}
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

func decodeKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("auth: paseto key is not valid base64: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("auth: paseto key must be 32 bytes, got %d", len(raw))
	}
	return raw, nil
}

// Common auth errors.
var (
	ErrNoSession = errors.New("auth: no valid session")
	ErrInactive  = errors.New("auth: account is inactive")
)
