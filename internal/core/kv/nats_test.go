package kv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenNATS_Validation(t *testing.T) {
	_, err := OpenNATS("", "bucket")
	assert.Error(t, err)

	_, err = OpenNATS("nats://localhost:4222", "")
	assert.Error(t, err)

	_, err = OpenNATS("nats://127.0.0.1:65530", "testbucket")
	assert.Error(t, err)
}

func TestDecodeNATSKey_Invalid(t *testing.T) {
	// Missing prefix
	_, ok := decodeNATSKey("not_prefixed")
	assert.False(t, ok)

	// Invalid base64
	_, ok = decodeNATSKey("k_!!!invalid-base64!!!")
	assert.False(t, ok)

	// Valid roundtrip
	orig := "clinic:patient:123"
	encoded := natsKey(orig)
	decoded, ok := decodeNATSKey(encoded)
	assert.True(t, ok)
	assert.Equal(t, orig, decoded)
}
