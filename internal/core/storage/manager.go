// This file implements the saga coordinator of the storage layer: the
// FileManager keeps the opaque blob Store and the SQLite master index
// consistent through compensating actions, since no distributed
// transaction exists between them.
//
// Upload: blob first, index row second. If the index write fails, the
// blob is deleted (compensation). A crash between the two leaves an
// orphaned blob that Reconcile removes.
//
// Delete: index row first, blob second. If the blob delete fails, the
// object becomes invisible immediately (index gone) and Reconcile
// retries the blob removal.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/storage/repository"
	"librevita.org/internal/types"
)

// StoredFile is the master-index metadata of one stored file.
type StoredFile struct {
	ID           uuid.UUID
	Key          string
	Domain       string
	ResourceID   uuid.UUID
	OriginalName string
	ContentType  string
	Size         int64
	ETag         string
	CreatedBy    uuid.UUID
	CreatedAt    types.DateTime
}

// UploadInput describes a new file before it is stored.
type UploadInput struct {
	// Domain is the attachment namespace, e.g. "patient_document".
	Domain string
	// ResourceID is the owning entity (e.g. the patient id).
	ResourceID uuid.UUID
	// OriginalName is the display name; it never appears in the key.
	OriginalName string
	// ContentType of the payload.
	ContentType string
	// CreatedBy is the uploading user, recorded for audit.
	CreatedBy uuid.UUID
}

// FileManager is the storage saga coordinator.
type FileManager struct {
	store Store
	q     *repository.Queries
	log   *slog.Logger
	// newID is injectable for tests; defaults to uuid.NewV7.
	newID func() (uuid.UUID, error)
}

// NewFileManager is the Fx provider.
func NewFileManager(db *sql.DB, store Store, log *slog.Logger) (*FileManager, error) {
	if db == nil {
		return nil, errors.New("storage: the master index requires the SQLite backend")
	}
	return &FileManager{store: store, q: repository.New(db), log: log, newID: uuid.NewV7}, nil
}

// Access classes of stored files. Public files (avatars) are meant for
// broad authenticated use; private files (clinical attachments) carry
// patient data and are protected by resource policies and access
// auditing. The class is derived from the domain, never chosen by the
// caller, so a sensitive file cannot be stored as public.
const (
	ClassPublic  = "public"
	ClassPrivate = "private"
)

// publicDomains maps the domains whose files are public-class. Every
// other domain is private by default: adding a new attachment kind
// requires an explicit opt-in here.
var publicDomains = map[string]bool{
	"avatar": true,
}

// objectKey derives the blob-store key from the index identity. The id
// is the object's own UUIDv7, so keys are unique, temporally sortable
// and never expose the original file name. The access class prefixes
// the key, separating public and private files physically.
func (m *FileManager) objectKey(domain string, resourceID, id uuid.UUID) string {
	class := ClassPrivate
	if publicDomains[domain] {
		class = ClassPublic
	}
	return class + "/" + domain + "/" + resourceID.String() + "/" + id.String()
}

// GetForResource resolves an object only when it belongs to the given
// domain and resource. A bare id is never enough to reach a file, so a
// caller who knows an attachment id of another patient cannot fetch it
// (IDOR protection).
func (m *FileManager) GetForResource(ctx context.Context, domain string, resourceID, id uuid.UUID) (*StoredFile, error) {
	row, err := m.q.GetStorageObjectByResourceAndID(ctx, repository.GetStorageObjectByResourceAndIDParams{
		Domain: domain, ResourceID: resourceID, ID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get index by resource: %w", err)
	}
	return storedFileFromRow(row), nil
}

// OpenForResource returns the metadata and blob of an object that
// belongs to the given resource. The caller must close the Object.
func (m *FileManager) OpenForResource(ctx context.Context, domain string, resourceID, id uuid.UUID) (*StoredFile, *Object, error) {
	meta, err := m.GetForResource(ctx, domain, resourceID, id)
	if err != nil {
		return nil, nil, err
	}
	obj, err := m.store.Get(ctx, meta.Key)
	if err != nil {
		return nil, nil, err
	}
	return meta, obj, nil
}

// Upload stores data in the blob store and registers the master-index
// row. If the index write fails, the blob is deleted so no orphan
// remains (saga compensation). The returned StoredFile carries the
// authoritative size and ETag from the blob store.
func (m *FileManager) Upload(ctx context.Context, in UploadInput, data io.Reader, size int64) (*StoredFile, error) {
	if in.Domain == "" {
		return nil, errors.New("storage: domain is required")
	}
	if in.OriginalName == "" {
		return nil, errors.New("storage: original name is required")
	}
	id, err := m.newID()
	if err != nil {
		return nil, fmt.Errorf("storage: generate object id: %w", err)
	}
	key := m.objectKey(in.Domain, in.ResourceID, id)

	blob, err := m.store.Put(ctx, key, data, size, in.ContentType)
	if err != nil {
		return nil, err
	}
	row, err := m.q.CreateStorageObject(ctx, repository.CreateStorageObjectParams{
		ID:           id,
		Key:          key,
		Domain:       in.Domain,
		ResourceID:   in.ResourceID,
		OriginalName: in.OriginalName,
		ContentType:  blob.ContentType,
		Size:         blob.Size,
		Etag:         blob.ETag,
		CreatedBy:    in.CreatedBy,
	})
	if err != nil {
		// Compensation: the index refused the row, so the blob must not
		// stay behind. A failed compensation leaves an orphan for the
		// reconciler; it is logged, never retried inline.
		if derr := m.store.Delete(ctx, key); derr != nil {
			m.log.Error("storage: upload compensation failed",
				"key", key, "delete_error", derr, "index_error", err)
		}
		return nil, fmt.Errorf("storage: index %q: %w", key, err)
	}
	return storedFileFromRow(row), nil
}

// Get returns the master-index metadata for the object.
func (m *FileManager) Get(ctx context.Context, id uuid.UUID) (*StoredFile, error) {
	row, err := m.q.GetStorageObjectByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("storage: get index: %w", err)
	}
	return storedFileFromRow(row), nil
}

