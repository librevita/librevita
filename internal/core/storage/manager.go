// This file implements the saga coordinator of the storage layer: the
// FileManager keeps the opaque blob Store and the database master index
// consistent through compensating actions, since no distributed
// transaction exists between them.
package storage

import (
	"context"
	"encoding/hex"
	"hash"
	"io"
	"log/slog"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"librevita.org/internal/core/crypto"
	"librevita.org/pkg/flow"
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
	// Checksum is the canonical BLAKE2b-256 digest of the payload,
	// witnessed in the audit trail at upload time.
	Checksum  string
	CreatedBy uuid.UUID
	CreatedAt time.Time
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

// IndexRepository defines the storage contract for master-index metadata.
type IndexRepository interface {
	Insert(ctx context.Context, f StoredFile) (*StoredFile, error)
	Get(ctx context.Context, id uuid.UUID) (*StoredFile, error)
	GetForResource(ctx context.Context, domain string, resourceID, id uuid.UUID) (*StoredFile, error)
	List(ctx context.Context, domain string, resourceID uuid.UUID) ([]StoredFile, error)
	Delete(ctx context.Context, id uuid.UUID) (string, error)
	KeyExists(ctx context.Context, key string) (bool, error)
}

// FileManager is the storage saga coordinator.
type FileManager struct {
	store Store
	repo  IndexRepository
	log   *slog.Logger
	// newID is injectable for tests; defaults to uuid.NewV7.
	newID func() (uuid.UUID, error)
}

// NewFileManager is the Fx provider.
func NewFileManager(repo IndexRepository, store Store, log *slog.Logger) (*FileManager, error) {
	if repo == nil {
		return nil, errors.New("storage: the master index requires the index repository")
	}
	return &FileManager{store: store, repo: repo, log: log, newID: uuid.NewV7}, nil
}

// Access classes of stored files.
const (
	ClassPublic  = "public"
	ClassPrivate = "private"
)

// publicDomains maps the domains whose files are public-class.
var publicDomains = map[string]bool{
	"avatar": true,
}

// objectKey derives the blob-store key from the index identity.
func (m *FileManager) objectKey(domain string, resourceID, id uuid.UUID) string {
	class := ClassPrivate
	if publicDomains[domain] {
		class = ClassPublic
	}
	return class + "/" + domain + "/" + resourceID.String() + "/" + id.String()
}

// GetForResource resolves an object only when it belongs to the given domain and resource.
func (m *FileManager) GetForResource(ctx context.Context, domain string, resourceID, id uuid.UUID) (*StoredFile, error) {
	return m.repo.GetForResource(ctx, domain, resourceID, id)
}

// OpenForResource returns the metadata and blob of an object that belongs to the given resource.
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

// OpenEncryptedForResource opens an encrypted patient object and exposes a
// streaming plaintext reader while retaining the index metadata.
func (m *FileManager) OpenEncryptedForResource(ctx context.Context, domain string, resourceID, id uuid.UUID, key, aad []byte) (*StoredFile, *Object, error) {
	meta, err := m.GetForResource(ctx, domain, resourceID, id)
	if err != nil {
		return nil, nil, err
	}
	obj, err := m.store.Get(ctx, meta.Key)
	if err != nil {
		return nil, nil, err
	}
	decrypted, err := NewDecryptedReader(obj.Data, key, aad)
	if err != nil {
		_ = obj.Data.Close()
		return nil, nil, err
	}
	return meta, &Object{
		ObjectInfo: ObjectInfo{
			Key:          meta.Key,
			Size:         meta.Size,
			ContentType:  meta.ContentType,
			ETag:         meta.ETag,
			Checksum:     meta.Checksum,
			LastModified: meta.CreatedAt,
		},
		Data: decrypted,
	}, nil
}

// Upload stores data in the blob store and registers the master-index row.
func (m *FileManager) Upload(ctx context.Context, in UploadInput, data io.Reader, size int64) (*StoredFile, error) {
	if in.Domain == "" {
		return nil, errors.New("storage: domain is required")
	}
	if in.OriginalName == "" {
		return nil, errors.New("storage: original name is required")
	}
	id, err := m.newID()
	if err != nil {
		return nil, errors.Wrap(err, "storage: generate object id")
	}
	key := m.objectKey(in.Domain, in.ResourceID, id)

	var blob ObjectInfo
	var created *StoredFile

	err = flow.New().
		StepWithRollback("store blob", func() error {
			var perr error
			blob, perr = m.store.Put(ctx, key, data, size, in.ContentType)
			return perr
		}, func() error {
			if derr := m.store.Delete(ctx, key); derr != nil {
				m.log.Error("storage: upload compensation failed",
					"key", key, "delete_error", derr)
				return derr
			}
			return nil
		}).
		Step("insert index", func() error {
			stFile := StoredFile{
				ID:           id,
				Key:          key,
				Domain:       in.Domain,
				ResourceID:   in.ResourceID,
				OriginalName: in.OriginalName,
				ContentType:  blob.ContentType,
				Size:         blob.Size,
				ETag:         blob.ETag,
				Checksum:     blob.Checksum,
				CreatedBy:    in.CreatedBy,
			}
			var ierr error
			created, ierr = m.repo.Insert(ctx, stFile)
			if ierr != nil {
				return errors.Wrapf(ierr, "storage: index %q", key)
			}
			return nil
		}).
		Err()

	if err != nil {
		return nil, err
	}
	return created, nil
}

// UploadEncrypted streams a patient-owned plaintext through the chunked
// XChaCha20-Poly1305 envelope before sending it to the blob store. The index
// keeps the plaintext size and checksum so callers see the same metadata as
// the unencrypted Store API.
func (m *FileManager) UploadEncrypted(ctx context.Context, in UploadInput, data io.Reader, size int64, key, aad []byte) (*StoredFile, error) {
	if in.Domain == "" {
		return nil, errors.New("storage: domain is required")
	}
	if in.OriginalName == "" {
		return nil, errors.New("storage: original name is required")
	}
	if data == nil {
		return nil, errors.New("storage: data is required")
	}
	if size < -1 {
		return nil, errors.New("storage: invalid size")
	}
	id, err := m.newID()
	if err != nil {
		return nil, errors.Wrap(err, "storage: generate object id")
	}
	keyName := m.objectKey(in.Domain, in.ResourceID, id)

	checksum, err := crypto.NewDigest()
	if err != nil {
		return nil, errors.Wrap(err, "storage: checksum")
	}
	source := io.Reader(data)
	if size >= 0 {
		source = io.LimitReader(data, size+1)
	}
	hashed := &hashingReader{source: source, hash: checksum}
	encrypted, err := NewEncryptedReader(hashed, key, aad)
	if err != nil {
		return nil, errors.Wrap(err, "storage: encrypt upload")
	}
	defer func() { _ = encrypted.Close() }()

	var blob ObjectInfo
	var created *StoredFile

	err = flow.New().
		StepWithRollback("store encrypted blob", func() error {
			var perr error
			blob, perr = m.store.Put(ctx, keyName, encrypted, EncryptedSize(size), in.ContentType)
			if perr != nil {
				return perr
			}
			if size >= 0 && hashed.n != size {
				return errors.Newf("storage: encrypted upload size mismatch: got %d, want %d", hashed.n, size)
			}
			return nil
		}, func() error {
			if derr := m.store.Delete(ctx, keyName); derr != nil {
				m.log.Error("storage: encrypted upload compensation failed",
					"key", keyName, "delete_error", derr)
				return derr
			}
			return nil
		}).
		Step("insert encrypted index", func() error {
			stFile := StoredFile{
				ID:           id,
				Key:          keyName,
				Domain:       in.Domain,
				ResourceID:   in.ResourceID,
				OriginalName: in.OriginalName,
				ContentType:  in.ContentType,
				Size:         hashed.n,
				ETag:         blob.ETag,
				Checksum:     hex.EncodeToString(checksum.Sum(nil)),
				CreatedBy:    in.CreatedBy,
			}
			var ierr error
			created, ierr = m.repo.Insert(ctx, stFile)
			if ierr != nil {
				return errors.Wrapf(ierr, "storage: index %q", keyName)
			}
			return nil
		}).
		Err()

	if err != nil {
		return nil, err
	}
	return created, nil
}

// Get returns the master-index metadata for the object.
func (m *FileManager) Get(ctx context.Context, id uuid.UUID) (*StoredFile, error) {
	return m.repo.Get(ctx, id)
}

// Open returns the metadata and the blob contents for download.
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
	return m.repo.List(ctx, domain, resourceID)
}

// Delete removes the index row first; the blob is deleted afterwards.
func (m *FileManager) Delete(ctx context.Context, id uuid.UUID) error {
	key, err := m.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if err := m.store.Delete(ctx, key); err != nil {
		m.log.Warn("storage: blob delete left an orphan for the reconciler",
			"key", key, "error", err)
	}
	return nil
}

const orphanGracePeriod = time.Hour

// Reconcile is the saga's async compensation.
func (m *FileManager) Reconcile(ctx context.Context) (int, error) {
	objects, err := m.store.List(ctx, "")
	if err != nil {
		return 0, errors.Wrap(err, "storage: reconcile list")
	}
	removed := 0
	for _, obj := range objects {
		exists, err := m.repo.KeyExists(ctx, obj.Key)
		if err != nil {
			m.log.Warn("storage: reconcile lookup failed", "key", obj.Key, "error", err)
			continue
		}
		if exists {
			continue
		}
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

type hashingReader struct {
	source io.Reader
	hash   hash.Hash
	n      int64
}

func (r *hashingReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if n > 0 {
		_, _ = r.hash.Write(p[:n])
		r.n += int64(n)
	}
	return n, err
}
