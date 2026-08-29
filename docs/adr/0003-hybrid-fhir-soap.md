# ADR 0003 — Hybrid FHIR R4 SOAP chart

- Status: accepted
- Date: 2026-08-28
- Relates to: [ADR 0002](0002-multi-clinic-shared-schema.md)

## Context

LibreVita needs a structured clinical chart (SOAP: Subjective, Objective,
Assessment, Plan) that clinicians reason about in domain language, while
interoperability clients speak FHIR R4. Two rejected alternatives:

1. **Persist FHIR JSON documents** as the source of truth. Application-layer
   field-level encryption (Patient DEK) would wrap opaque blobs, search and
   clinic isolation would fight a document store, and the core would be
   coupled to one wire version (R4 today, R5 tomorrow).
2. **Use FHIR resource types inside `model/`**. The domain would leak
   `Composition.section`, `CodeableConcept`, and Bundle entry order into
   use cases, tests, and Ent schemas.

The chart must remain encrypted with the Patient DEK (ADR 0002), isolated by
`clinic_id`, and shreddable with the patient aggregate.

## Decision

**FHIR R4 on the wire, domain in the core.** Mapping is a replaceable
communication module, not part of the SOAP bounded context.

- The aggregate root is `Episode`: a SOAP note of one encounter, with
  narrative sections plus structured children (`Finding`, `Problem`,
  `PlanItem`). Persistence is Ent tables, not a FHIR resource graph.
- `internal/domain/episode` must not import `internal/interop`. The
  adapter imports the domain.
- FHIR R4 lives in `internal/interop/fhir` (`package fhir`): JSON DTOs
  (`Bundle`, `Encounter`, `Composition`, `Observation`, `Condition`,
  `ClinicalImpression`, `CarePlan`, `OperationOutcome`,
  `CapabilityStatement`) plus mapper and HTTP. DTO files do not use
  domain types; only mapper/handler import `episode`.
- `cmd/web` wires `episode.Module` (repository + use case) beside
  `fhir.Module` (routes `/fhir/r4/*`). A future R5 is a sibling module
  (for example `internal/interop/fhir5`) and another line in `cmd/web`;
  the Episode aggregate stays intact.
- Observation, Condition, and CarePlan are **not independently addressable**.
  Write path is `POST /fhir/r4/Bundle` (SOAP document, not a FHIR
  `POST [base]` transaction). Create returns 201 + `Location`; update and
  finalize return 200. Read path is `Composition/{id}/$document` plus
  Encounter search by patient. CapabilityStatement lists only those
  interactions.
- A `finalized` episode is immutable. Amendment is a **linear**
  `replaces` chain: `predecessor_id` is unique, so each note has at most
  one successor. `Amend` returns the open successor draft when one
  exists; after that successor is finalized, the next amend is of the
  successor, not the original. On the wire this is
  `Composition.relatesTo` code `replaces`.
- FHIR HTTP uses the clinic session cookie. The interop module registers
  CSRF and global body-limit skippers for `/fhir/r4`; `internal/core/server`
  has no FHIR paths. Clients send `application/fhir+json` without a form
  token. SameSite=Lax cookies and the absence of CORS keep cookie POSTs
  same-origin. Errors on these routes are `OperationOutcome`, not RFC 7807.
  Successful writes record `chart.create` / `chart.update` / `chart.finalize`;
  `$document`, Encounter read, and Encounter search record `chart.view`.
- Clinical codes (`findings.code`, `problems.code`) use
  `fle.SearchableDocument` so exact-match lookup is a keyed blind index,
  the same pipeline as identification documents. SOAP narratives are not
  token-indexed in this slice.
- Finding, Problem, and PlanItem enums are clinical tokens (`recorded`,
  `encounter`, `exam`, …). FHIR R4 value sets (`Observation.status`,
  `Condition.clinicalStatus`, CarePlan activity `kind`) are mapped only
  in `internal/interop/fhir`. Visit setting is `CareSetting` (stored as
  `episodes.class`); ActCode `AMB`/`EMER`/`IMP`/`VR` is mapper-only.

## Consequences

- Adding R5 or another profile is a new interop module, not a schema rewrite
  and not a change to `episode.Module`.
- Coded findings and CID-10 problems are encrypted; exact-match search goes
  through the generated `code_blind_index` (clinic-scoped). Tokenized n-gram
  search on SOAP narratives waits for a chart search UI.
- This is not a general-purpose FHIR server: no `$everything`, history,
  PATCH, SMART-on-FHIR, or MedicationRequest/ServiceRequest in this slice.
- Domain CHECKs stay on clinical tokens. Expanding a FHIR value set is a
  mapper change, not a Goose CHECK rewrite.
