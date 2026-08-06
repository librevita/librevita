package usecase_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/domain/user/usecase"
)

func openAuthDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:auth-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newService(t *testing.T, db *sql.DB) *usecase.Service {
	t.Helper()
	svc, _ := newServiceWithSessions(t, db)
	return svc
}

func newServiceWithSessions(t *testing.T, db *sql.DB) (*usecase.Service, *auth.SessionManager) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "test"}, log)
	if err != nil {
		t.Fatal(err)
	}
	auditLogger, err := audit.NewLogger(db, log)
	if err != nil {
		t.Fatal(err)
	}
	return usecase.NewService(db, sessions, auditLogger, log), sessions
}

func validInput() usecase.RegisterInput {
	return usecase.RegisterInput{
		Name:     "Ana Souza",
		Email:    "ana@example.org",
		Password: "password-123",
	}
}

func TestRegisterFirstUserBecomesAdmin(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	p, token, err := svc.Register(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if p.Role != auth.RoleAdmin {
		t.Fatalf("first user role = %q, want admin", p.Role)
	}
	if p.Email != "ana@example.org" {
		t.Fatalf("principal email = %q", p.Email)
	}
	if token == "" {
		t.Fatal("Register returned an empty session token")
	}
}

func TestRegisterSecondUserBecomesPatient(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	if _, _, err := svc.Register(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}

	input := validInput()
	input.Email = "bruno@example.org"
	p, _, err := svc.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if p.Role != auth.RolePatient {
		t.Fatalf("second user role = %q, want patient", p.Role)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	if _, _, err := svc.Register(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}
	input := validInput()
	input.Name = "Another Ana"
	if _, _, err := svc.Register(context.Background(), input); !errors.Is(err, usecase.ErrEmailTaken) {
		t.Fatalf("duplicate register = %v, want ErrEmailTaken", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	cases := []struct {
		name  string
		mut   func(*usecase.RegisterInput)
		error string
	}{
		{"missing name", func(i *usecase.RegisterInput) { i.Name = " " }, "display name"},
		{"bad email", func(i *usecase.RegisterInput) { i.Email = "not-an-email" }, "valid email"},
		{"short password", func(i *usecase.RegisterInput) { i.Password = "short" }, "at least 8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validInput()
			tc.mut(&input)
			_, _, err := svc.Register(context.Background(), input)
			var v *usecase.ValidationError
			if !errors.As(err, &v) {
				t.Fatalf("Register = %v, want ValidationError", err)
			}
		})
	}
}

func TestLoginSuccessAndFailure(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	if _, _, err := svc.Register(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}

	p, token, err := svc.Login(context.Background(), usecase.Credentials{Email: "ANA@example.org", Password: "password-123"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if p.ID == 0 || token == "" {
		t.Fatalf("Login returned incomplete result: %+v", p)
	}

	for _, creds := range []usecase.Credentials{
		{Email: "ana@example.org", Password: "wrong"},
		{Email: "ghost@example.org", Password: "password-123"},
	} {
		if _, _, err := svc.Login(context.Background(), creds); !errors.Is(err, usecase.ErrInvalidCredentials) {
			t.Fatalf("Login(%+v) = %v, want ErrInvalidCredentials", creds, err)
		}
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	db := openAuthDB(t)
	svc, sessions := newServiceWithSessions(t, db)

	_, token, err := svc.Register(context.Background(), validInput())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sessions.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("session should be valid: %v", err)
	}

	if err := svc.Logout(context.Background(), token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := sessions.Authenticate(context.Background(), token); err != auth.ErrNoSession {
		t.Fatalf("session after logout = %v, want ErrNoSession", err)
	}
}

func TestConcurrentRegistrationYieldsSingleAdmin(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	const users = 8
	var wg sync.WaitGroup
	errs := make([]error, users)
	for i := range users {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := validInput()
			input.Email = "user" + string(rune('a'+i)) + "@example.org"
			_, _, errs[i] = svc.Register(context.Background(), input)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("register %d failed: %v", i, err)
		}
	}

	var admins int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if admins != 1 {
		t.Fatalf("concurrent bootstrap produced %d admins, want exactly 1", admins)
	}
}

func TestDuplicateEmailMapsToDomainError(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	if _, _, err := svc.Register(context.Background(), validInput()); err != nil {
		t.Fatal(err)
	}

	input := validInput()
	input.Email = "ANA@example.org" // NOCASE duplicate
	if _, _, err := svc.Register(context.Background(), input); !errors.Is(err, usecase.ErrEmailTaken) {
		t.Fatalf("duplicate register = %v, want ErrEmailTaken", err)
	}
}
