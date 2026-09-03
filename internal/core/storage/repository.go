package storage

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/storageobject"
	"librevita.org/pkg/ident"
)

type indexRepository struct {
	client *record.Client
}

// NewIndexRepository creates a storage master index repository adapter.
func NewIndexRepository(client *record.Client) IndexRepository {
	return &indexRepository{client: client}
}

func (r *indexRepository) Insert(ctx context.Context, f StoredFile) (*StoredFile, error) {
	clinicID, err := clinicctx.MustClinicID(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage repository: insert")
	}
	created, err := r.client.StorageObject.Create().
		SetID(ident.StorageObjectID(f.ID)).
		SetClinicID(clinicID).
		SetKey(f.Key).
		SetDomain(f.Domain).
		SetResourceID(f.ResourceID.String()).
		SetOriginalName(f.OriginalName).
		SetContentType(f.ContentType).
		SetSize(f.Size).
		SetEtag(f.ETag).
		SetChecksum(f.Checksum).
		SetCreatedBy(ident.UserID(f.CreatedBy)).
		Save(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage repository: insert")
	}
	return storedFileFromEnt(created), nil
}

func (r *indexRepository) Get(ctx context.Context, objectID uuid.UUID) (*StoredFile, error) {
	obj, err := r.client.StorageObject.Get(ctx, ident.StorageObjectID(objectID))
	if err != nil {
		if record.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "storage repository: get")
	}
	return storedFileFromEnt(obj), nil
}

func (r *indexRepository) GetForResource(ctx context.Context, domain string, resourceID, objectID uuid.UUID) (*StoredFile, error) {
	obj, err := r.client.StorageObject.Query().
		Where(
			storageobject.DomainEQ(domain),
			storageobject.ResourceIDEQ(resourceID.String()),
			storageobject.IDEQ(ident.StorageObjectID(objectID)),
		).
		Only(ctx)
	if err != nil {
		if record.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "storage repository: get for resource")
	}
	return storedFileFromEnt(obj), nil
}

func (r *indexRepository) List(ctx context.Context, domain string, resourceID uuid.UUID) ([]StoredFile, error) {
	objs, err := r.client.StorageObject.Query().
		Where(
			storageobject.DomainEQ(domain),
			storageobject.ResourceIDEQ(resourceID.String()),
		).
		Order(record.Desc(storageobject.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "storage repository: list")
	}
	out := make([]StoredFile, 0, len(objs))
	for _, obj := range objs {
		out = append(out, *storedFileFromEnt(obj))
	}
	return out, nil
}

func (r *indexRepository) Delete(ctx context.Context, objectID uuid.UUID) (string, error) {
	obj, err := r.client.StorageObject.Get(ctx, ident.StorageObjectID(objectID))
	if err != nil {
		if record.IsNotFound(err) {
			return "", ErrNotFound
		}
		return "", errors.Wrap(err, "storage repository: get for delete")
	}
	if err := r.client.StorageObject.DeleteOneID(ident.StorageObjectID(objectID)).Exec(ctx); err != nil {
		return "", errors.Wrap(err, "storage repository: delete")
	}
	return obj.Key, nil
}

func (r *indexRepository) KeyExists(ctx context.Context, key string) (bool, error) {
	return r.client.StorageObject.Query().Where(storageobject.KeyEQ(key)).Exist(ctx)
}

func storedFileFromEnt(obj *record.StorageObject) *StoredFile {
	resID, _ := uuid.Parse(obj.ResourceID)
	return &StoredFile{
		ID:           obj.ID.UUID(),
		Key:          obj.Key,
		Domain:       obj.Domain,
		ResourceID:   resID,
		OriginalName: obj.OriginalName,
		ContentType:  obj.ContentType,
		Size:         obj.Size,
		ETag:         obj.Etag,
		Checksum:     obj.Checksum,
		CreatedBy:    obj.CreatedBy.UUID(),
		CreatedAt:    obj.CreatedAt,
	}
}
