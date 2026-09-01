// Package auth implements session-based authentication using PASETO v4.local
// tokens with server-side revocation in a key-value store.
package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/pkg/log"
)

// SessionCookieName is the name of the session cookie.
const SessionCookieName = "session"

// sessionTTL is the validity period of a session token.
const sessionTTL = 24 * time.Hour

// Principal is the authenticated user identity carried through the request.
type Principal struct {
	ID        string
	Email     string
	Name      string
	Role      Role
	Timezone  string
	UITheme   UITheme
	ClinicID  string
	PatientID string
	Platform  bool
}

// SessionUser holds the user information joined in active session queries.
type SessionUser struct {
	ID        uuid.UUID
	Email     string
	Name      string
	Role      Role
	Active    bool
	Timezone  string
	UITheme   UITheme
	ClinicID  uuid.UUID
	PatientID uuid.UUID
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
// revocation index in a key-value store.
type SessionManager struct {
	repo     SessionRepository
	platform PlatformSessionRepository
	key      paseto.V4SymmetricKey
	hasher   crypto.Hasher
	ttl      time.Duration
	secure   bool
	log      log.Logger
}

// NewSessionManager is the Fx provider.
func NewSessionManager(repo SessionRepository, cfg *config.Config, logger log.Logger) (*SessionManager, error) {
	if repo == nil {
		return nil, errors.New("auth: session repository is nil")
	}

	raw, err := decodeKey(cfg.PasetoKey)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		if !cfg.IsDevelopment() {
			err := errors.New("auth: paseto key is required outside development (LIBREVITA_PASETO_KEY)")
			return nil, errors.WithHint(err, "Gere uma chave simétrica segura de 32 bytes em base64 (ex: openssl rand -base64 32) e configure na variável LIBREVITA_PASETO_KEY.")
		}
		logger.Warn("no paseto key configured; using an ephemeral key (sessions reset on restart)")
		var randErr error
		raw, randErr = crypto.RandomBytes(32)
		if randErr != nil {
			return nil, errors.Wrap(randErr, "auth: ephemeral paseto key")
		}
	}

	key, err := paseto.V4SymmetricKeyFromBytes(raw)
	if err != nil {
		crypto.ZeroBytes(raw)
		return nil, errors.Wrap(err, "auth: paseto key")
	}

	algo := ""
	if cfg != nil {
		algo = cfg.Crypto.HashAlgorithm
	}
	if algo == "" {
		algo = crypto.DefaultHashAlgorithm
	}
	hasher, err := crypto.NewSessionHasher(raw, crypto.WithHashAlgorithm(algo))
	crypto.ZeroBytes(raw)
	if err != nil {
		return nil, errors.Wrap(err, "auth: session hasher init")
	}

	secure := false
	if cfg != nil {
		secure = !cfg.IsDevelopment()
	}

	return &SessionManager{
		repo:   repo,
		key:    key,
		hasher: hasher,
		ttl:    sessionTTL,
		secure: secure,
		log:    logger,
	}, nil
}

// SetPlatformRepository attaches apex session storage. Optional in tests.
func (m *SessionManager) SetPlatformRepository(repo PlatformSessionRepository) {
	m.platform = repo
}

