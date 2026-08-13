// Package testutil provides shared test seeds built on the typed sqlc
// repositories, so tests exercise the real persistence path instead of
// hand-written SQL.
package testutil

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	clinicrepo "librevita.org/internal/domain/clinic/repository"
	userrepo "librevita.org/internal/domain/user/repository"
)

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Clinic seeds a clinic row with the onboarding defaults (BR,
// America/Sao_Paulo) so callers only provide the identifying fields.
func Clinic(ctx context.Context, db *sql.DB, id, name, taxID string) error {
	_, err := clinicrepo.New(db).CreateClinic(ctx, clinicrepo.CreateClinicParams{
		ID:       uuid.MustParse(id),
		Name:     name,
		TaxID:    strPtr(taxID),
		Country:  "BR",
		Timezone: "America/Sao_Paulo",
	})
	return err
}

// User seeds an account with the given role name. The password hash is
// the caller's responsibility: authentication tests need a real
// Argon2id hash (via auth.HashPassword), the rest use a placeholder.
func User(ctx context.Context, db *sql.DB, id, email, role, passwordHash string) error {
	roleRow, err := userrepo.New(db).GetRoleByName(ctx, role)
	if err != nil {
		return err
	}
	_, err = userrepo.New(db).CreateUser(ctx, userrepo.CreateUserParams{
		ID:           uuid.MustParse(id),
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  email,
		RoleID:       roleRow.ID,
	})
	return err
}
