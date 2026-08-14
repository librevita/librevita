package identifier

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"librevita.org/internal/domain/patient/repository"
)

// ErrSystemNotFound is returned when the system does not exist.
var ErrSystemNotFound = errors.New("identifier: system not found")

// SystemInput is the editable definition of a document system.
type SystemInput struct {
	System           string
	DisplayName      string
	Pattern          string
	Transform        Transform
	CheckAlgorithm   CheckAlgorithm
	CheckBaseLen     int
	CheckDVCount     int
	CheckStartWeight int
}

// SystemsService administers identifier_systems. Every change is
// validated, persisted, and applied to the shared Registry, so the new
// or updated document type is usable immediately.
type SystemsService struct {
	q   *repository.Queries
	reg *Registry
	log *slog.Logger
}

// NewSystemsService is the Fx provider.
func NewSystemsService(db *sql.DB, reg *Registry, log *slog.Logger) *SystemsService {
	return &SystemsService{q: repository.New(db), reg: reg, log: log}
}

// LoadActiveSystems returns the active systems in detection order.
// It is used at boot to fill the registry.
func LoadActiveSystems(ctx context.Context, db *sql.DB) ([]repository.IdentifierSystem, error) {
	rows, err := repository.New(db).ListActiveIdentifierSystems(ctx)
	if err != nil {
		return nil, fmt.Errorf("identifier: load active systems: %w", err)
	}
	return rows, nil
}

// List returns every system, active and inactive, ordered by URN.
func (s *SystemsService) List(ctx context.Context) ([]repository.IdentifierSystem, error) {
	rows, err := s.q.ListIdentifierSystems(ctx)
	if err != nil {
		return nil, fmt.Errorf("identifier: list systems: %w", err)
	}
	return rows, nil
}

// SystemByID returns one system, active or inactive.
func (s *SystemsService) SystemByID(ctx context.Context, id string) (*repository.IdentifierSystem, error) {
	row, err := s.q.GetIdentifierSystemByID(ctx, uuid.MustParse(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSystemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("identifier: get system: %w", err)
	}
	return &row, nil
}

// Create registers a new document system. The URN must be unique and
// in the application namespace, the pattern must compile, and the
// check-digit fields must be consistent. The registry is reloaded on
// success.
func (s *SystemsService) Create(ctx context.Context, createdBy string, in SystemInput) (*repository.IdentifierSystem, error) {
	cfg, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("identifier: generate system id: %w", err)
	}
	row, err := s.q.CreateIdentifierSystem(ctx, repository.CreateIdentifierSystemParams{
		ID:               id,
		System:           cfg.System,
		DisplayName:      cfg.DisplayName,
		Pattern:          cfg.Pattern,
		Transform:        string(cfg.Transform),
		CheckAlgorithm:   string(cfg.CheckAlgorithm),
		CheckBaseLen:     int64(cfg.CheckBaseLen),
		CheckDvCount:     int64(cfg.CheckDVCount),
		CheckStartWeight: int64(cfg.CheckStartWeight),
		CreatedBy:        uuidOrNil(createdBy),
	})
	if err != nil {
		return nil, fmt.Errorf("identifier: create system: %w", err)
	}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return &row, nil
}

// Update replaces the definition of an existing system. The URN itself
// is immutable: it is the FHIR system of stored identifiers, so
// renaming it would orphan them. The active flag is preserved: editing
// a deactivated system must not silently reactivate it.
func (s *SystemsService) Update(ctx context.Context, id string, in SystemInput) (*repository.IdentifierSystem, error) {
	cfg, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	existing, err := s.SystemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	row, err := s.q.UpdateIdentifierSystem(ctx, repository.UpdateIdentifierSystemParams{
		DisplayName:      cfg.DisplayName,
		Pattern:          cfg.Pattern,
		Transform:        string(cfg.Transform),
		CheckAlgorithm:   string(cfg.CheckAlgorithm),
		CheckBaseLen:     int64(cfg.CheckBaseLen),
		CheckDvCount:     int64(cfg.CheckDVCount),
		CheckStartWeight: int64(cfg.CheckStartWeight),
		Active:           existing.Active,
		ID:               uuid.MustParse(id),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSystemNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("identifier: update system: %w", err)
	}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return &row, nil
}

// SetActive activates or deactivates a system. Deactivated systems
// keep their stored identifiers (they remain readable and removable)
// but no longer participate in detection or exact lookup.
func (s *SystemsService) SetActive(ctx context.Context, id string, active bool) error {
	value := int64(0)
	if active {
		value = 1
	}
	existing, err := s.q.GetIdentifierSystemByID(ctx, uuid.MustParse(id))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSystemNotFound
	}
	if err != nil {
		return fmt.Errorf("identifier: get system: %w", err)
	}
	_, err = s.q.UpdateIdentifierSystem(ctx, repository.UpdateIdentifierSystemParams{
		DisplayName:      existing.DisplayName,
		Pattern:          existing.Pattern,
		Transform:        existing.Transform,
		CheckAlgorithm:   existing.CheckAlgorithm,
		CheckBaseLen:     existing.CheckBaseLen,
		CheckDvCount:     existing.CheckDvCount,
		CheckStartWeight: existing.CheckStartWeight,
		Active:           value,
		ID:               existing.ID,
	})
	if err != nil {
		return fmt.Errorf("identifier: set system active: %w", err)
	}
	return s.reload(ctx)
}

// reload refreshes the shared registry from the database.
func (s *SystemsService) reload(ctx context.Context) error {
	rows, err := s.q.ListActiveIdentifierSystems(ctx)
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

func uuidOrNil(s string) uuid.UUID {
	if s == "" {
		return uuid.Nil
	}
	return uuid.MustParse(s)
}
