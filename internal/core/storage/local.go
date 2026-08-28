// This file implements the directory backend of the storage port:
// objects are written atomically (temp file + rename) so a crash never
// leaves a truncated object, and each object has a sidecar metadata
// file under .meta/ with the content type and ETag. Keys are validated
// before touching the filesystem, so traversal outside the root is
// impossible even with hostile keys.
package storage

import (
	"context"
	// #nosec G501 -- the ETag MD5 is required for S3 interoperability;
	// content integrity uses BLAKE2b (ObjectInfo.Checksum).
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/crypto"
)

const metaDir = ".meta"

// Local is the directory-backed implementation of Store.
type Local struct {
	root string
}

// NewLocal creates the store root (and the metadata directory) and
// returns the backend. The directory is created with 0750.
func NewLocal(dir string) (*Local, error) {
	if dir == "" {
		return nil, errors.New("storage: local directory is required")
	}
	if err := os.MkdirAll(filepath.Join(dir, metaDir), 0o750); err != nil {
		return nil, errors.Wrap(err, "storage: create root")
	}
	return &Local{root: dir}, nil
}

// Root returns the configured directory, for diagnostics.
func (s *Local) Root() string { return s.root }

// path resolves a validated key into the file path inside the root.
func (s *Local) path(key string) (string, error) {
	if err := ValidKey(key); err != nil {
		return "", err
	}
	p := filepath.Join(s.root, filepath.FromSlash(key))
	if p != s.root && !strings.HasPrefix(p, s.root+string(filepath.Separator)) {
		return "", errors.New("storage: key escapes the root")
	}
	return p, nil
}

// Put stores data atomically: the payload is streamed to a temp file in
// the target directory (same filesystem, so rename is atomic) while the
// MD5 is computed for the ETag, then renamed into place. The sidecar
// metadata is written last.
func (s *Local) Put(ctx context.Context, key string, data io.Reader, size int64, contentType string) (ObjectInfo, error) {
	p, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return ObjectInfo{}, errors.Wrap(err, "storage: create parent")
	}

	tmp, err := os.CreateTemp(filepath.Dir(p), ".lv-upload-*")
	if err != nil {
		return ObjectInfo{}, errors.Wrap(err, "storage: create temp")
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	// #nosec G401 -- MD5 for the S3-style ETag only; integrity is BLAKE2b/BLAKE2s via crypto.
	etagHash := md5.New()
	checksum, err := crypto.NewDigest()
	if err != nil {
		tmp.Close()
		return ObjectInfo{}, errors.Wrap(err, "storage: checksum")
	}
	written, err := io.Copy(io.MultiWriter(tmp, etagHash, checksum), data)
	if err != nil {
		tmp.Close()
		return ObjectInfo{}, errors.Wrap(err, "storage: write")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return ObjectInfo{}, errors.Wrap(err, "storage: sync")
	}
	if err := tmp.Close(); err != nil {
		return ObjectInfo{}, errors.Wrap(err, "storage: close temp")
	}
	if err := os.Rename(tmpName, p); err != nil {
		return ObjectInfo{}, errors.Wrap(err, "storage: rename")
	}

	info := ObjectInfo{
		Key:          key,
		Size:         written,
		ContentType:  contentType,
		ETag:         hex.EncodeToString(etagHash.Sum(nil)),
		Checksum:     hex.EncodeToString(checksum.Sum(nil)),
		LastModified: time.Now().UTC(),
	}
	if err := s.writeMeta(key, info); err != nil {
		// The object is already stored; a broken sidecar degrades
		// metadata only, so surface it but keep the object.
		return info, errors.Wrap(err, "storage: metadata")
	}
	return info, nil
}

// Get returns the object contents and metadata.
func (s *Local) Get(ctx context.Context, key string) (*Object, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	// #nosec G304 -- key was validated by ValidKey (path traversal is
	// rejected before reaching here).
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "storage: open")
	}
	info, err := s.stat(key, p)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Object{ObjectInfo: info, Data: f}, nil
}

// Delete removes the object and its sidecar. Missing objects are not
// an error (idempotent, like the S3 backend).
func (s *Local) Delete(ctx context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	_ = os.Remove(p)
	_ = os.Remove(s.metaPath(key))
	return nil
}

// Stat returns the metadata for key.
func (s *Local) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	p, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	return s.stat(key, p)
}

// stat merges the sidecar metadata with the file stat (size and
// modification time are authoritative from the file).
func (s *Local) stat(key, p string) (ObjectInfo, error) {
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return ObjectInfo{}, ErrNotFound
	}
	if err != nil {
		return ObjectInfo{}, errors.Wrap(err, "storage: stat")
	}
	info := ObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		LastModified: fi.ModTime().UTC(),
	}
	if meta, err := s.readMeta(key); err == nil {
		info.ContentType = meta.ContentType
		info.ETag = meta.ETag
		info.Checksum = meta.Checksum
	}
	return info, nil
}

// List returns the objects under prefix, sorted by key. Hidden
// metadata and temp files are skipped.
func (s *Local) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	base := filepath.Join(s.root, filepath.FromSlash(prefix))
	var out []ObjectInfo
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Never descend into the metadata directory.
			if d.Name() == metaDir && p != s.root {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".lv-upload-") {
			return nil
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		info, err := s.stat(key, p)
		if err != nil {
			return err
		}
		out = append(out, info)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, errors.Wrap(err, "storage: list")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *Local) metaPath(key string) string {
	return filepath.Join(s.root, metaDir, filepath.FromSlash(key)+".json")
}

func (s *Local) writeMeta(key string, info ObjectInfo) error {
	p := s.metaPath(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s *Local) readMeta(key string) (ObjectInfo, error) {
	data, err := os.ReadFile(s.metaPath(key))
	if err != nil {
		return ObjectInfo{}, err
	}
	var info ObjectInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ObjectInfo{}, err
	}
	return info, nil
}
