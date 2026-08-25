package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/testutil"
)

const testUserID = "01990000-0000-7000-8000-000000000001"

func openSessionTest(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:session-test-"+uuid.NewString()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	log := slog.New(slog.DiscardHandler)
	err = database.Migrate(context.Background(), db, log)
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func seedUser(t *testing.T, client *ent.Client, id string) {
	t.Helper()
	hash, err := HashPassword("test-password")
	require.NoError(t, err)
	err = testutil.User(context.Background(), client, id, "user@example.org", "admin", hash)
	require.NoError(t, err)
}

func newManager(t *testing.T, client *ent.Client, ttl time.Duration) *SessionManager {
	t.Helper()
	return &SessionManager{
		repo:   NewSessionRepository(client),
		ttl:    ttl,
		secure: false,
		log:    slog.New(slog.DiscardHandler),
	}
}

func TestSessionLifecycle(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	p, err := m.Authenticate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, testUserID, p.ID)
	assert.Equal(t, RoleAdmin, p.Role)
	assert.Equal(t, "user@example.org", p.Email)

	err = m.Destroy(context.Background(), token)
	require.NoError(t, err)

	_, err = m.Authenticate(context.Background(), token)
	assert.Equal(t, ErrNoSession, err)
}

func TestAuthenticateRejectsOtherClinic(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	home := clinicctx.WithTestClinic(context.Background())
	token, err := m.Create(home, Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	p, err := m.Authenticate(home, token)
	require.NoError(t, err)
	assert.Equal(t, testUserID, p.ID)
	assert.Equal(t, clinicctx.TestClinicID.String(), p.ClinicID)

	other := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{
		ID:       uuid.MustParse("01990000-0000-7000-8000-0000000000c2"),
		Slug:     "other",
		Name:     "Other",
		Timezone: "America/Sao_Paulo",
	})
	_, err = m.Authenticate(other, token)
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionUnknownToken(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	_, err := m.Authenticate(context.Background(), "bogus")
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionExpiry(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, -time.Hour) // Already expired.

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	_, err = m.Authenticate(context.Background(), token)
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionRejectsDeactivatedUser(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)

	userUUID := uuid.MustParse(testUserID)
	err := client.User.UpdateOneID(userUUID).SetActive(false).Exec(context.Background())
	require.NoError(t, err)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	_, err = m.Authenticate(context.Background(), token)
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionManagerRequiresClient(t *testing.T) {
	_, err := NewSessionManager(nil, &config.Config{Mode: "development"}, slog.New(slog.DiscardHandler))
	assert.Error(t, err)
}

func TestSessionManagerRequiresKeyInProduction(t *testing.T) {
	client := openSessionTest(t)
	cfg := &config.Config{Mode: "production"}
	_, err := NewSessionManager(NewSessionRepository(client), cfg, slog.New(slog.DiscardHandler))
	assert.Error(t, err)
}

func TestSessionManagerRejectsMalformedKey(t *testing.T) {
	client := openSessionTest(t)
	for _, key := range []string{"not-base64!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		cfg := &config.Config{Mode: "production", PasetoKey: key}
		_, err := NewSessionManager(NewSessionRepository(client), cfg, slog.New(slog.DiscardHandler))
		assert.Error(t, err, "NewSessionManager with key %q should fail", key)
	}
}

func TestSessionManagerAcceptsConfiguredKey(t *testing.T) {
	client := openSessionTest(t)
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	m, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "production", PasetoKey: key}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	seedUser(t, client, testUserID)
	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	_, err = m.Authenticate(context.Background(), token)
	assert.NoError(t, err)
}

func TestSessionRejectsTamperedToken(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	tampered := token[:len(token)-1] + string(flipByte(token[len(token)-1]))
	_, err = m.Authenticate(context.Background(), tampered)
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionTokenIsPaseto(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, "v4.local."), "token %q does not use PASETO v4.local", token)
}

func flipByte(b byte) byte {
	if b == 0 {
		return 1
	}
	return b ^ 0x01
}

func TestSessionManagerKeyBoundary(t *testing.T) {
	client := openSessionTest(t)
	log := slog.New(slog.DiscardHandler)

	for _, env := range []string{"production", "staging", "prod", "test"} {
		_, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: env}, log)
		assert.Error(t, err, "env %q must require a paseto key", env)
	}

	m, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "development"}, log)
	require.NoError(t, err)
	assert.False(t, m.secure, "development cookies must not use Secure")

	m, err = NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "staging", PasetoKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))}, log)
	require.NoError(t, err)
	assert.True(t, m.secure, "non-development cookies must use Secure")
}

func TestSessionCookieAttributes(t *testing.T) {
	client := openSessionTest(t)
	log := slog.New(slog.DiscardHandler)

	dev, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "development"}, log)
	require.NoError(t, err)
	prod, err := NewSessionManager(NewSessionRepository(client), &config.Config{
		Mode: "production", PasetoKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)),
	}, log)
	require.NoError(t, err)

	for name, m := range map[string]*SessionManager{"development": dev, "production": prod} {
		for _, c := range []*http.Cookie{m.Cookie("token"), m.ClearCookie()} {
			assert.True(t, c.HttpOnly, "%s: session cookie must be HttpOnly", name)
			assert.Equal(t, http.SameSiteLaxMode, c.SameSite, "%s: session cookie must be SameSite=Lax", name)
			assert.Empty(t, c.Domain, "%s: session cookie must be host-only", name)
			wantSecure := name == "production"
			assert.Equal(t, wantSecure, c.Secure, "%s: Secure mismatch", name)
		}
	}
}
