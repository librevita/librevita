package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
		return nil, fmt.Errorf("vault: nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("vault: nats jetstream init: %w", err)
	}

	ctx := context.Background()
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("vault: nats kv bucket init: %w", err)
	}

	return &NATSVault{nc: nc, kv: kv}, nil
}

// PutDEK stores the encrypted DEK bytes indexed by patientURN.
func (v *NATSVault) PutDEK(ctx context.Context, patientURN string, encryptedDEK []byte) error {
	key := sanitizeNATSKey(patientURN)
	if _, err := v.kv.Put(ctx, key, encryptedDEK); err != nil {
		return fmt.Errorf("vault: nats put: %w", err)
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
		return nil, fmt.Errorf("vault: nats get: %w", err)
	}
	val := make([]byte, len(entry.Value()))
	copy(val, entry.Value())
	return val, nil
}

// DeleteDEK purges the patient's DEK from storage, performing instant Crypto-Shredding.
func (v *NATSVault) DeleteDEK(ctx context.Context, patientURN string) error {
	key := sanitizeNATSKey(patientURN)
	// Purge removes all history and tombstones for the key, executing physical shredding.
	if err := v.kv.Purge(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("vault: nats purge: %w", err)
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
	r := strings.NewReplacer(":", "_", "/", "_", ".", "_", " ", "_")
	return r.Replace(urn)
}