// Create starts a session for p and returns the raw PASETO token. The token
// id is stored hashed so that it can be revoked.
func (m *SessionManager) Create(ctx context.Context, p Principal) (string, error) {
	now := time.Now().UTC()
	expires := now.Add(m.ttl)

	jtiHex, err := crypto.RandomHex(32)
	if err != nil {
		return "", errors.Wrap(err, "auth: session id")
	}

	token := paseto.NewToken()
	token.SetSubject(p.ID)
	token.SetJti(jtiHex)
	token.SetIssuedAt(now)
	token.SetExpiration(expires)
	rawToken := token.V4Encrypt(m.key, nil)

	userUUID, err := uuid.Parse(p.ID)
	if err != nil {
		return "", errors.Wrap(err, "auth: invalid user id")
	}

	hashed, err := m.hashToken(jtiHex)
	if err != nil {
		return "", err
	}
	if p.Platform || clinicctx.IsApex(ctx) {
		if m.platform == nil {
			return "", errors.New("auth: platform session repository is not configured")
		}
		if err := m.platform.Create(ctx, hashed, userUUID, expires); err != nil {
			return "", errors.Wrap(err, "auth: store platform session")
		}
		return rawToken, nil
	}
	if err := m.repo.Create(ctx, hashed, userUUID, expires); err != nil {
		return "", errors.Wrap(err, "auth: store session")
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
	hashed, err := m.hashToken(jti)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	if clinicctx.IsApex(ctx) {
		return m.authenticatePlatform(ctx, hashed, now)
	}
	return m.authenticateClinic(ctx, hashed, now)
}

func (m *SessionManager) authenticateClinic(ctx context.Context, hashed string, now time.Time) (*Principal, error) {
	sess, err := m.repo.GetActive(ctx, hashed, now)
	if err != nil || sess == nil || sess.User == nil || !sess.User.Active {
		return nil, ErrNoSession
	}
	u := sess.User
	if cid, ok := clinicctx.ClinicID(ctx); ok && u.ClinicID != uuid.Nil && u.ClinicID != cid {
		return nil, ErrNoSession
	}
	p := &Principal{
		ID:        u.ID.String(),
		Email:     u.Email,
		Name:      u.Name,
		Role:      u.Role,
		Timezone:  u.Timezone,
		UITheme:   u.UITheme,
		ClinicID:  u.ClinicID.String(),
		PatientID: "",
	}
	if u.PatientID != uuid.Nil {
		p.PatientID = u.PatientID.String()
	}
	if u.ClinicID == uuid.Nil {
		p.ClinicID = ""
	}
	return p, nil
}

func (m *SessionManager) authenticatePlatform(ctx context.Context, hashed string, now time.Time) (*Principal, error) {
	if m.platform == nil {
		return nil, ErrNoSession
	}
	sess, err := m.platform.GetActive(ctx, hashed, now)
	if err != nil || sess == nil || sess.User == nil || !sess.User.Active {
		return nil, ErrNoSession
	}
	u := sess.User
	return &Principal{
		ID:       u.ID.String(),
		Email:    u.Email,
		Name:     u.Name,
		Timezone: u.Timezone,
		UITheme:  u.UITheme,
		Platform: true,
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
	hashed, err := m.hashToken(jti)
	if err != nil {
		return err
	}
	if clinicctx.IsApex(ctx) && m.platform != nil {
		return m.platform.Delete(ctx, hashed)
	}
	return m.repo.Delete(ctx, hashed)
}

// CleanupExpired removes expired sessions from the session store.
// Login and Authenticate do not call this; a background ticker does.
func (m *SessionManager) CleanupExpired(ctx context.Context) error {
	now := time.Now().UTC()
	if err := m.repo.CleanupExpired(ctx, now); err != nil {
		return err
	}
	if m.platform != nil {
		return m.platform.CleanupExpired(ctx, now)
	}
	return nil
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

// hashToken computes the keyed fingerprint of the session token id (jti)
// stored in the sessions store for revocation. There is no unprefixed fallback.
func (m *SessionManager) hashToken(token string) (string, error) {
	if m.hasher == nil {
		return "", errors.New("auth: session hasher is not configured")
	}
	digest, err := m.hasher.HashString(token)
	if err != nil {
		return "", errors.Wrap(err, "auth: session fingerprint")
	}
	return digest, nil
}

func decodeKey(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "auth: paseto key is not valid base64")
	}
	if len(raw) != 32 {
		return nil, errors.Newf("auth: paseto key must be 32 bytes, got %d", len(raw))
	}
	return raw, nil
}

// Common auth errors.
var (
	ErrNoSession = errors.New("auth: no valid session")
	ErrInactive  = errors.New("auth: account is inactive")
)
