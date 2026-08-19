// Package testutil provides shared test seeds built on Ent ORM.
package testutil

import (
	"context"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/role"
)

// Clinic seeds a clinic row with the onboarding defaults (BR,
// America/Sao_Paulo) so callers only provide the identifying fields.
func Clinic(ctx context.Context, client *ent.Client, id, name, taxID string) error {
	create := client.Clinic.Create().
		SetID(uuid.MustParse(id)).
		SetName(name).
		SetCountry("BR").
		SetTimezone("America/Sao_Paulo")
	if taxID != "" {
		create.SetTaxID(taxID)
	}
	_, err := create.Save(ctx)
	return err
}

// User seeds an account with the given role name.
func User(ctx context.Context, client *ent.Client, id, email, roleName, passwordHash string) error {
	roleRow, err := client.Role.Query().Where(role.NameEQ(roleName)).Only(ctx)
	if err != nil {
		return err
	}
	_, err = client.User.Create().
		SetID(uuid.MustParse(id)).
		SetEmail(email).
		SetPasswordHash(passwordHash).
		SetDisplayName(email).
		SetRoleID(roleRow.ID).
		Save(ctx)
	return err
}
