package usecase

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	identifiermodel "librevita.org/internal/domain/identifier/model"
)

// systemURNPrefix is the namespace administrators use when registering
// a new document system.
const systemURNPrefix = "urn:librevita:id:"

type systemsService struct {
	repo identifiermodel.SystemRepository
	reg  *identifiermodel.Registry
}

// NewSystemsService creates a new SystemsService implementation for administering document systems.
func NewSystemsService(repo identifiermodel.SystemRepository, reg *identifiermodel.Registry) SystemsService {
	return &systemsService{repo: repo, reg: reg}
}

// List returns every system, active and inactive, ordered by URN.
func (s *systemsService) List(ctx context.Context) ([]*identifiermodel.IdentifierSystem, error) {
	return s.repo.ListAll(ctx)
}

// SystemByID returns one system, active or inactive.
func (s *systemsService) SystemByID(ctx context.Context, id string) (*identifiermodel.IdentifierSystem, error) {
	uUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: invalid system id")
	}
	return s.repo.GetByID(ctx, uUUID)
}

// Create registers a new document system.
func (s *systemsService) Create(ctx context.Context, createdBy string, in SystemInput) (*identifiermodel.IdentifierSystem, error) {
	cfg, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, errors.Wrap(err, "identifier: generate system id")
	}

	var cb *uuid.UUID
	if createdBy != "" {
		parsed := uuid.MustParse(createdBy)
		cb = &parsed
	}

	sys := &identifiermodel.IdentifierSystem{
		ID:               id,
		System:           cfg.System,
		DisplayName:      cfg.DisplayName,
		Pattern:          cfg.Pattern,
		Mask:             cfg.Mask,
		Transform:        cfg.Transform,
		CheckAlgorithm:   cfg.CheckAlgorithm,
		CheckBaseLen:     cfg.CheckBaseLen,
		CheckDVCount:     cfg.CheckDVCount,
		CheckStartWeight: cfg.CheckStartWeight,
		Active:           true,
		CreatedBy:        cb,
	}

	saved, err := s.repo.Create(ctx, sys)
	if err != nil {
		return nil, err
	}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return saved, nil
}

// Update replaces the definition of an existing system.
func (s *systemsService) Update(ctx context.Context, id string, in SystemInput) (*identifiermodel.IdentifierSystem, error) {
	cfg, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	uUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: invalid system id")
	}

	sys := &identifiermodel.IdentifierSystem{
		ID:               uUUID,
		System:           cfg.System,
		DisplayName:      cfg.DisplayName,
		Pattern:          cfg.Pattern,
		Mask:             cfg.Mask,
		Transform:        cfg.Transform,
		CheckAlgorithm:   cfg.CheckAlgorithm,
		CheckBaseLen:     cfg.CheckBaseLen,
		CheckDVCount:     cfg.CheckDVCount,
		CheckStartWeight: cfg.CheckStartWeight,
	}

	updated, err := s.repo.Update(ctx, sys)
	if err != nil {
		return nil, err
	}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return updated, nil
}

// SetActive activates or deactivates a system.
func (s *systemsService) SetActive(ctx context.Context, id string, active bool) error {
	uUUID, err := uuid.Parse(id)
	if err != nil {
		return errors.Wrap(err, "identifier: invalid system id")
	}
	if err := s.repo.SetActive(ctx, uUUID, active); err != nil {
		return err
	}
	return s.reload(ctx)
}

// reload refreshes the shared registry from the database.
func (s *systemsService) reload(ctx context.Context) error {
	rows, err := s.repo.ListActive(ctx)
	if err != nil {
		return errors.Wrap(err, "identifier: reload systems")
	}
	if err := s.reg.Reload(rows); err != nil {
		return errors.Wrap(err, "identifier: reload systems")
	}
	return nil
}

func validateInput(in SystemInput) (identifiermodel.SystemConfig, error) {
	cfg := identifiermodel.SystemConfig{
		System:           strings.TrimSpace(in.System),
		DisplayName:      strings.TrimSpace(in.DisplayName),
		Pattern:          strings.TrimSpace(in.Pattern),
		Transform:        in.Transform,
		CheckAlgorithm:   in.CheckAlgorithm,
		CheckBaseLen:     in.CheckBaseLen,
		CheckDVCount:     in.CheckDVCount,
		CheckStartWeight: in.CheckStartWeight,
	}
	if err := cfg.ValidateShape(); err != nil {
		return identifiermodel.SystemConfig{}, &identifiermodel.ValidationError{Msg: err.Error()}
	}
	if !strings.HasPrefix(cfg.System, systemURNPrefix) {
		return identifiermodel.SystemConfig{}, &identifiermodel.ValidationError{Msg: "system must start with " + systemURNPrefix}
	}
	if len(cfg.DisplayName) > 80 {
		return identifiermodel.SystemConfig{}, &identifiermodel.ValidationError{Msg: "display name is too long"}
	}
	if _, err := identifiermodel.ParseSystemConfig(&identifiermodel.IdentifierSystem{
		System:           cfg.System,
		DisplayName:      cfg.DisplayName,
		Pattern:          cfg.Pattern,
		Transform:        cfg.Transform,
		CheckAlgorithm:   cfg.CheckAlgorithm,
		CheckBaseLen:     cfg.CheckBaseLen,
		CheckDVCount:     cfg.CheckDVCount,
		CheckStartWeight: cfg.CheckStartWeight,
	}); err != nil {
		return identifiermodel.SystemConfig{}, &identifiermodel.ValidationError{Msg: "pattern is not a valid regex: " + err.Error()}
	}
	return cfg, nil
}
