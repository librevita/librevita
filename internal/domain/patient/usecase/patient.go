// Package usecase holds the patient domain service: validation and
// persistence of patient records with application-layer field encryption.
package usecase

import (
	"context"
	"strings"
	"unicode"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	normalizer "librevita.org/internal/core/normalize"
	"librevita.org/internal/core/policy"
	patientmodel "librevita.org/internal/domain/patient/model"
	"librevita.org/pkg/errors"
	"librevita.org/pkg/flow"
	"librevita.org/pkg/ident"
	"librevita.org/pkg/validator"
)

const (
	maxPatientNameLen = 120
	maxPhoneLen       = 20
	maxEmailLen       = 120
	maxAddressLen     = 120
	maxNotesLen       = 4000
)

// ValidationError is returned for invalid patient input.
type ValidationError = validator.ValidationError

// Re-export domain models and contracts from patient/model.
type (
	Patient                  = patientmodel.Patient
	GetPatientWithCreatorRow = patientmodel.GetPatientWithCreatorRow
	PatientInput             = patientmodel.PatientInput
	PatientRepository        = patientmodel.PatientRepository
)

var (
	ErrNotFound  = patientmodel.ErrNotFound
	ErrForbidden = patientmodel.ErrForbidden
)

// Service persists and validates patient records.
type Service struct {
	repo     PatientRepository
	policies *policy.PolicyEngine
	engine   *crypto.Engine
}

// NewService is the Fx provider.
func NewService(repo PatientRepository, policies *policy.PolicyEngine, engine *crypto.Engine) *Service {
	return &Service{
		repo:     repo,
		policies: policies,
		engine:   engine,
	}
}

// AuthorizePatientEdit evaluates the fine-grained patient.edit policy.
func (s *Service) AuthorizePatientEdit(ctx context.Context, principal *auth.Principal, id string, createdBy *string, status patientmodel.PatientStatus) error {
	resource := map[string]any{
		"id":         id,
		"created_by": orEmpty(createdBy),
		"status":     status.String(),
	}
	allowed, err := s.policies.AllowedResource(ctx, "patient.edit", principal,
		policy.RequestInfo{Method: "POST", Path: "/patients/" + id}, resource, nil)
	if err != nil {
		return errors.Wrap(err, "usecase: authorize patient edit")
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// Create validates in and inserts a patient domain entity.
func (s *Service) Create(ctx context.Context, clinicID, createdBy string, in PatientInput) (*Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	patientID := ident.New[ident.PatientID]()
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid clinic id")
	}

	var cb *ident.UserID
	if createdBy != "" {
		parsed, perr := ident.ParseUser(createdBy)
		if perr != nil {
			return nil, errors.Wrap(perr, "usecase: invalid created_by")
		}
		cb = &parsed
	}

	p := Patient{
		ID:          patientID,
		ClinicID:    cUUID,
		DisplayName: normalized.DisplayName,
		BirthDate:   strPtr(normalized.BirthDate),
		Sex:         normalized.Sex,
		Phone:       strPtr(normalized.Phone),
		Email:       strPtr(normalized.Email),
		Street:      strPtr(normalized.Street),
		City:        strPtr(normalized.City),
		State:       strPtr(normalized.State),
		PostalCode:  strPtr(normalized.PostalCode),
		Notes:       strPtr(normalized.Notes),
		Status:      patientmodel.PatientStatusActive,
		CreatedBy:   cb,
	}

	var created *Patient
	err = flow.New().
		StepWithRollback("provision patient dek", func() error {
			if s.engine != nil {
				_, err := s.engine.EnsurePatientDEKForClinic(ctx, cUUID, patientID)
				if err != nil {
					return errors.Wrap(err, "usecase: provision patient dek")
				}
			}
			return nil
		}, func() error {
			if s.engine != nil {
				return s.engine.DeletePatientDEKForClinic(ctx, cUUID, patientID)
			}
			return nil
		}).
		Step("create patient record", func() error {
			var cerr error
			created, cerr = s.repo.Create(ctx, p)
			return cerr
		}).
		Err()

	if err != nil {
		return nil, err
	}
	return created, nil
}

// Get returns a patient by id, scoped to the clinic.
func (s *Service) Get(ctx context.Context, clinicID, patientID string) (*Patient, error) {
	pUUID, err := ident.ParsePatient(patientID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid patient id")
	}
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid clinic id")
	}

	return s.repo.Get(ctx, cUUID, pUUID)
}

