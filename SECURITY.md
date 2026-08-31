# Security Policy

LibreVita is a sovereign electronic health record (EHR) and clinic management platform handling Protected Health Information (PHI). We treat security vulnerabilities with the highest priority and follow a responsible disclosure process.

## Supported Versions

Only the latest release and the current `master` branch receive security updates and patches.

| Version        | Supported          |
| -------------- | ------------------ |
| `master`       | :white_check_mark: |
| Latest release | :white_check_mark: |
| Older releases | :x:                |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

### Preferred Method: GitHub Private Vulnerability Reporting

1. Navigate to the **[Security](https://github.com/librevita/librevita/security)** tab of the repository.
2. Click **"Report a vulnerability"** under **Advisories**.
3. Fill out the advisory form with detailed information and submit.

### Alternative Method: Email

If you are unable to use GitHub Advisories, send an encrypted or direct email to:

- **`security@librevita.org`** (or contact the maintainers directly via `gustavo.veiga@outlook.com.br`)

### What to Include in Your Report

To help us triage and reproduce the issue quickly, please include:

- A clear description of the vulnerability and its potential impact.
- Affected component(s) (e.g., cryptographic envelope, blind indexing, CEL policy engine, multi-clinic isolation, HTTP surface, or audit log).
- Step-by-step reproduction instructions or a minimal Proof of Concept (PoC).
- Any proposed remediation or mitigation steps, if known.

## Response & Disclosure Process

1. **Acknowledgment:** We will acknowledge receipt of your vulnerability report within **48 hours**.
2. **Triage & Investigation:** We will confirm the issue, determine severity, and evaluate the attack surface.
3. **Fix & Verification:** A fix will be developed in a private security fork, reviewed, and tested against our validation gates (`task all`).
4. **Release & Advisory:** A coordinated security advisory (CVE if applicable) will be published alongside a patched release.
5. **Credit:** Reporters who practice responsible disclosure will be acknowledged in release notes and security advisories (unless you prefer to remain anonymous).

## Scope & Security Boundaries

We particularly appreciate research in the following core areas:

- **Application-Layer Field-Level Encryption (AL-FLE):** Failures in XChaCha20-Poly1305 encryption, envelope wrapping (KEK $\to$ DEK), or plaintext leakage into logs, storage, or SQLite/Postgres.
- **Blind Indexing & Search:** Information leakage in keyed blind indexes or tokenized n-gram search across clinics.
- **Multi-Clinic Isolation:** Cross-clinic data leakage, tenant bypass, or session confusion across distinct Host domains.
- **CEL Access Policies:** Logic bypasses in Common Expression Language policies, role escalation, or unauthorized access to clinical/administrative endpoints.
- **Audit Log Chain:** Attacks that break or forge the hash-chained `audit_log` without triggering detection via `GET /audit/integrity`.

### Out of Scope

- Attacks requiring physical/root-level machine compromise where process memory or `LIBREVITA_MASTER_KEY` is already extracted.
- Social engineering or phishing targeting operators or users.
- Denial of Service (DoS) attacks that rely solely on raw network flooding.
