package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/kv"
	"librevita.org/internal/testutil"
	"librevita.org/pkg/log"
)

const testUserID = "01990000-0000-7000-8000-000000000001"

func openSessionTest(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:session-test-"+uuid.NewString()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	logger := log.Nop()
	err = database.Migrate(context.Background(), db, logger)
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

func testCtx() context.Context {
	return clinicctx.WithTestClinic(context.Background())
}

func testSessionRepo(t *testing.T, client *ent.Client) SessionRepository {
	t.Helper()
	store, err := kv.OpenBBolt(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return NewSessionRepository(store, client)
}

func newManager(t *testing.T, client *ent.Client, ttl time.Duration) *SessionManager {
	t.Helper()
	raw, err := crypto.RandomBytes(32)
	require.NoError(t, err)
	key, err := paseto.V4SymmetricKeyFromBytes(raw)
	require.NoError(t, err)
	hasher, err := crypto.NewSessionHasher(raw)
	require.NoError(t, err)
	crypto.ZeroBytes(raw)
	return &SessionManager{
		repo:   testSessionRepo(t, client),
		key:    key,
		hasher: hasher,
		ttl:    ttl,
		secure: false,
		log:    log.Nop(),
	}
}

func TestSessionLifecycle(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	p, err := m.Authenticate(testCtx(), token)
	require.NoError(t, err)
	assert.Equal(t, testUserID, p.ID)
	assert.Equal(t, RoleAdmin, p.Role)
	assert.Equal(t, "user@example.org", p.Email)

	err = m.Destroy(testCtx(), token)
	require.NoError(t, err)

	_, err = m.Authenticate(testCtx(), token)
	assert.Equal(t, ErrNoSession, err)
}

func TestCreateFailsWithoutHasher(t *testing.T) {
	client := openSessionTest(t)
	raw, err := crypto.RandomBytes(32)
	require.NoError(t, err)
	key, err := paseto.V4SymmetricKeyFromBytes(raw)
	require.NoError(t, err)
	crypto.ZeroBytes(raw)
	m := &SessionManager{
		repo: testSessionRepo(t, client),
		key:  key,
		ttl:  time.Hour,
		log:  log.Nop(),
	}
	_, err = m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session hasher is not configured")
}

func TestSessionFingerprintIsPrefixed(t *testing.T) {
	client := openSessionTest(t)
	m := newManager(t, client, time.Hour)
	fp, err := m.hashToken("jti-example")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(fp, "blake2s$ms$01$"))
}

func TestCreateDoesNotSweepExpiredSessions(t *testing.T) {
	repo := &recordingSessionRepo{}
	m, err := NewSessionManager(repo, &config.Config{Mode: "development"}, log.Nop())
	require.NoError(t, err)

	token, err := m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, 1, repo.creates)
	assert.Equal(t, 0, repo.cleanups)
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
		ID:       ident.MustParseClinic("01990000-0000-7000-8000-0000000000c2"),
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

	token, err := m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	_, err = m.Authenticate(testCtx(), token)
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionRejectsDeactivatedUser(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)

	userUUID := ident.MustParseUser(testUserID)
	err := client.User.UpdateOneID(userUUID).SetActive(false).Exec(context.Background())
	require.NoError(t, err)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	_, err = m.Authenticate(testCtx(), token)
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionManagerRequiresClient(t *testing.T) {
	_, err := NewSessionManager(nil, &config.Config{Mode: "development"}, log.Nop())
	assert.Error(t, err)
}

func TestSessionManagerRequiresKeyInProduction(t *testing.T) {
	client := openSessionTest(t)
	cfg := &config.Config{Mode: "production"}
	_, err := NewSessionManager(testSessionRepo(t, client), cfg, log.Nop())
	assert.Error(t, err)
}

func TestSessionManagerRejectsMalformedKey(t *testing.T) {
	client := openSessionTest(t)
	for _, key := range []string{"not-base64!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		cfg := &config.Config{Mode: "production", PasetoKey: key}
		_, err := NewSessionManager(testSessionRepo(t, client), cfg, log.Nop())
		assert.Error(t, err, "NewSessionManager with key %q should fail", key)
	}
}

func TestSessionManagerAcceptsConfiguredKey(t *testing.T) {
	client := openSessionTest(t)
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	m, err := NewSessionManager(testSessionRepo(t, client), &config.Config{Mode: "production", PasetoKey: key}, log.Nop())
	require.NoError(t, err)

	seedUser(t, client, testUserID)
	token, err := m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	_, err = m.Authenticate(testCtx(), token)
	assert.NoError(t, err)
}

func TestSessionRejectsTamperedToken(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	require.NoError(t, err)

	tampered := token[:len(token)-1] + string(flipByte(token[len(token)-1]))
	_, err = m.Authenticate(context.Background(), tampered)
	assert.Equal(t, ErrNoSession, err)
}

func TestSessionTokenIsPaseto(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(testCtx(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
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
	logger := log.Nop()

	for _, env := range []string{"production", "staging", "prod", "test"} {
		_, err := NewSessionManager(testSessionRepo(t, client), &config.Config{Mode: env}, logger)
		assert.Error(t, err, "env %q must require a paseto key", env)
	}

	m, err := NewSessionManager(testSessionRepo(t, client), &config.Config{Mode: "development"}, logger)
	require.NoError(t, err)
	assert.False(t, m.secure, "development cookies must not use Secure")

	m, err = NewSessionManager(testSessionRepo(t, client), &config.Config{Mode: "staging", PasetoKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))}, logger)
	require.NoError(t, err)
	assert.True(t, m.secure, "non-development cookies must use Secure")
}

func TestSessionCookieAttributes(t *testing.T) {
	client := openSessionTest(t)
	logger := log.Nop()

	dev, err := NewSessionManager(testSessionRepo(t, client), &config.Config{Mode: "development"}, logger)
	require.NoError(t, err)
	prod, err := NewSessionManager(testSessionRepo(t, client), &config.Config{
		Mode: "production", PasetoKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)),
	}, logger)
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

type recordingSessionRepo struct {
	creates  int
	cleanups int
}

func (r *recordingSessionRepo) CleanupExpired(context.Context, time.Time) error {
	r.cleanups++
	return nil
}

func (r *recordingSessionRepo) Create(context.Context, string, uuid.UUID, time.Time) error {
	r.creates++
	return nil
}

func (r *recordingSessionRepo) GetActive(context.Context, string, time.Time) (*SessionRecord, error) {
	return nil, nil
}

func (r *recordingSessionRepo) Delete(context.Context, string) error {
	return nil
}