// GetWithCreator returns a patient with the registrar's email, scoped to the clinic.
func (s *Service) GetWithCreator(ctx context.Context, clinicID, patientID string) (*GetPatientWithCreatorRow, error) {
	pUUID, err := ident.ParsePatient(patientID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid patient id")
	}
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid clinic id")
	}

	return s.repo.GetWithCreator(ctx, cUUID, pUUID)
}

// GetMany returns patients from one clinic, preserving the order of ids.
func (s *Service) GetMany(ctx context.Context, clinicID string, ids []string) ([]Patient, error) {
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid clinic id")
	}
	patientIDs := make([]ident.PatientID, 0, len(ids))
	for _, rawID := range ids {
		pUUID, err := ident.ParsePatient(rawID)
		if err != nil {
			return nil, errors.Wrap(err, "usecase: invalid patient id")
		}
		patientIDs = append(patientIDs, pUUID)
	}
	if optimized, ok := s.repo.(patientmodel.PatientQueryRepository); ok {
		patients, err := optimized.GetMany(ctx, cUUID, patientIDs)
		if err != nil {
			return nil, err
		}
		byID := make(map[ident.PatientID]Patient, len(patients))
		for _, patient := range patients {
			byID[patient.ID] = patient
		}
		out := make([]Patient, 0, len(patientIDs))
		for _, id := range patientIDs {
			if patient, ok := byID[id]; ok {
				out = append(out, patient)
			}
		}
		return out, nil
	}
	out := make([]Patient, 0, len(patientIDs))
	for _, id := range patientIDs {
		patient, err := s.repo.Get(ctx, cUUID, id)
		if err != nil {
			return nil, err
		}
		if patient != nil {
			out = append(out, *patient)
		}
	}
	return out, nil
}

// Delete removes a patient aggregate after the caller has completed its
// crypto-shredding step. The concrete repository must support aggregate
// deletion in production.
func (s *Service) Delete(ctx context.Context, clinicID, patientID string) error {
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid clinic id")
	}
	pUUID, err := ident.ParsePatient(patientID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid patient id")
	}
	repo, ok := s.repo.(patientmodel.PatientDeletionRepository)
	if !ok {
		return errors.New("usecase: patient deletion repository is unavailable")
	}
	return repo.DeleteAggregate(ctx, cUUID, pUUID)
}

// Update validates in and updates the patient.
func (s *Service) Update(ctx context.Context, clinicID, patientID string, in PatientInput) (*Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	pUUID, err := ident.ParsePatient(patientID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid patient id")
	}
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid clinic id")
	}

	existing, err := s.repo.Get(ctx, cUUID, pUUID)
	if err != nil {
		return nil, err
	}

	p := Patient{
		ID:          pUUID,
		ClinicID:    cUUID,
		DisplayName: normalized.DisplayName,
		BirthDate:   strPtr(normalized.BirthDate),
		Sex:         normalized.Sex,
		Phone:       strPtr(normalized.Phone),
		Email:       strPtr(normalized.Email),
		Street:      strPtr(normalized.Street),
		City:        strPtr(normalized.City),
		State:       strPtr(normalized.State),
		PostalCode:  strPtr(normalized.PostalCode),
		Notes:       strPtr(normalized.Notes),
		Status:      existing.Status,
		CreatedBy:   existing.CreatedBy,
	}

	return s.repo.Update(ctx, p)
}

