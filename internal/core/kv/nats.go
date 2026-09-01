package kv

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSStore is a Store backed by a JetStream KeyValue bucket.
type NATSStore struct {
	nc *nats.Conn
	kv jetstream.KeyValue
}

// OpenNATS connects to NATS and opens or creates the bucket.
func OpenNATS(url, bucketName string) (*NATSStore, error) {
	if url == "" {
		return nil, errors.New("kv: nats url is required")
	}
	if bucketName == "" {
		return nil, errors.New("kv: nats bucket is required")
	}

	nc, err := nats.Connect(url)
	if err != nil {
		return nil, errors.Wrap(err, "kv: nats connect")
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, errors.Wrap(err, "kv: nats jetstream init")
	}

	ctx := context.Background()
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
	})
	if err != nil {
		nc.Close()
		return nil, errors.Wrap(err, "kv: nats kv bucket init")
	}

	return &NATSStore{nc: nc, kv: kv}, nil
}

func natsKey(key string) string {
	return "k_" + base64.RawURLEncoding.EncodeToString([]byte(key))
}

func decodeNATSKey(encoded string) (string, bool) {
	raw, ok := strings.CutPrefix(encoded, "k_")
	if !ok {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", false
	}
	return string(decoded), true
}

func (s *NATSStore) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := s.kv.Get(ctx, natsKey(key))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, errors.Wrap(err, "kv: nats get")
	}
	return bytesClone(entry.Value()), nil
}

func (s *NATSStore) GetMany(ctx context.Context, keys []string) (map[string]Result, error) {
	return batchGetWithWorkers(ctx, keys, defaultBatchWorkers, s.Get)
}

func (s *NATSStore) Put(ctx context.Context, key string, value []byte) error {
	if _, err := s.kv.Put(ctx, natsKey(key), value); err != nil {
		return errors.Wrap(err, "kv: nats put")
	}
	return nil
}

func (s *NATSStore) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	_, err := s.kv.Create(ctx, natsKey(key), value)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyExists) {
		return false, nil
	}
	return false, errors.Wrap(err, "kv: nats create")
}

func (s *NATSStore) Delete(ctx context.Context, key string) error {
	err := s.kv.Delete(ctx, natsKey(key))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return errors.Wrap(err, "kv: nats delete")
	}
	return nil
}

func (s *NATSStore) Shred(ctx context.Context, key string) error {
	err := s.kv.Purge(ctx, natsKey(key))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return errors.Wrap(err, "kv: nats purge")
	}
	return nil
}

func (s *NATSStore) ListPrefix(ctx context.Context, prefix string) ([]Entry, error) {
	keys, err := s.kv.Keys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return []Entry{}, nil
		}
		return nil, errors.Wrap(err, "kv: nats keys")
	}
	var entries []Entry
	for _, encoded := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		logical, ok := decodeNATSKey(encoded)
		if !ok || !strings.HasPrefix(logical, prefix) {
			continue
		}
		value, err := s.Get(ctx, logical)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		entries = append(entries, Entry{Key: logical, Value: value})
	}
	if entries == nil {
		entries = []Entry{}
	}
	return entries, nil
}

func (s *NATSStore) Close() error {
	s.nc.Close()
	return nil
}

func bytesClone(in []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

var _ Shredder = (*NATSStore)(nil)
