package identifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// ErrSystemNotFound is returned when the system does not exist.
var ErrSystemNotFound = errors.New("identifier: system not found")

// SystemInput is the editable definition of a document system.
type SystemInput struct {
	System           string
	DisplayName      string
	Pattern          string
	Mask             string
	Transform        Transform
	CheckAlgorithm   CheckAlgorithm
	CheckBaseLen     int
	CheckDVCount     int
	CheckStartWeight int
}

// SystemsService administers identifier_systems.
type SystemsService struct {
	repo SystemRepository
	reg  *Registry
	log  *slog.Logger
}

// NewSystemsService is the Fx provider.
func NewSystemsService(repo SystemRepository, reg *Registry, log *slog.Logger) *SystemsService {
	return &SystemsService{repo: repo, reg: reg, log: log}
}

// List returns every system, active and inactive, ordered by URN.
func (s *SystemsService) List(ctx context.Context) ([]*IdentifierSystem, error) {
	return s.repo.ListAll(ctx)
}

// SystemByID returns one system, active or inactive.
func (s *SystemsService) SystemByID(ctx context.Context, id string) (*IdentifierSystem, error) {
	uUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("identifier: invalid system id: %w", err)
	}
	return s.repo.GetByID(ctx, uUUID)
}

// Create registers a new document system.
func (s *SystemsService) Create(ctx context.Context, createdBy string, in SystemInput) (*IdentifierSystem, error) {
	cfg, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("identifier: generate system id: %w", err)
	}

	var cb *uuid.UUID
	if createdBy != "" {
		parsed := uuid.MustParse(createdBy)
		cb = &parsed
	}

	sys := &IdentifierSystem{
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
func (s *SystemsService) Update(ctx context.Context, id string, in SystemInput) (*IdentifierSystem, error) {
	cfg, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	uUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("identifier: invalid system id: %w", err)
	}

	sys := &IdentifierSystem{
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
func (s *SystemsService) SetActive(ctx context.Context, id string, active bool) error {
	uUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("identifier: invalid system id: %w", err)
	}
	if err := s.repo.SetActive(ctx, uUUID, active); err != nil {
		return err
	}
	return s.reload(ctx)
}

// reload refreshes the shared registry from the database.
func (s *SystemsService) reload(ctx context.Context) error {
	rows, err := s.repo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("identifier: reload systems: %w", err)
	}
	if err := s.reg.Reload(rows); err != nil {
		return fmt.Errorf("identifier: reload systems: %w", err)
	}
	return nil
}

func validateInput(in SystemInput) (SystemConfig, error) {
	cfg := SystemConfig{
		System:           strings.TrimSpace(in.System),
		DisplayName:      strings.TrimSpace(in.DisplayName),
		Pattern:          strings.TrimSpace(in.Pattern),
		Transform:        in.Transform,
		CheckAlgorithm:   in.CheckAlgorithm,
		CheckBaseLen:     in.CheckBaseLen,
		CheckDVCount:     in.CheckDVCount,
		CheckStartWeight: in.CheckStartWeight,
	}
	if err := cfg.validateShape(); err != nil {
		return SystemConfig{}, &ValidationError{Msg: err.Error()}
	}
	if !strings.HasPrefix(cfg.System, systemURNPrefix) {
		return SystemConfig{}, &ValidationError{Msg: "system must start with " + systemURNPrefix}
	}
	if len(cfg.DisplayName) > 80 {
		return SystemConfig{}, &ValidationError{Msg: "display name is too long"}
	}
	if _, err := compilePattern(cfg.Pattern); err != nil {
		return SystemConfig{}, &ValidationError{Msg: "pattern is not a valid regex: " + err.Error()}
	}
	return cfg, nil
}