// SetStatus updates the patient status, scoped to the clinic.
func (s *Service) SetStatus(ctx context.Context, clinicID, patientID string, status patientmodel.PatientStatus) error {
	pUUID, err := ident.ParsePatient(patientID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid patient id")
	}
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid clinic id")
	}

	count, err := s.repo.BulkSetStatus(ctx, cUUID, []ident.PatientID{pUUID}, status)
	if err != nil {
		return errors.Wrap(err, "usecase: set patient status")
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// BulkSetStatus updates the status for multiple patients in a clinic.
func (s *Service) BulkSetStatus(ctx context.Context, clinicID string, ids []string, status patientmodel.PatientStatus) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return 0, errors.Wrap(err, "usecase: invalid clinic id")
	}
	uuids := make([]ident.PatientID, 0, len(ids))
	for _, rawID := range ids {
		u, err := ident.ParsePatient(rawID)
		if err != nil {
			return 0, errors.Wrapf(err, "usecase: invalid patient id %q", rawID)
		}
		uuids = append(uuids, u)
	}
	return s.repo.BulkSetStatus(ctx, cUUID, uuids, status)
}

// List returns decrypted patients matching clinic and status filters with pagination.
func (s *Service) List(ctx context.Context, clinicID, q, status, field string, limit, offset int) ([]Patient, int64, error) {
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return nil, 0, errors.Wrap(err, "usecase: invalid clinic id")
	}

	var statusFilter *patientmodel.PatientStatus
	if status != "" {
		st := patientmodel.PatientStatus(status)
		statusFilter = &st
	}

	if optimized, ok := s.repo.(patientmodel.PatientQueryRepository); ok {
		return s.listOptimized(ctx, optimized, cUUID, strings.TrimSpace(q), field, statusFilter, limit, offset)
	}

	patients, err := s.repo.ListByClinicAndStatus(ctx, cUUID, statusFilter)
	if err != nil {
		return nil, 0, errors.Wrap(err, "usecase: list patients")
	}

	filtered := filterPatients(patients, strings.ToLower(strings.TrimSpace(q)), field)
	return paginatePatients(filtered, limit, offset), int64(len(filtered)), nil
}

func filterPatients(patients []Patient, trimmedQ, field string) []Patient {
	filtered := make([]Patient, 0, len(patients))
	for _, pt := range patients {
		if trimmedQ != "" && !patientMatches(pt, trimmedQ, field) {
			continue
		}
		filtered = append(filtered, pt)
	}
	return filtered
}

func patientMatches(pt Patient, trimmedQ, field string) bool {
	matchesName := matchesNameFuzzy(pt.DisplayName, trimmedQ)
	matchesEmail := pt.Email != nil && strings.HasPrefix(strings.ToLower(*pt.Email), trimmedQ)
	switch field {
	case "name":
		return matchesName
	case "email":
		return matchesEmail
	default:
		return matchesName || matchesEmail
	}
}

// MatchesNameFuzzy reports whether displayName satisfies all query words under phonetic and prefix matching.
func MatchesNameFuzzy(displayName, query string) bool {
	return matchesNameFuzzy(displayName, query)
}