// Open returns the metadata and the blob contents for download. The
// caller must close the returned Object.
func (m *FileManager) Open(ctx context.Context, id uuid.UUID) (*StoredFile, *Object, error) {
	meta, err := m.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	obj, err := m.store.Get(ctx, meta.Key)
	if err != nil {
		return nil, nil, err
	}
	return meta, obj, nil
}

// List returns the files of one resource, newest first.
func (m *FileManager) List(ctx context.Context, domain string, resourceID uuid.UUID) ([]StoredFile, error) {
	rows, err := m.q.ListStorageObjectsByResource(ctx, repository.ListStorageObjectsByResourceParams{
		Domain: domain, ResourceID: resourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: list index: %w", err)
	}
	out := make([]StoredFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, *storedFileFromRow(row))
	}
	return out, nil
}

// Delete removes the index row first; the blob is deleted afterwards.
// A failed blob delete leaves an orphan for Reconcile, so the file
// disappears from every view immediately.
func (m *FileManager) Delete(ctx context.Context, id uuid.UUID) error {
	row, err := m.q.GetStorageObjectByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("storage: get index: %w", err)
	}
	if err := m.q.DeleteStorageObject(ctx, id); err != nil {
		return fmt.Errorf("storage: delete index: %w", err)
	}
	if err := m.store.Delete(ctx, row.Key); err != nil {
		m.log.Warn("storage: blob delete left an orphan for the reconciler",
			"key", row.Key, "error", err)
	}
	return nil
}

// orphanGracePeriod is the tolerance window before an orphaned blob is
// removed. An upload in flight has already written the blob but not yet
// its index row; deleting such a blob would break the file the doctor
// just uploaded. Only blobs older than this window are treated as
// orphaned, so no in-flight upload is ever interrupted.
const orphanGracePeriod = time.Hour

// Reconcile is the saga's async compensation: it lists the blob store
// and removes every object whose key has no master-index row and whose
// LastModified is older than orphanGracePeriod (the time buffer).
// Younger blobs may be the first half of an upload whose index write is
// still in flight and are never touched. It returns the number of
// removed orphans.
func (m *FileManager) Reconcile(ctx context.Context) (int, error) {
	objects, err := m.store.List(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("storage: reconcile list: %w", err)
	}
	removed := 0
	for _, obj := range objects {
		if _, err := m.q.GetStorageObjectByKey(ctx, obj.Key); err == nil {
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			m.log.Warn("storage: reconcile lookup failed", "key", obj.Key, "error", err)
			continue
		}
		// Time buffer: a missing LastModified is treated as fresh (the
		// safe direction), never as an orphan.
		if obj.LastModified.IsZero() || time.Since(obj.LastModified) < orphanGracePeriod {
			continue
		}
		if err := m.store.Delete(ctx, obj.Key); err != nil {
			m.log.Warn("storage: reconcile delete failed", "key", obj.Key, "error", err)
			continue
		}
		removed++
	}
	if removed > 0 {
		m.log.Info("storage: reconciled orphaned objects", "removed", removed)
	}
	return removed, nil
}

func storedFileFromRow(row repository.StorageObject) *StoredFile {
	return &StoredFile{
		ID:           row.ID,
		Key:          row.Key,
		Domain:       row.Domain,
		ResourceID:   row.ResourceID,
		OriginalName: row.OriginalName,
		ContentType:  row.ContentType,
		Size:         row.Size,
		ETag:         row.Etag,
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt,
	}
}
