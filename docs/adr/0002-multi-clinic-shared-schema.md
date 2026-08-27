# ADR 0002 — Multi-clinic isolation on a shared schema

- Status: accepted
- Date: 2026-08-24
- Supersedes: [ADR 0001](0001-single-clinic-tenant.md)

## Context

ADR 0001 treated the **installation** as the tenant: one process, one
database, one clinic row resolved with `Clinic.Query().First()`. Users,
roles, CEL policies, sessions, audit, and storage were global. A second
clinic in the same process would leak identity and PHI.

The product now hosts many clinics in that same process and schema.
Isolation must hold at runtime (queries, sessions, FLE keys) without
Postgres RLS or a database per clinic.

## Decision

**The isolation boundary is the clinic**, identified by Host
`{slug}.{base_domain}`. Nomenclature in schema, context, Principal, CEL,
and FLE is only `clinic_id` / `ClinicID`. There is no `tenant_id` column
and no `WithTenantID` alias.

### Routing

Accept only `base_domain`, `www.base_domain`, or `{slug}.base_domain`.
Unknown slug → 404 (the clinic must be provisioned first). Apex has no
clinic in context. Session and CSRF cookies are **host-only** (no
`Domain=.{base_domain}`).

### Identity

- `users.clinic_id` is NOT NULL; unique `(clinic_id, email)`. The same
  email in clinic B is another account.
- No `user_clinics` membership. Roles and CEL policies are copied per
  clinic at onboard (canonical names `admin`, `physician`,
  `receptionist`, `patient`).
- Platform operators live in `platform_users` (no `clinic_id`) and
  authenticate only on the apex via `platform_sessions`.
- Patient portal: `patients.user_id` (nullable, unique per clinic when
  set) plus role `patient`. Reception does not auto-create a login.

### Catalog vs instance

- `identifier_systems` stay global. Clinics opt in through
  `clinic_identifier_systems`.
- URNs: catalog `urn:librevita:id:br:cpf` (no clinic). Instance crypto
  `urn:librevita:clinic:<id>` and
  `urn:librevita:clinic:<id>:patient:<patient_id>`.
- Blind-index **domain** stays the catalog URN; isolation is the hasher
  **key** derived from the Clinic DEK.

### Crypto envelope

`LIBREVITA_MASTER_KEY` → installation KEK → Clinic DEK
(`urn:librevita:clinic:<id>`) → Patient DEK
(`urn:librevita:clinic:<id>:patient:<id>`) → PHI.

Patient, episode, appointment, identifier, and attachment PHI use the
Patient DEK. Blind indexes and clinic-owned data use a key derived from the
Clinic DEK, not the master. Patient-owned AAD is the Patient URN.
Crypto-shred of a patient deletes its Patient DEK; crypto-shred of a clinic
deletes its Clinic DEK and therefore makes all child Patient DEKs unreadable.
An operator with master **and** the vault can still unwrap any clinic.

### Onboarding

1. Apex bootstrap creates the first `platform_users` row, then
   provisions a clinic **shell** (`onboarded_at` null) and
   `EnsureClinicDEK`.
2. `{slug}.{base}/setup` seeds roles, DefaultPolicies, identifier
   opt-in, the first clinic admin, and sets `onboarded_at`.
   `SetupGate` uses `onboarded_at`, not `meta.setup_completed`.

### Query isolation

Ent privacy (or an equivalent interceptor) filters `clinic_id` from
context on User and every other clinic-scoped entity. Queries without
a clinic in context fail, except an explicit skip for migrate, seed,
and apex `platform_users` access.

## Consequences

- ClockProvider reads the clinic from request context (cache per id).
- PolicyEngine compiles and caches policies per `clinic_id`.
- `principal.clinic_id` and `principal.patient_id` are available in CEL.
- Existing single-clinic installs are backfilled onto one slug (from
  the name, fallback `default`) with `onboarded_at = created_at`.
- Wildcard DNS and TLS for `*.base_domain` are an operations
  requirement. Dev uses `LIBREVITA_BASE_DOMAIN=lv.test`.
