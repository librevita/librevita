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
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/testutil"
)

const testUserID = "01990000-0000-7000-8000-000000000001"

func openSessionTest(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:session-test-"+uuid.NewString()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	log := slog.New(slog.DiscardHandler)
	if err := database.Migrate(context.Background(), db, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	return client
}

func seedUser(t *testing.T, client *ent.Client, id string) {
	t.Helper()
	hash, err := HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.User(context.Background(), client, id, "user@example.org", "admin", hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func newManager(t *testing.T, client *ent.Client, ttl time.Duration) *SessionManager {
	t.Helper()
	m := &SessionManager{
		repo:   NewSessionRepository(client),
		ttl:    ttl,
		secure: false,
		log:    slog.New(slog.DiscardHandler),
	}
	return m
}

func TestSessionLifecycle(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("Create returned an empty token")
	}

	p, err := m.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.ID != testUserID || p.Role != RoleAdmin || p.Email != "user@example.org" {
		t.Fatalf("unexpected principal: %+v", p)
	}

	if err := m.Destroy(context.Background(), token); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := m.Authenticate(context.Background(), token); err != ErrNoSession {
		t.Fatalf("Authenticate after destroy = %v, want ErrNoSession", err)
	}
}

func TestSessionUnknownToken(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	if _, err := m.Authenticate(context.Background(), "bogus"); err != ErrNoSession {
		t.Fatalf("Authenticate = %v, want ErrNoSession", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, -time.Hour) // Already expired.

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := m.Authenticate(context.Background(), token); err != ErrNoSession {
		t.Fatalf("Authenticate of expired session = %v, want ErrNoSession", err)
	}
}

func TestSessionRejectsDeactivatedUser(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)

	userUUID := uuid.MustParse(testUserID)
	if err := client.User.UpdateOneID(userUUID).SetActive(false).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := m.Authenticate(context.Background(), token); err != ErrNoSession {
		t.Fatalf("Authenticate of deactivated user = %v, want ErrNoSession", err)
	}
}

func TestSessionManagerRequiresClient(t *testing.T) {
	if _, err := NewSessionManager(nil, &config.Config{Mode: "development"}, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("NewSessionManager(nil) should fail")
	}
}

func TestSessionManagerRequiresKeyInProduction(t *testing.T) {
	client := openSessionTest(t)
	cfg := &config.Config{Mode: "production"}
	if _, err := NewSessionManager(NewSessionRepository(client), cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("NewSessionManager without a key in production should fail")
	}
}

func TestSessionManagerRejectsMalformedKey(t *testing.T) {
	client := openSessionTest(t)
	for _, key := range []string{"not-base64!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		cfg := &config.Config{Mode: "production", PasetoKey: key}
		if _, err := NewSessionManager(NewSessionRepository(client), cfg, slog.New(slog.DiscardHandler)); err == nil {
			t.Fatalf("NewSessionManager with key %q should fail", key)
		}
	}
}

func TestSessionManagerAcceptsConfiguredKey(t *testing.T) {
	client := openSessionTest(t)
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	m, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "production", PasetoKey: key}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	seedUser(t, client, testUserID)
	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := m.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
}

func TestSessionRejectsTamperedToken(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tampered := token[:len(token)-1] + string(flipByte(token[len(token)-1]))
	if _, err := m.Authenticate(context.Background(), tampered); err != ErrNoSession {
		t.Fatalf("Authenticate of tampered token = %v, want ErrNoSession", err)
	}
}

func TestSessionTokenIsPaseto(t *testing.T) {
	client := openSessionTest(t)
	seedUser(t, client, testUserID)
	m := newManager(t, client, time.Hour)

	token, err := m.Create(context.Background(), Principal{ID: testUserID, Email: "user@example.org", Name: "Test User", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(token, "v4.local.") {
		t.Fatalf("token %q does not use PASETO v4.local", token)
	}
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
		if _, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: env}, log); err == nil {
			t.Fatalf("env %q must require a paseto key", env)
		}
	}

	m, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "development"}, log)
	if err != nil {
		t.Fatalf("development env must allow an ephemeral key: %v", err)
	}
	if m.secure {
		t.Fatal("development cookies must not use Secure")
	}

	m, err = NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "staging", PasetoKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))}, log)
	if err != nil {
		t.Fatalf("staging with a key must work: %v", err)
	}
	if !m.secure {
		t.Fatal("non-development cookies must use Secure")
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	client := openSessionTest(t)
	log := slog.New(slog.DiscardHandler)

	dev, err := NewSessionManager(NewSessionRepository(client), &config.Config{Mode: "development"}, log)
	if err != nil {
		t.Fatal(err)
	}
	prod, err := NewSessionManager(NewSessionRepository(client), &config.Config{
		Mode: "production", PasetoKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)),
	}, log)
	if err != nil {
		t.Fatal(err)
	}

	for name, m := range map[string]*SessionManager{"development": dev, "production": prod} {
		for _, c := range []*http.Cookie{m.Cookie("token"), m.ClearCookie()} {
			if !c.HttpOnly {
				t.Fatalf("%s: session cookie must be HttpOnly", name)
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Fatalf("%s: session cookie must be SameSite=Lax", name)
			}
			wantSecure := name == "production"
			if c.Secure != wantSecure {
				t.Fatalf("%s: Secure = %v, want %v", name, c.Secure, wantSecure)
			}
		}
	}
}
