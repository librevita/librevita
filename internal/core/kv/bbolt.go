package kv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"go.etcd.io/bbolt"
)

var bucketName = []byte("data")

// BBoltStore is a Store backed by one bbolt file.
type BBoltStore struct {
	db *bbolt.DB
}

// OpenBBolt creates or opens a bbolt database at path.
func OpenBBolt(path string) (*BBoltStore, error) {
	if path == "" {
		return nil, errors.New("kv: bbolt path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, errors.Wrap(err, "kv: mkdir")
	}

	db, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, errors.Wrap(err, "kv: open bbolt")
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, errors.Wrap(err, "kv: init bucket")
	}

	return &BBoltStore{db: db}, nil
}

func (s *BBoltStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var val []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		data := b.Get([]byte(key))
		if data == nil {
			return ErrNotFound
		}
		val = bytes.Clone(data)
		return nil
	})
	return val, err
}

func (s *BBoltStore) GetMany(ctx context.Context, keys []string) (map[string]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	unique := uniqueKeys(keys)
	results := make(map[string]Result, len(unique))
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		for _, key := range unique {
			if err := ctx.Err(); err != nil {
				return err
			}
			data := b.Get([]byte(key))
			if data == nil {
				results[key] = Result{Err: ErrNotFound}
				continue
			}
			results[key] = Result{Value: bytes.Clone(data)}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (s *BBoltStore) Put(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketName).Put([]byte(key), value)
	})
}

func (s *BBoltStore) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	created := false
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b.Get([]byte(key)) != nil {
			return nil
		}
		if err := b.Put([]byte(key), value); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

func (s *BBoltStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketName).Delete([]byte(key))
	})
}

func (s *BBoltStore) ListPrefix(ctx context.Context, prefix string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var entries []Entry
	pre := []byte(prefix)
	err := s.db.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket(bucketName).Cursor()
		for k, v := c.Seek(pre); k != nil && bytes.HasPrefix(k, pre); k, v = c.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries = append(entries, Entry{Key: string(k), Value: bytes.Clone(v)})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}

func (s *BBoltStore) Close() error {
	return s.db.Close()
}
