package database

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/identifiersystem"
	"librevita.org/ent/role"
	"librevita.org/internal/core/config"
)

// SeedInitialData seeds default roles and identifier systems using Ent ORM.
func SeedInitialData(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}

	// 1. Seed Roles via Ent
	roles := []struct {
		ID         uuid.UUID
		Name       string
		System     bool
		IsClinical bool
	}{
		{uuid.MustParse("00000000-0000-7000-8000-000000000001"), "admin", true, false},
		{uuid.MustParse("00000000-0000-7000-8000-000000000002"), "physician", true, true},
		{uuid.MustParse("00000000-0000-7000-8000-000000000003"), "receptionist", true, false},
		{uuid.MustParse("00000000-0000-7000-8000-000000000004"), "patient", true, false},
	}

	for _, r := range roles {
		exists, err := client.Role.Query().Where(role.NameEQ(r.Name)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := client.Role.Create().
				SetID(r.ID).
				SetName(r.Name).
				SetSystem(r.System).
				SetIsClinical(r.IsClinical).
				Save(ctx); err != nil {
				return err
			}
		}
	}

	// 2. Seed Identifier Systems via Ent
	systems := []struct {
		ID               uuid.UUID
		System           string
		DisplayName      string
		Pattern          string
		Transform        identifiersystem.Transform
		CheckAlgorithm   identifiersystem.CheckAlgorithm
		CheckBaseLen     int
		CheckDvCount     int
		CheckStartWeight int
		Active           bool
		Mask             string
	}{
		{uuid.MustParse("00000000-0000-7000-8000-000000000011"), "urn:librevita:id:br:cpf", "CPF (Brasil)", "[0-9]{11}", identifiersystem.TransformDigits, identifiersystem.CheckAlgorithmMod11Desc, 9, 2, 10, true, "000.000.000-00"},
		{uuid.MustParse("00000000-0000-7000-8000-000000000012"), "urn:librevita:id:br:sus", "Cartão SUS (Brasil)", "[0-9]{15}", identifiersystem.TransformDigits, identifiersystem.CheckAlgorithmMod11Cyclic, 14, 1, 10, true, "000 0000 0000 0000"},
		{uuid.MustParse("00000000-0000-7000-8000-000000000013"), "urn:librevita:id:pt:nif", "NIF (Portugal)", "[0-9]{9}", identifiersystem.TransformDigits, identifiersystem.CheckAlgorithmMod11Desc, 8, 1, 9, true, "000 000 000"},
		{uuid.MustParse("00000000-0000-7000-8000-000000000014"), "urn:librevita:id:passport", "Passaporte", "[A-Z]{1,2}[0-9]{6,9}", identifiersystem.TransformUpper, identifiersystem.CheckAlgorithmNone, 0, 1, 10, true, ""},
	}

	for _, s := range systems {
		exists, err := client.IdentifierSystem.Query().Where(identifiersystem.SystemEQ(s.System)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := client.IdentifierSystem.Create().
				SetID(s.ID).
				SetSystem(s.System).
				SetDisplayName(s.DisplayName).
				SetPattern(s.Pattern).
				SetTransform(s.Transform).
				SetCheckAlgorithm(s.CheckAlgorithm).
				SetCheckBaseLen(s.CheckBaseLen).
				SetCheckDvCount(s.CheckDvCount).
				SetCheckStartWeight(s.CheckStartWeight).
				SetActive(s.Active).
				SetMask(s.Mask).
				Save(ctx); err != nil {
				return err
			}
		}
	}

	return nil
}

// EnsureAuditTriggers creates append-only triggers for the audit_log table.
func EnsureAuditTriggers(ctx context.Context, db *sql.DB, driver string) error {
	if db == nil {
		return nil
	}
	if driver == config.DriverPostgres {
		stmts := []string{
			`CREATE OR REPLACE FUNCTION audit_log_immutable()
			RETURNS trigger AS $$
			BEGIN
			    RAISE EXCEPTION 'audit_log is append-only';
			END;
			$$ LANGUAGE plpgsql;`,
			`DROP TRIGGER IF EXISTS audit_log_no_update ON "audit_log";
			CREATE TRIGGER audit_log_no_update
			BEFORE UPDATE ON "audit_log"
			FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();`,
			`DROP TRIGGER IF EXISTS audit_log_no_delete ON "audit_log";
			CREATE TRIGGER audit_log_no_delete
			BEFORE DELETE ON "audit_log"
			FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();`,
		}
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	}

	// SQLite
	stmts := []string{
		`CREATE TRIGGER IF NOT EXISTS audit_log_no_update
		BEFORE UPDATE ON audit_log
		BEGIN
		    SELECT RAISE(ABORT, 'audit_log is append-only');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS audit_log_no_delete
		BEFORE DELETE ON audit_log
		BEGIN
		    SELECT RAISE(ABORT, 'audit_log is append-only');
		END;`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
