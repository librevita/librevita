package crypto_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/crypto"
)

func TestKVURNs(t *testing.T) {
	clinicID := uuid.MustParse("01990000-0000-7000-8000-0000000000c1")
	assert.Equal(t, "urn:librevita:meta:setup_completed", crypto.MetaURN("setup_completed"))
	assert.Equal(t, "urn:librevita:clinic:"+clinicID.String()+":session:blake2s$abc",
		crypto.ClinicSessionURN(clinicID, "blake2s$abc"))
	assert.Equal(t, "urn:librevita:platform:session:blake2s$abc",
		crypto.PlatformSessionURN("blake2s$abc"))
}
