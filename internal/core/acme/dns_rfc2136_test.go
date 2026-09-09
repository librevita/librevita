package acme

import (
	"context"
	"encoding/binary"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveZone(t *testing.T) {
	assert.Equal(t, "example.org.", deriveZone("example.org"))
	assert.Equal(t, "example.org.", deriveZone("*.example.org"))
	assert.Equal(t, "sub.example.org.", deriveZone("sub.example.org."))
}

func TestRFC2136Provider_WireUpdate(t *testing.T) {
	ctx := context.Background()

	// Start local mock UDP DNS server
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = pc.Close() }()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 12 {
				continue
			}
			// Respond with DNS success (rcode 0, response bit set)
			resp := make([]byte, 12)
			copy(resp[0:2], buf[0:2])                     // Copy ID
			binary.BigEndian.PutUint16(resp[2:4], 0x8000) // QR=1 (response), RCODE=0
			_, _ = pc.WriteTo(resp, addr)
		}
	}()

	p := NewRFC2136Provider(RFC2136Config{
		Nameserver:    pc.LocalAddr().String(),
		Zone:          "example.org.",
		TSIGKeyName:   "key.",
		TSIGSecret:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		TSIGAlgorithm: "hmac-sha256",
	})

	err = p.Present(ctx, "example.org", "val123")
	assert.NoError(t, err)

	err = p.CleanUp(ctx, "example.org", "val123")
	assert.NoError(t, err)
}
