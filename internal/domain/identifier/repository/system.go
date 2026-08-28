package repository

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/identifiersystem"
	identifiermodel "librevita.org/internal/domain/identifier/model"
)

type systemRepository struct {
	client *ent.Client
}

// NewSystemRepository creates an identifier system repository adapter.
func NewSystemRepository(client *ent.Client) identifiermodel.SystemRepository {
	return &systemRepository{client: client}
}

func (r *systemRepository) ListAll(ctx context.Context) ([]*identifiermodel.IdentifierSystem, error) {
	rows, err := r.client.IdentifierSystem.Query().
		Order(ent.Asc(identifiersystem.FieldDisplayName)).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "system repository: list all")
	}
	out := make([]*identifiermodel.IdentifierSystem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSystemDomain(row))
	}
	return out, nil
}

func (r *systemRepository) ListActive(ctx context.Context) ([]*identifiermodel.IdentifierSystem, error) {
	rows, err := r.client.IdentifierSystem.Query().
		Where(identifiersystem.ActiveEQ(true)).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "system repository: list active")
	}
	out := make([]*identifiermodel.IdentifierSystem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSystemDomain(row))
	}
	return out, nil
}

func (r *systemRepository) GetByID(ctx context.Context, id uuid.UUID) (*identifiermodel.IdentifierSystem, error) {
	row, err := r.client.IdentifierSystem.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.WithSecondaryError(identifiermodel.ErrSystemNotFound, err)
		}
		return nil, errors.Wrap(err, "system repository: get by id")
	}
	return toSystemDomain(row), nil
}

func (r *systemRepository) GetBySystem(ctx context.Context, system string) (*identifiermodel.IdentifierSystem, error) {
	row, err := r.client.IdentifierSystem.Query().
		Where(identifiersystem.SystemEQ(system)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.WithSecondaryError(identifiermodel.ErrSystemNotFound, err)
		}
		return nil, errors.Wrap(err, "system repository: get by system")
	}
	return toSystemDomain(row), nil
}

func (r *systemRepository) Create(ctx context.Context, s *identifiermodel.IdentifierSystem) (*identifiermodel.IdentifierSystem, error) {
	create := r.client.IdentifierSystem.Create().
		SetID(s.ID).
		SetSystem(s.System).
		SetDisplayName(s.DisplayName).
		SetPattern(s.Pattern).
		SetMask(s.Mask).
		SetTransform(identifiersystem.Transform(s.Transform)).
		SetCheckAlgorithm(identifiersystem.CheckAlgorithm(s.CheckAlgorithm)).
		SetCheckBaseLen(s.CheckBaseLen).
		SetCheckDvCount(s.CheckDVCount).
		SetCheckStartWeight(s.CheckStartWeight).
		SetActive(s.Active)
	if s.CreatedBy != nil {
		create.SetCreatedBy(*s.CreatedBy)
	}

	saved, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errors.WithSecondaryError(identifiermodel.ErrDuplicate, err)
		}
		return nil, errors.Wrap(err, "system repository: create")
	}
	return toSystemDomain(saved), nil
}

func (r *systemRepository) Update(ctx context.Context, s *identifiermodel.IdentifierSystem) (*identifiermodel.IdentifierSystem, error) {
	update := r.client.IdentifierSystem.UpdateOneID(s.ID).
		SetDisplayName(s.DisplayName).
		SetPattern(s.Pattern).
		SetMask(s.Mask).
		SetTransform(identifiersystem.Transform(s.Transform)).
		SetCheckAlgorithm(identifiersystem.CheckAlgorithm(s.CheckAlgorithm)).
		SetCheckBaseLen(s.CheckBaseLen).
		SetCheckDvCount(s.CheckDVCount).
		SetCheckStartWeight(s.CheckStartWeight).
		SetUpdatedAt(time.Now())

	if s.System != "" {
		update.SetSystem(s.System)
	}

	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.WithSecondaryError(identifiermodel.ErrSystemNotFound, err)
		}
		if ent.IsConstraintError(err) {
			return nil, errors.WithSecondaryError(identifiermodel.ErrDuplicate, err)
		}
		return nil, errors.Wrap(err, "system repository: update")
	}
	return toSystemDomain(updated), nil
}

