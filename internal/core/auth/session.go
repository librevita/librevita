package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"aidanwoods.dev/go-paseto"
	"github.com/google/uuid"
	"librevita.org/internal/core/auth/repository"
	"librevita.org/internal/core/config"
)

const (
	// SessionCookieName is the browser cookie holding the session token.
	SessionCookieName = "lv_session"

	// sessionTTL is the lifetime of a session.
	sessionTTL = 7 * 24 * time.Hour
)

// ErrNoSession indicates an unknown, expired, revoked, or tampered token.
var ErrNoSession = errors.New("auth: no valid session")

// Principal is the authenticated identity carried by a session.
type Principal struct {
	ID    string // UUIDv7 of the user account.
	Email string
	Name  string
	Role  Role
}

// SessionManager issues PASETO v4.local session tokens and keeps a
// revocation index in SQLite. The token itself carries the user id and
// expiration date, validated cryptographically on every request; the
// sessions table exists only to support logout and account deactivation.
type SessionManager struct {
	db      *sql.DB
	queries *repository.Queries
	key     paseto.V4SymmetricKey
	ttl     time.Duration
	secure  bool
	log     *slog.Logger
}

// NewSessionManager is the Fx provider. The SQLite backend is required
// because session revocation lives in the embedded database.
//
// The PASETO key comes from config.PasetoKey (base64, 32 bytes). Every
// environment except the explicit "development" requires the key; in
// development an ephemeral key is generated, which invalidates sessions on
// restart.
func NewSessionManager(db *sql.DB, cfg *config.Config, log *slog.Logger) (*SessionManager, error) {
	if db == nil {
		return nil, errors.New("auth: sessions require the SQLite backend")
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
		db:      db,
		queries: repository.New(db),
		key:     key,
		ttl:     sessionTTL,
		secure:  !cfg.IsDevelopment(),
		log:     log,
	}, nil
}

// Create starts a session for p and returns the raw PASETO token. The token
// id is stored hashed in the sessions table so that it can be revoked.
func (m *SessionManager) Create(ctx context.Context, p Principal) (string, error) {
	now := time.Now().UTC()
	expires := now.Add(m.ttl)

	if err := m.queries.DeleteExpiredSessions(ctx, formatTime(now)); err != nil {
		return "", fmt.Errorf("auth: expire sessions: %w", err)
	}

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

	if err := m.queries.CreateSession(ctx, repository.CreateSessionParams{
		TokenHash: hashToken(jtiHex), UserID: uuid.MustParse(p.ID), ExpiresAt: formatTime(expires),
	}); err != nil {
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

	row, err := m.queries.GetSessionUser(ctx, hashToken(jti))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, fmt.Errorf("auth: resolve session: %w", err)
	}

	// Roles are relational rows the administrator can extend, so the
	// name is used as-is; validity is defined by the database.
	return &Principal{ID: row.ID, Email: row.Email, Name: row.DisplayName, Role: Role(row.RoleName)}, nil
}

// Destroy revokes the session behind token.
func (m *SessionManager) Destroy(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	parsed, err := paseto.NewParser().ParseV4Local(m.key, token, nil)
	if err != nil {
		// Expired or tampered tokens have no revocable row.
		return nil
	}
	jti, err := parsed.GetJti()
	if err != nil {
		return nil
	}
	if err := m.queries.DeleteSession(ctx, hashToken(jti)); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

// Cookie builds the session cookie for token.
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

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

// decodeKey parses the base64 PASETO key. It returns nil, nil when the
// configuration value is empty.
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
