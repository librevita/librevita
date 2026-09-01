# ADR 0005 — Key scope in ciphertext and hashes

- Status: accepted
- Date: 2026-09-01
- Relates to: [ADR 0002](0002-multi-clinic-shared-schema.md)

## Context

Ciphertexts identified the AEAD with a magic byte (`0x01` =
XChaCha20-Poly1305). Keyed hashes identified the digest with a `$`
prefix (`blake2s$…`). Wrapped DEKs already stamped a scope byte
(clinic vs patient). Data blobs did not, so a KEK encryptor, a clinic
DEK encryptor, and a patient DEK encryptor produced indistinguishable
envelopes. The same held for the master-derived hasher versus a clinic
hasher.

Keyed hashes at the same tier can still use different key material
(HKDF blind-index vs PASETO session key). Scope alone does not
distinguish those purposes.

Which key to use was inferred from request context and the row's
`clinic_id` / `patient_id`. That is enough to *select* a DEK (the URN
is still the keystore key and the AEAD AAD) but not enough to *refuse*
the wrong tier before `Open`, or to tell a dump of a single column
which hierarchy step sealed it.

## Decision

Stamp the **key tier** and **key generation** in-band. Do not stamp the URN
(identity stays on the row, in AAD, and in the keystore key).

Ciphertext envelope:

```
[ magic | key_scope | kid | nonce | ciphertext ‖ tag ]
```

`0x01` remains XChaCha20-Poly1305. `key_scope` is the ASCII byte
`m` (master/KEK), `c` (clinic DEK), or `p` (patient DEK) — the same
letters as the hash token. Byte `0` is invalid. `kid` is the
key generation (`DefaultKeyID` = `1`; `0` is invalid). The three-byte
header is authenticated as AAD together with the caller AAD, matching
wrapped DEKs. Decrypt fails closed (`ErrKeyScopeMismatch` /
`ErrKeyIDMismatch`) when the blob's scope or kid does not match the
Encryptor.

Wrapped DEKs: `[ 0xD1 | version | scope | kid | nonce | wrapped DEK ]`.
LVFE: `LVFE | version | patient scope | kid | nonce`.

Keyed hash:

```
<algorithm>$<scope><purpose>$<kid>$<hex>
```

`m` / `c` / `p` then `i` / `s` in one two-character field (`mi` master
index, `ci` clinic index, `ms` session). `i` is HKDF `InfoBlindIndex`
(global or clinic-derived). `s` is the PASETO session fingerprint key
and is master-scoped only. `kid` is two lowercase hex digits (`01`).
`Verify` requires all four fields and matching scope, purpose, *and*
kid. Ciphertext, wrapped DEKs, and LVFE do **not** carry purpose: those
formats already separate wrap, FLE, and attachments.

Constructors do not take a free scope, purpose, or kid.
`NewMasterEncryptor` / `NewClinicEncryptor` / `NewPatientEncryptor`
stamp the matching hierarchy letter and `DefaultKeyID`.
`NewMasterIndexHasher` / `NewClinicIndexHasher` (`NewHasherFromDEK`)
always HKDF-derive `InfoBlindIndex` and stamp `i`; `NewSessionHasher`
always stamps master + `s` on the PASETO key. Engine wrap/PHI paths
use the unexported constructors with `e.kid` so a future keyring can
open retired generations without exposing `(key, scope, kid)` as
independent arguments.
`NewHasherFromConfig` / `NewEncryptorFromConfig`
stamp `DefaultKeyID`. Patient PHI encryptors are patient-scoped.
Identifier values and `Seal`/`Open` go through the same Encryptor
envelope (the identifier `nonce` column is removed). Attachment files
(`LVFE`) carry `key_scope` and `kid` after the version byte (patient).

There is no dual-read of the previous formats.

## Consequences

- A blob answers "KEK, clinic DEK, or patient DEK?" without the row,
  and which generation sealed it (`kid`). Selecting *which* clinic or
  patient DEK still uses the URN.
- A keyed hash also answers "blind index or session fingerprint?" and
  the hasher generation. Wrong-purpose / wrong-kid verify is
  `ErrKeyPurposeMismatch` / `ErrKeyIDMismatch`, not a digest miss.
- Wrong-tier decrypt is a scope error, not a generic AEAD failure.
- Blind indexes are `blake2s$ci$01$` + 64 hex = 78; identifier
  `MaxLen` is 88.
- Session fingerprints include `$ms$01`; existing dev sessions are
  invalid after upgrade. `hashToken` has no unprefixed hex fallback.
- `kid` is not a keyring yet: the process seals and opens with
  `DefaultKeyID` only. Rotation is a later lookup of retired ids.
