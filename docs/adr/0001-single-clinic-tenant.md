# ADR 0001 — Single-clinic tenant model

- Status: superseded by [ADR 0002](0002-multi-clinic-shared-schema.md)
- Date: 2026-08-12

## Context

LibreVita is a self-hosted EHR. The data model carries a `clinics`
table and `clinic_id` columns on the clinical tables (`patients`,
`specialties`), resolved per request through `ClockProvider.ClinicID`
(which loads the single clinic row, cached for 60s). Everything else
(`users`, `roles`, `policies`, `identifier_systems`, `audit_log`,
`storage_objects`, `staff_change_requests`, `meta`, `sessions`) is
global.

The question is what the tenant is: an installation, or a clinic that
shares an installation with other clinics?

## Decision

**The tenant is the installation. The product is single-clinic per
deployment.** `clinics` is the installation's profile (name,
timezone, address), not a multi-tenant boundary. `clinic_id` on
clinical tables is deliberate future-proofing: the resolution point
(`ClockProvider`) is already centralized, so a future multi-clinic
mode changes wiring, not the clinical schema.

Consequences:

- No UI, route, or API exists (or will be added without a product
  decision) to create a second clinic.
- `GetClinic` is a singleton by design (deterministic via
  `ORDER BY created_at LIMIT 1`).
- Policies, roles, identifier systems, and the audit log are global
  by design. The audit log has no `clinic_id`: every event belongs to
  the only clinic, and adding the column would require versioning the
  BLAKE2b chain payload (a migration cost with no consumer).
- The blind index of `patient_identifiers` is unique
  deployment-wide: a document belongs to exactly one patient
  anywhere, by product decision.
- The dqlite backend works end to end through the pure-Go wire
  protocol driver (github.com/canonical/go-dqlite/v3): real
  transactions (BEGIN/COMMIT replicated through Raft), prepared
  statements, strong consistency, and the embedded goose migrations
  run unchanged. The cluster itself is operated as a separate server
  process (a dqlite node binary built with CGO, outside the
  CGO-disabled application build).

## Scope guard

Every sqlc query touching a tenant table (`patients`, `specialties`)
must carry `clinic_id` in the SQL. `db/scope_test.go` enforces this:
add a new tenant table to `tenantTables` there and the guard starts
checking it. Queries reaching a tenant table through an
account-scoped association (`user_specialties`) are scoped by user id
and verified in the service layer (`SetUserSpecialties`).

## Path to multi-clinic (not executed, documented for the decision)

If multi-clinic becomes a product requirement:

1. `user_clinics` membership table; `users` stays global (a user may
   work in several clinics).
2. `Principal.ClinicID` (active clinic per session);
   `GetSessionUser` joins the membership.
3. `context.clinic_id` in policies — already available today via
   `PolicyEngine.SetClockProvider`.
4. `audit_log.clinic_id` with a versioned chain payload (v1/v2) and
   an index on `(clinic_id, id DESC)`.
5. Scope `staff.sql` queries by clinic; storage access stays bound
   through the clinic-scoped resource lookups.

None of this is done today; the product is single-clinic and the
global catalogs are correct for that model.
