package usecase_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/google/uuid"

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
	sessions, err := auth.NewSessionManager(db, &config.Config{Env: "development"}, log)
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

func TestRegisterCreatesPatient(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	p, token, err := svc.Register(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if p.Role != auth.RolePatient {
		t.Fatalf("registered user role = %q, want patient", p.Role)
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
	if p.ID == "" || token == "" {
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

func TestConcurrentRegistrationCreatesNoAdmins(t *testing.T) {
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
			input.Email = "user" + string(rune('a'+i)) + "@example.org" // #nosec G115 -- bounded loop counter
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM users u JOIN roles r ON r.id = u.role_id WHERE r.name = 'admin'`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if admins != 0 {
		t.Fatalf("public registration produced %d admins, want 0", admins)
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

func TestRegisterUsesUUIDv7ID(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	p, _, err := svc.Register(context.Background(), validInput())
	if err != nil {
		t.Fatal(err)
	}

	id, err := uuid.Parse(p.ID)
	if err != nil {
		t.Fatalf("principal id %q is not a UUID: %v", p.ID, err)
	}
	if id.Version() != 7 {
		t.Fatalf("principal id %q is not UUIDv7 (version %d)", p.ID, id.Version())
	}
}

func validClinicInput() usecase.ClinicInput {
	return usecase.ClinicInput{
		Name:    "Clínica Exemplo",
		TaxID:   "12.345.678/0001-90",
		City:    "São Paulo",
		State:   "SP",
		Country: "BR",
	}
}

func TestIsOnboarded(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	ok, err := svc.IsOnboarded(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("empty system must not be onboarded")
	}

	if _, _, err := svc.Onboard(context.Background(), validInput(), validClinicInput()); err != nil {
		t.Fatal(err)
	}
	ok, err = svc.IsOnboarded(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("system with an admin must be onboarded")
	}
}

func TestOnboardCreatesAdminAndClinic(t *testing.T) {
	db := openAuthDB(t)
	svc, sessions := newServiceWithSessions(t, db)

	p, token, err := svc.Onboard(context.Background(), validInput(), validClinicInput())
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if p.Role != auth.RoleAdmin {
		t.Fatalf("onboarded role = %q, want admin", p.Role)
	}

	if _, err := sessions.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("onboard session invalid: %v", err)
	}

	var clinicName string
	if err := db.QueryRow(`SELECT name FROM clinics`).Scan(&clinicName); err != nil {
		t.Fatal(err)
	}
	if clinicName != "Clínica Exemplo" {
		t.Fatalf("clinic name = %q", clinicName)
	}

	var admins int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users u JOIN roles r ON r.id = u.role_id WHERE r.name = 'admin'`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if admins != 1 {
		t.Fatalf("admins = %d, want 1", admins)
	}
}

func TestOnboardFailsWhenAlreadyOnboarded(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	if _, _, err := svc.Onboard(context.Background(), validInput(), validClinicInput()); err != nil {
		t.Fatal(err)
	}

	input := validInput()
	input.Email = "outro@example.org"
	if _, _, err := svc.Onboard(context.Background(), input, validClinicInput()); !errors.Is(err, usecase.ErrAlreadyOnboarded) {
		t.Fatalf("second Onboard = %v, want ErrAlreadyOnboarded", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("users = %d, want 1 (failed onboard must not create users)", count)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM clinics`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("clinics = %d, want 1", count)
	}
}

func TestConcurrentOnboardSingleWinner(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	const attempts = 6
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := validInput()
			input.Email = "admin" + string(rune('a'+i)) + "@example.org" // #nosec G115 -- bounded loop counter
			_, _, errs[i] = svc.Onboard(context.Background(), input, validClinicInput())
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, usecase.ErrAlreadyOnboarded) {
			t.Fatalf("Onboard = %v, want success or ErrAlreadyOnboarded", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent onboarding succeeded %d times, want exactly 1", successes)
	}

	var admins int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users u JOIN roles r ON r.id = u.role_id WHERE r.name = 'admin'`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if admins != 1 {
		t.Fatalf("admins = %d, want 1", admins)
	}
}

func TestOnboardValidation(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	cases := []struct {
		name    string
		mutate  func(*usecase.RegisterInput, *usecase.ClinicInput)
		message string
	}{
		{"missing admin name", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { a.Name = " " }, "display name"},
		{"short password", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { a.Password = "short" }, "at least 8"},
		{"missing clinic name", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { c.Name = "" }, "clinic name"},
		{"bad clinic email", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { c.Email = "not-an-email" }, "clinic email"},
		{"invalid timezone", func(a *usecase.RegisterInput, c *usecase.ClinicInput) { c.Timezone = "Mars/Olympus" }, "from the list"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			admin := validInput()
			clinic := validClinicInput()
			tc.mutate(&admin, &clinic)
			_, _, err := svc.Onboard(context.Background(), admin, clinic)
			var v *usecase.ValidationError
			if !errors.As(err, &v) {
				t.Fatalf("Onboard = %v, want ValidationError", err)
			}
		})
	}
}

func TestSetupCannotBeReexecutedAfterDataRemoval(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	if _, _, err := svc.Onboard(context.Background(), validInput(), validClinicInput()); err != nil {
		t.Fatal(err)
	}

	// Even with every account and the clinic deleted, the persisted
	// marker keeps the system onboarded and setup remains impossible.
	if _, err := db.Exec(`DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM clinics`); err != nil {
		t.Fatal(err)
	}

	ok, err := svc.IsOnboarded(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("system must stay onboarded after data removal")
	}

	if _, _, err := svc.Onboard(context.Background(), validInput(), validClinicInput()); !errors.Is(err, usecase.ErrAlreadyOnboarded) {
		t.Fatalf("Onboard after data removal = %v, want ErrAlreadyOnboarded", err)
	}
}

func TestSetupMarkerGuardsDeletedMarkerEdgeCase(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)

	if _, _, err := svc.Onboard(context.Background(), validInput(), validClinicInput()); err != nil {
		t.Fatal(err)
	}

	// If the marker row itself is deleted but accounts remain, the user
	// count keeps the system onboarded.
	if _, err := db.Exec(`DELETE FROM meta`); err != nil {
		t.Fatal(err)
	}

	ok, err := svc.IsOnboarded(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("user count must keep the system onboarded when the marker is missing")
	}

	if _, _, err := svc.Onboard(context.Background(), validInput(), validClinicInput()); !errors.Is(err, usecase.ErrAlreadyOnboarded) {
		t.Fatalf("Onboard with deleted marker = %v, want ErrAlreadyOnboarded", err)
	}
}
