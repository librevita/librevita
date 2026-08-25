// Package testutil provides shared test seeds built on Ent ORM.
package testutil

import (
	"context"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/role"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/database"
)

// SeedInitialData seeds default identifier systems using Ent ORM.
func SeedInitialData(ctx context.Context, client *ent.Client) error {
	return database.SeedInitialData(ctx, client)
}

func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "clinic"
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// Clinic seeds a clinic row with the onboarding defaults (BR,
// America/Sao_Paulo) so callers only provide the identifying fields.
func Clinic(ctx context.Context, client *ent.Client, id, name, taxID string) error {
	ctx = clinicctx.WithSkipIsolation(ctx)
	create := client.Clinic.Create().
		SetID(uuid.MustParse(id)).
		SetSlug(slugify(name)).
		SetName(name).
		SetCountry("BR").
		SetTimezone("America/Sao_Paulo")
	if taxID != "" {
		create.SetTaxID(taxID)
	}
	_, err := create.Save(ctx)
	if err != nil {
		return err
	}
	if err := database.SeedInitialData(ctx, client); err != nil {
		return err
	}
	systems, err := client.IdentifierSystem.Query().All(ctx)
	if err != nil {
		return err
	}
	clinicUUID := uuid.MustParse(id)
	for _, sys := range systems {
		if _, err := client.ClinicIdentifierSystem.Create().
			SetClinicID(clinicUUID).
			SetIdentifierSystemID(sys.ID).
			Save(ctx); err != nil && !ent.IsConstraintError(err) {
			return err
		}
	}
	return nil
}

// User seeds an account with the given role name in the first clinic
// (creating a test clinic and role when missing).
func User(ctx context.Context, client *ent.Client, id, email, roleName, passwordHash string) error {
	ctx = clinicctx.WithSkipIsolation(ctx)
	_ = database.SeedInitialData(ctx, client)

	row, err := client.Clinic.Query().First(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		if err := Clinic(ctx, client, clinicctx.TestClinicID.String(), "Test Clinic", ""); err != nil {
			return err
		}
		row, err = client.Clinic.Get(ctx, clinicctx.TestClinicID)
		if err != nil {
			return err
		}
	}

	roleRow, err := client.Role.Query().Where(role.NameEQ(roleName), role.ClinicIDEQ(row.ID)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			return err
		}
		roleRow, err = client.Role.Create().
			SetClinicID(row.ID).
			SetName(roleName).
			SetSystem(true).
			Save(ctx)
		if err != nil {
			return err
		}
	}
	_, err = client.User.Create().
		SetID(uuid.MustParse(id)).
		SetClinicID(row.ID).
		SetEmail(email).
		SetPasswordHash(passwordHash).
		SetDisplayName(email).
		SetRoleID(roleRow.ID).
		Save(ctx)
	return err
}
