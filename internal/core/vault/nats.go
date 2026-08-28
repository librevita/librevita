package vault

import (
	"context"
	"encoding/base64"

	"github.com/cockroachdb/errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"librevita.org/internal/core/crypto"
)

// NATSVault implements crypto.KeyVault using NATS JetStream KeyValue store.
type NATSVault struct {
	nc *nats.Conn
	kv jetstream.KeyValue
}

// NewNATSVault initializes connection to NATS server and opens or creates JetStream KeyValue bucket.
func NewNATSVault(url, bucketName string) (*NATSVault, error) {
	if url == "" {
		return nil, errors.New("vault: nats url is required")
	}
	if bucketName == "" {
		bucketName = "patient_deks"
	}

	nc, err := nats.Connect(url)
	if err != nil {
		return nil, errors.Wrap(err, "vault: nats connect")
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, errors.Wrap(err, "vault: nats jetstream init")
	}

	ctx := context.Background()
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
	})
	if err != nil {
		nc.Close()
		return nil, errors.Wrap(err, "vault: nats kv bucket init")
	}

	return &NATSVault{nc: nc, kv: kv}, nil
}

// PutDEK stores the encrypted DEK bytes indexed by patientURN.
func (v *NATSVault) PutDEK(ctx context.Context, patientURN string, encryptedDEK []byte) error {
	key := sanitizeNATSKey(patientURN)
	if entry, err := v.kv.Get(ctx, key); err == nil {
		if crypto.IsDestroyedDEK(entry.Value()) {
			return crypto.ErrKeyDestroyed
		}
	} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return errors.Wrap(err, "vault: nats check existing")
	}
	if _, err := v.kv.Put(ctx, key, encryptedDEK); err != nil {
		return errors.Wrap(err, "vault: nats put")
	}
	return nil
}

// GetDEK retrieves the encrypted DEK bytes for patientURN.
// Returns crypto.ErrKeyNotFound if key does not exist.
func (v *NATSVault) GetDEK(ctx context.Context, patientURN string) ([]byte, error) {
	key := sanitizeNATSKey(patientURN)
	entry, err := v.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, crypto.ErrKeyNotFound
		}
		return nil, errors.Wrap(err, "vault: nats get")
	}
	if crypto.IsDestroyedDEK(entry.Value()) {
		return nil, crypto.ErrKeyDestroyed
	}
	val := make([]byte, len(entry.Value()))
	copy(val, entry.Value())
	return val, nil
}

// GetDEKs retrieves multiple wrapped DEKs with bounded concurrent JetStream
// requests. NATS KV does not expose a native multi-get operation.
func (v *NATSVault) GetDEKs(ctx context.Context, patientURNs []string) (map[string]crypto.DEKResult, error) {
	return batchGetWithWorkers(ctx, patientURNs, defaultBatchWorkers, v.GetDEK)
}

// PutIfAbsent creates a wrapped DEK without replacing an existing value.
func (v *NATSVault) PutIfAbsent(ctx context.Context, patientURN string, encryptedDEK []byte) (bool, error) {
	key := sanitizeNATSKey(patientURN)
	_, err := v.kv.Create(ctx, key, encryptedDEK)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyExists) {
		if _, getErr := v.GetDEK(ctx, patientURN); getErr != nil {
			if errors.Is(getErr, crypto.ErrKeyDestroyed) {
				return false, getErr
			}
			return false, errors.Wrap(getErr, "vault: nats verify existing")
		}
		return false, nil
	}
	return false, errors.Wrap(err, "vault: nats create")
}

// DeleteDEK purges the patient's DEK from storage and writes a terminal
// tombstone, performing instant Crypto-Shredding without allowing recreation.
func (v *NATSVault) DeleteDEK(ctx context.Context, patientURN string) error {
	key := sanitizeNATSKey(patientURN)
	// Purge removes all history and tombstones for the key, executing physical shredding.
	if err := v.kv.Purge(ctx, key); err != nil {
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return errors.Wrap(err, "vault: nats purge")
		}
	}
	if _, err := v.kv.Put(ctx, key, crypto.DestroyedDEKMarker()); err != nil {
		return errors.Wrap(err, "vault: nats tombstone")
	}
	return nil
}

// Close closes the underlying NATS connection.
func (v *NATSVault) Close() error {
	v.nc.Close()
	return nil
}

// sanitizeNATSKey replaces characters invalid in NATS subjects with underscores.
func sanitizeNATSKey(urn string) string {
	return "k_" + base64.RawURLEncoding.EncodeToString([]byte(urn))
}
