# ADR 0004 — Partitioned KV: keystore, meta, sessions

- Status: accepted
- Date: 2026-08-31
- Relates to: [ADR 0002](0002-multi-clinic-shared-schema.md)

## Context

Wrapped Clinic and Patient DEKs already lived outside SQL (bbolt, NATS,
etcd, or HashiCorp Vault / OpenBao). Installation metadata (`meta`) and
PASETO revocation rows (`sessions`, `platform_sessions`) still sat in
the clinical database. That mixed three access patterns in one engine:

- **keystore** — low-churn secrets, crypto-shred tombstones, optional
  Vault KV v2 hard-delete.
- **meta** — installation flags, not secrets, not high QPS.
- **sessions** — Get on every authenticated request plus list-and-delete
  of expired fingerprints.

Vault/OpenBao is a secrets engine. Session QPS, latency, and `LIST` are
a poor fit. Meta is not a secret. One shared `vault` config block and
the `hashicorp` backend alias also made the DEK store look like “the
Vault product” rather than a keystore that _may_ use Vault.

## Decision

Three **independent** KV stores, each with its own `backend` and file /
bucket / prefix. They may be the same engine type (three bbolt files) or
mixed (keystore in Vault, sessions in bbolt).

| Store        | Role              | Backends                          | Default bbolt            | Default NATS bucket | Default etcd prefix    | Default Vault path    |
| ------------ | ----------------- | --------------------------------- | ------------------------ | ------------------- | ---------------------- | --------------------- |
| **keystore** | Wrapped DEKs      | `bbolt` `nats` `etcd` **`vault`** | `<data-dir>/keystore.db` | `keystore`          | `/librevita/keystore/` | `librevita/keystore/` |
| **meta**     | Installation KV   | `bbolt` `nats` `etcd`             | `<data-dir>/meta.db`     | `meta`              | `/librevita/meta/`     | —                     |
| **sessions** | PASETO revocation | `bbolt` `nats` `etcd`             | `<data-dir>/sessions.db` | `sessions`          | `/librevita/sessions/` | —                     |

`backend: vault` (HashiCorp Vault / OpenBao KV v2) is valid **only** on
keystore. OpenBao uses the same adapter via `keystore.vault.address`.
Meta and sessions reject Vault at config validation and at `kv.Open`.

Generic adapters live in `internal/core/kv`. The DEK port is
`crypto.KeyStore` (was `KeyVault`); `internal/core/keystore` wraps the
keystore `Store` with shredding tombstones. Meta and sessions `Delete`
for real.

Logical URNs:

- DEKs unchanged: `urn:librevita:clinic:<id>`, `…:patient:<id>`
- meta: `urn:librevita:meta:<key>`
- clinic session: `urn:librevita:clinic:<clinic_uuid>:session:<token_hash>`
- apex session: `urn:librevita:platform:session:<token_hash>`

`token_hash` remains the keyed BLAKE2 fingerprint of the PASETO jti.
Session `GetActive` loads the user from SQL. The SQL tables `meta`,
`sessions`, and `platform_sessions` are dropped. There is no migration
of old `keys.db`, `patient_deks`, `/librevita/keys/`, `librevita/deks/`,
or the `hashicorp` / `LIBREVITA_VAULT_*` aliases.

## Consequences

- Operators configure three blocks (`keystore`, `meta`, `sessions`) and
  repeat NATS/etcd URLs when they share a server.
- A stolen clinical database no longer contains session fingerprints or
  installation KV; those files / buckets must be backed up with the
  keystore, `master_key`, and attachments.
- Native TTLs (etcd lease, NATS KV TTL) are out of scope; expiry is
  still application-side `expires_at` plus `CleanupExpired`.