func (r *systemRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	err := r.client.IdentifierSystem.UpdateOneID(id).
		SetActive(active).
		SetUpdatedAt(time.Now()).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.WithSecondaryError(identifiermodel.ErrSystemNotFound, err)
		}
		return errors.Wrap(err, "system repository: set active")
	}
	return nil
}

func (r *systemRepository) SeedDefaults(ctx context.Context) error {
	defaultSystems := []*identifiermodel.IdentifierSystem{
		{
			System:           "urn:librevita:id:br:cpf",
			DisplayName:      "CPF (Brasil)",
			Pattern:          `[0-9]{11}`,
			Transform:        identifiermodel.TransformDigits,
			CheckAlgorithm:   identifiermodel.CheckMod11Desc,
			CheckBaseLen:     9,
			CheckDVCount:     2,
			CheckStartWeight: 10,
			Active:           true,
			Mask:             "000.000.000-00",
		},
		{
			System:           "urn:librevita:id:br:sus",
			DisplayName:      "Cartão SUS (Brasil)",
			Pattern:          `[0-9]{15}`,
			Transform:        identifiermodel.TransformDigits,
			CheckAlgorithm:   identifiermodel.CheckMod11Cyclic,
			CheckBaseLen:     14,
			CheckDVCount:     1,
			CheckStartWeight: 10,
			Active:           true,
			Mask:             "000 0000 0000 0000",
		},
		{
			System:           "urn:librevita:id:pt:nif",
			DisplayName:      "NIF (Portugal)",
			Pattern:          `[0-9]{9}`,
			Transform:        identifiermodel.TransformDigits,
			CheckAlgorithm:   identifiermodel.CheckMod11Desc,
			CheckBaseLen:     8,
			CheckDVCount:     1,
			CheckStartWeight: 9,
			Active:           true,
			Mask:             "000 000 000",
		},
		{
			System:           "urn:librevita:id:passport",
			DisplayName:      "Passaporte",
			Pattern:          `[A-Z]{1,2}[0-9]{6,9}`,
			Transform:        identifiermodel.TransformUpper,
			CheckAlgorithm:   identifiermodel.CheckNone,
			CheckBaseLen:     0,
			CheckDVCount:     1,
			CheckStartWeight: 10,
			Active:           true,
			Mask:             "",
		},
	}
	for _, sys := range defaultSystems {
		exists, err := r.client.IdentifierSystem.Query().Where(identifiersystem.SystemEQ(sys.System)).Exist(ctx)
		if err != nil {
			return errors.Wrapf(err, "system repository: check seed %q", sys.System)
		}
		if exists {
			continue
		}
		sID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if err := r.client.IdentifierSystem.Create().
			SetID(sID).
			SetSystem(sys.System).
			SetDisplayName(sys.DisplayName).
			SetPattern(sys.Pattern).
			SetTransform(identifiersystem.Transform(sys.Transform)).
			SetCheckAlgorithm(identifiersystem.CheckAlgorithm(sys.CheckAlgorithm)).
			SetCheckBaseLen(sys.CheckBaseLen).
			SetCheckDvCount(sys.CheckDVCount).
			SetCheckStartWeight(sys.CheckStartWeight).
			SetActive(sys.Active).
			SetMask(sys.Mask).
			Exec(ctx); err != nil && !ent.IsConstraintError(err) {
			return errors.Wrapf(err, "system repository: seed insert %q", sys.System)
		}
	}
	return nil
}

func toSystemDomain(row *ent.IdentifierSystem) *identifiermodel.IdentifierSystem {
	if row == nil {
		return nil
	}
	return &identifiermodel.IdentifierSystem{
		ID:               row.ID,
		System:           row.System,
		DisplayName:      row.DisplayName,
		Pattern:          row.Pattern,
		Transform:        identifiermodel.Transform(row.Transform),
		CheckAlgorithm:   identifiermodel.CheckAlgorithm(row.CheckAlgorithm),
		CheckBaseLen:     row.CheckBaseLen,
		CheckDVCount:     row.CheckDvCount,
		CheckStartWeight: row.CheckStartWeight,
		Active:           row.Active,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
