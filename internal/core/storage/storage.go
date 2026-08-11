// Package storage is the file storage port of LibreVita. Medical files
// (documents, images, exports) are stored as opaque objects addressed
// by slash-separated keys such as "patients/<id>/prescription.pdf"; the
// domain layer owns the key layout.
//
// Two backends implement the port:
//
//   - local: a directory on the server filesystem.
//   - s3: any S3-compatible API (MinIO, Garage, Ceph, ...), not
//     necessarily AWS.
//
// The backend is selected through storage.backend in the configuration.
package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

// ErrNotFound is returned when the object does not exist. Use
// IsNotFound to test errors from any backend.
var ErrNotFound = errors.New("storage: object not found")

// IsNotFound reports whether err is a not-found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// ObjectInfo describes a stored object without its contents.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
	// Checksum is the canonical application-level digest (BLAKE2b-256,
	// hex) of the whole payload. Backend ETags are not comparable (S3
	// multipart ETags digest the parts, not the object), so this value
	// is computed by the application in every backend.
	Checksum     string
	LastModified time.Time
}

// Object is a stored object with its contents.
type Object struct {
	ObjectInfo
	// Data is the object payload; the caller must close it.
	Data io.ReadCloser
}

// Store persists objects. Implementations must be safe for concurrent
// use. Put overwrites the object at key; Delete is idempotent; Get and
// Stat return ErrNotFound for missing objects.
type Store interface {
	// Put stores data at key. size may be -1 when unknown.
	Put(ctx context.Context, key string, data io.Reader, size int64, contentType string) (ObjectInfo, error)
	// Get returns the object and its metadata.
	Get(ctx context.Context, key string) (*Object, error)
	// Delete removes the object. Removing a missing object is not an
	// error.
	Delete(ctx context.Context, key string) error
	// Stat returns the object metadata without its contents.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// List returns the objects under prefix, sorted by key.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// ValidKey rejects keys that would escape the storage root (path
// traversal) or confuse the key layout: empty keys, absolute paths,
// backslashes, and empty or dot path segments.
func ValidKey(key string) error {
	switch {
	case key == "":
		return errors.New("storage: empty key")
	case strings.HasPrefix(key, "/"):
		return errors.New("storage: key must be relative")
	case strings.Contains(key, "\\"):
		return errors.New("storage: key must use forward slashes")
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("storage: invalid key segment")
		}
	}
	return nil
}