func matchesNameFuzzy(displayName, query string) bool {
	queryWords := strings.Fields(strings.ToLower(query))
	if len(queryWords) == 0 {
		return true
	}
	displayWords := strings.Fields(strings.ToLower(displayName))
	if len(displayWords) == 0 {
		return false
	}

	type wordProfile struct {
		raw      string
		norm     string
		phonetic []string
	}

	dProfiles := make([]wordProfile, 0, len(displayWords))
	for _, dw := range displayWords {
		clean := strings.TrimFunc(dw, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if clean == "" || normalizer.IsStopWord(clean) {
			continue
		}
		norm := normalizer.NormalizeBrazilian(clean)
		var phs []string
		if len(clean) >= 3 {
			phs = append(phs, normalizer.PhoneticTokens(clean, 4)...)
			if norm != clean {
				phs = append(phs, normalizer.PhoneticTokens(norm, 4)...)
			}
		}
		dProfiles = append(dProfiles, wordProfile{
			raw:      clean,
			norm:     norm,
			phonetic: phs,
		})
	}

	if len(dProfiles) == 0 {
		return false
	}

	hasNonStopWords := false
	for _, qw := range queryWords {
		cleanQ := strings.TrimFunc(qw, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if cleanQ != "" && !normalizer.IsStopWord(cleanQ) {
			hasNonStopWords = true
			break
		}
	}

	for _, qw := range queryWords {
		cleanQ := strings.TrimFunc(qw, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if cleanQ == "" {
			continue
		}
		if hasNonStopWords && normalizer.IsStopWord(cleanQ) {
			continue
		}

		normQ := normalizer.NormalizeBrazilian(cleanQ)
		var qPhs []string
		if len(cleanQ) >= 3 {
			qPhs = append(qPhs, normalizer.PhoneticTokens(cleanQ, 4)...)
			if normQ != cleanQ {
				qPhs = append(qPhs, normalizer.PhoneticTokens(normQ, 4)...)
			}
		}

		wordMatched := false
		for _, dp := range dProfiles {
			// 1. Literal prefix match (handles "j" -> "joana", "jo" -> "joana", "maria" -> "maria")
			if strings.HasPrefix(dp.raw, cleanQ) {
				wordMatched = true
				break
			}

			// 2. Brazilian normalized prefix match (handles "yara" -> "iara", "iar" -> "iara", "artur" -> "arthur", "walter" -> "valter")
			if normQ != "" && (strings.HasPrefix(dp.norm, normQ) || strings.HasPrefix(dp.norm, cleanQ) || strings.HasPrefix(dp.raw, normQ)) {
				wordMatched = true
				break
			}

			// 3. Sousa / Souza vowel equivalence (souza <-> sousa, soza <-> sosa)
			if (strings.HasPrefix(cleanQ, "sou") || strings.HasPrefix(cleanQ, "so")) &&
				(strings.HasPrefix(dp.raw, "sou") || strings.HasPrefix(dp.raw, "so")) {
				sQ := strings.ReplaceAll(strings.ReplaceAll(cleanQ, "sou", "so"), "z", "s")
				sD := strings.ReplaceAll(strings.ReplaceAll(dp.raw, "sou", "so"), "z", "s")
				if strings.HasPrefix(sD, sQ) {
					wordMatched = true
					break
				}
			}

			// 4. Beider-Morse phonetic match (strictly for len >= 3)
			if len(cleanQ) >= 3 && len(qPhs) > 0 && len(dp.phonetic) > 0 {
				phMatch := false
				for _, qp := range qPhs {
					for _, dpp := range dp.phonetic {
						if qp == dpp {
							phMatch = true
							break
						}
					}
					if phMatch {
						break
					}
				}
				if phMatch {
					wordMatched = true
					break
				}
			}
		}

		if !wordMatched {
			return false
		}
	}
	return true
}

func paginatePatients(filtered []Patient, limit, offset int) []Patient {
	start := offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end]
}

func (s *Service) listOptimized(
	ctx context.Context,
	repo patientmodel.PatientQueryRepository,
	clinicID ident.ClinicID,
	q, field string,
	status *patientmodel.PatientStatus,
	limit, offset int,
) ([]Patient, int64, error) {
	sq, err := searchBlindIndexes(ctx, q, field)
	if err != nil {
		return nil, 0, err
	}
	if sq.empty {
		return []Patient{}, 0, nil
	}
	candidates, total, err := repo.ListCandidates(ctx, clinicID, status, sq.nameTokenGroups, sq.emailBlindIndex, limit, offset)
	if err != nil {
		return nil, 0, errors.Wrap(err, "usecase: list patient candidates")
	}
	return hydrateCandidates(ctx, repo, clinicID, candidates, total)
}

type searchQuery struct {
	nameTokenGroups [][]string
	emailBlindIndex string
	empty           bool
}

func searchBlindIndexes(ctx context.Context, q, field string) (searchQuery, error) {
	if q == "" {
		return searchQuery{}, nil
	}
	hasher := fle.ResolveHasher(ctx, nil)
	if hasher == nil {
		return searchQuery{}, errors.New("usecase: clinic hasher is required for patient search")
	}
	if field == "email" || (field == "" && strings.Contains(q, "@")) {
		emailBlindIndex, err := hasher.BlindIndex("patient.email", normalizer.Email(q))
		if err != nil {
			return searchQuery{}, errors.Wrap(err, "usecase: hash patient email search")
		}
		return searchQuery{emailBlindIndex: emailBlindIndex}, nil
	}
	return nameSearchTokens(hasher, q)
}

func nameSearchTokens(hasher crypto.Hasher, q string) (searchQuery, error) {
	words := strings.Fields(normalizer.Text(q))
	if len(words) == 0 {
		return searchQuery{empty: true}, nil
	}

	hasNonStopWords := false
	for _, word := range words {
		if !normalizer.IsStopWord(word) {
			hasNonStopWords = true
			break
		}
	}

	var tokenGroups [][]string

	for _, word := range words {
		if hasNonStopWords && normalizer.IsStopWord(word) {
			continue
		}

		var group []string
		seen := make(map[string]struct{})

		addHash := func(tok string) error {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				return nil
			}
			h, err := hasher.BlindIndexRaw("patient.token", tok)
			if err != nil {
				return err
			}
			if _, exists := seen[h]; !exists {
				seen[h] = struct{}{}
				group = append(group, h)
			}
			return nil
		}

		// 1. Prefix token
		tokens := normalizer.NameTokens(word)
		if len(tokens) > 0 {
			if err := addHash(tokens[len(tokens)-1]); err != nil {
				return searchQuery{}, errors.Wrap(err, "usecase: hash patient name search prefix")
			}
		}

		// 2. Beider-Morse multi-language phonetic tokens (only for words len >= 3)
		if len(word) >= 3 {
			for _, ph := range normalizer.PhoneticTokens(word, 4) {
				if err := addHash(ph); err != nil {
					return searchQuery{}, errors.Wrap(err, "usecase: hash patient name search phonetic")
				}
			}
		}

		// 3. Brazilian Portuguese normalized tokens
		br := normalizer.NormalizeBrazilian(word)
		if br != "" && br != word {
			brTokens := normalizer.NameTokens(br)
			if len(brTokens) > 0 {
				if err := addHash(brTokens[len(brTokens)-1]); err != nil {
					return searchQuery{}, errors.Wrap(err, "usecase: hash patient name search br norm prefix")
				}
			}
			if len(br) >= 3 {
				for _, ph := range normalizer.PhoneticTokens(br, 4) {
					if err := addHash(ph); err != nil {
						return searchQuery{}, errors.Wrap(err, "usecase: hash patient name search br phonetic")
					}
				}
			}
		}

		// 4. Souza / Sousa equivalence
		if strings.Contains(word, "ou") {
			oVar := strings.ReplaceAll(word, "ou", "o")
			oTokens := normalizer.NameTokens(oVar)
			if len(oTokens) > 0 {
				if err := addHash(oTokens[len(oTokens)-1]); err != nil {
					return searchQuery{}, errors.Wrap(err, "usecase: hash patient name search ou prefix")
				}
			}
		}
		if strings.Contains(word, "sou") {
			zVar := strings.ReplaceAll(word, "sou", "souz")
			zTokens := normalizer.NameTokens(zVar)
			if len(zTokens) > 0 {
				if err := addHash(zTokens[len(zTokens)-1]); err != nil {
					return searchQuery{}, errors.Wrap(err, "usecase: hash patient name search souz prefix")
				}
			}
		}

		if len(group) > 0 {
			tokenGroups = append(tokenGroups, group)
		}
	}

	if len(tokenGroups) == 0 {
		return searchQuery{empty: true}, nil
	}
	return searchQuery{nameTokenGroups: tokenGroups}, nil
}

func hydrateCandidates(
	ctx context.Context,
	repo patientmodel.PatientQueryRepository,
	clinicID ident.ClinicID,
	candidates []patientmodel.PatientCandidate,
	total int,
) ([]Patient, int64, error) {
	ids := make([]ident.PatientID, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	hydrated, err := repo.GetMany(ctx, clinicID, ids)
	if err != nil {
		return nil, 0, errors.Wrap(err, "usecase: hydrate patients")
	}
	byID := make(map[ident.PatientID]Patient, len(hydrated))
	for _, patient := range hydrated {
		byID[patient.ID] = patient
	}
	out := make([]Patient, 0, len(candidates))
	for _, candidate := range candidates {
		if patient, ok := byID[candidate.ID]; ok {
			out = append(out, patient)
		}
	}
	return out, int64(total), nil
}

// ListPage returns one page of decrypted patients.
func (s *Service) ListPage(ctx context.Context, clinicID, q, field, status string, limit, offset int) ([]Patient, int64, error) {
	return s.List(ctx, clinicID, q, status, field, limit, offset)
}

func matchWordPrefix(text, query string) bool {
	words := strings.Fields(strings.ToLower(text))
	for _, w := range words {
		if strings.HasPrefix(w, query) {
			return true
		}
	}
	return false
}

// Count returns total number of patients for the clinic.
func (s *Service) Count(ctx context.Context, clinicID string) (int64, error) {
	cUUID, err := ident.ParseClinic(clinicID)
	if err != nil {
		return 0, errors.Wrap(err, "usecase: invalid clinic id")
	}
	count, err := s.repo.Count(ctx, cUUID)
	if err != nil {
		return 0, errors.Wrap(err, "usecase: count patients")
	}
	return int64(count), nil
}

func normalize(in PatientInput) (PatientInput, error) {
	out := PatientInput{
		DisplayName: strings.TrimSpace(in.DisplayName),
		BirthDate:   strings.TrimSpace(in.BirthDate),
		Sex:         patientmodel.Sex(strings.TrimSpace(in.Sex.String())),
		Phone:       strings.TrimSpace(in.Phone),
		Email:       strings.TrimSpace(in.Email),
		Street:      strings.TrimSpace(in.Street),
		City:        strings.TrimSpace(in.City),
		State:       strings.TrimSpace(in.State),
		PostalCode:  strings.TrimSpace(in.PostalCode),
		Notes:       strings.TrimSpace(in.Notes),
	}
	if out.Sex == "" {
		out.Sex = patientmodel.SexUnknown
	}

	v := validator.New()

	v.Field(out.DisplayName, "display_name", "patient name").
		Required().
		Max(maxPatientNameLen)

	v.Field(out.Phone, "phone", "patient phone").
		Required().
		Max(maxPhoneLen)

	v.Field(out.Email, "email", "patient email").
		Required().
		Email().
		Max(maxEmailLen)

	v.Field(out.BirthDate, "birth_date", "birth date").
		Optional().
		DateISO()

	v.Field(out.Street, "street", "street").
		Optional().
		Max(maxAddressLen)

	v.Field(out.City, "city", "city").
		Optional().
		Max(maxAddressLen)

	v.Field(out.State, "state", "state").
		Optional().
		Max(maxAddressLen)

	v.Field(out.PostalCode, "postal_code", "postal code").
		Optional().
		Max(maxAddressLen)

	v.Field(out.Notes, "notes", "notes").
		Optional().
		Max(maxNotesLen)

	v.Validatable(out.Sex, "sex", "sex")

	if err := v.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
