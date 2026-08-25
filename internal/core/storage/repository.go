package storage

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/storageobject"
	"librevita.org/internal/core/clinicctx"
)

type indexRepository struct {
	client *ent.Client
}

// NewIndexRepository creates a storage master index repository adapter.
func NewIndexRepository(client *ent.Client) IndexRepository {
	return &indexRepository{client: client}
}

func (r *indexRepository) Insert(ctx context.Context, f StoredFile) (*StoredFile, error) {
	clinicID, err := clinicctx.MustClinicID(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage repository: insert: %w", err)
	}
	created, err := r.client.StorageObject.Create().
		SetID(f.ID).
		SetClinicID(clinicID).
		SetKey(f.Key).
		SetDomain(f.Domain).
		SetResourceID(f.ResourceID.String()).
		SetOriginalName(f.OriginalName).
		SetContentType(f.ContentType).
		SetSize(f.Size).
		SetEtag(f.ETag).
		SetChecksum(f.Checksum).
		SetCreatedBy(f.CreatedBy).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage repository: insert: %w", err)
	}
	return storedFileFromEnt(created), nil
}

func (r *indexRepository) Get(ctx context.Context, id uuid.UUID) (*StoredFile, error) {
	obj, err := r.client.StorageObject.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage repository: get: %w", err)
	}
	return storedFileFromEnt(obj), nil
}

func (r *indexRepository) GetForResource(ctx context.Context, domain string, resourceID, id uuid.UUID) (*StoredFile, error) {
	obj, err := r.client.StorageObject.Query().
		Where(
			storageobject.DomainEQ(domain),
			storageobject.ResourceIDEQ(resourceID.String()),
			storageobject.IDEQ(id),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage repository: get for resource: %w", err)
	}
	return storedFileFromEnt(obj), nil
}

func (r *indexRepository) List(ctx context.Context, domain string, resourceID uuid.UUID) ([]StoredFile, error) {
	objs, err := r.client.StorageObject.Query().
		Where(
			storageobject.DomainEQ(domain),
			storageobject.ResourceIDEQ(resourceID.String()),
		).
		Order(ent.Desc(storageobject.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage repository: list: %w", err)
	}
	out := make([]StoredFile, 0, len(objs))
	for _, obj := range objs {
		out = append(out, *storedFileFromEnt(obj))
	}
	return out, nil
}

func (r *indexRepository) Delete(ctx context.Context, id uuid.UUID) (string, error) {
	obj, err := r.client.StorageObject.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("storage repository: get for delete: %w", err)
	}
	if err := r.client.StorageObject.DeleteOneID(id).Exec(ctx); err != nil {
		return "", fmt.Errorf("storage repository: delete: %w", err)
	}
	return obj.Key, nil
}

func (r *indexRepository) KeyExists(ctx context.Context, key string) (bool, error) {
	return r.client.StorageObject.Query().Where(storageobject.KeyEQ(key)).Exist(ctx)
}

func storedFileFromEnt(obj *ent.StorageObject) *StoredFile {
	resID, _ := uuid.Parse(obj.ResourceID)
	return &StoredFile{
		ID:           obj.ID,
		Key:          obj.Key,
		Domain:       obj.Domain,
		ResourceID:   resID,
		OriginalName: obj.OriginalName,
		ContentType:  obj.ContentType,
		Size:         obj.Size,
		ETag:         obj.Etag,
		Checksum:     obj.Checksum,
		CreatedBy:    obj.CreatedBy,
		CreatedAt:    obj.CreatedAt,
	}
}
