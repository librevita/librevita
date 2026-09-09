package acme

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net"
	"strings"
	"time"

	"librevita.org/pkg/errors"
)

// RFC2136Config holds connection and TSIG parameters for RFC 2136 dynamic DNS updates.
type RFC2136Config struct {
	Nameserver    string
	Zone          string
	TSIGKeyName   string
	TSIGSecret    string
	TSIGAlgorithm string
}

// RFC2136Provider implements DNSProvider for RFC 2136 dynamic updates.
type RFC2136Provider struct {
	cfg RFC2136Config
}

// NewRFC2136Provider creates a new RFC 2136 provider.
func NewRFC2136Provider(cfg RFC2136Config) *RFC2136Provider {
	if cfg.TSIGAlgorithm == "" {
		cfg.TSIGAlgorithm = "hmac-sha256"
	}
	if !strings.Contains(cfg.Nameserver, ":") {
		cfg.Nameserver = net.JoinHostPort(cfg.Nameserver, "53")
	}
	return &RFC2136Provider{cfg: cfg}
}

const (
	dnsOpcodeUpdate = 5
	dnsTypeSOA      = 6
	dnsTypeTXT      = 16
	dnsTypeTSIG     = 250
	dnsTypeANY      = 255
	dnsClassIN      = 1
	dnsClassANY     = 255
	dnsClassNONE    = 254
)

func (p *RFC2136Provider) Present(ctx context.Context, domain, keyAuthRecord string) error {
	recordName := ChallengeRecordName(domain)
	zone := p.cfg.Zone
	if zone == "" {
		zone = deriveZone(domain)
	}
	return p.sendUpdate(ctx, zone, recordName, keyAuthRecord, true)
}

func (p *RFC2136Provider) CleanUp(ctx context.Context, domain, keyAuthRecord string) error {
	recordName := ChallengeRecordName(domain)
	zone := p.cfg.Zone
	if zone == "" {
		zone = deriveZone(domain)
	}
	return p.sendUpdate(ctx, zone, recordName, keyAuthRecord, false)
}

func deriveZone(domain string) string {
	d := strings.TrimPrefix(domain, "*.")
	d = strings.TrimPrefix(d, ".")
	if !strings.HasSuffix(d, ".") {
		d += "."
	}
	return d
}

func (p *RFC2136Provider) sendUpdate(ctx context.Context, zone, recordName, txtVal string, isAdd bool) error {
	var buf bytes.Buffer
	// Header: ID (2), Flags (2), QDCOUNT/ZOCOUNT (2), ANCOUNT/PRCOUNT (2), NSCOUNT/UPCOUNT (2), ARCOUNT (2)
	id := uint16(time.Now().UnixNano() & 0xFFFF)
	flags := uint16(dnsOpcodeUpdate << 11) // Opcode 5 (Update), non-authoritative client query

	_ = binary.Write(&buf, binary.BigEndian, id)
	_ = binary.Write(&buf, binary.BigEndian, flags)
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // ZOCOUNT = 1
	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // PRCOUNT = 0
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // UPCOUNT = 1

	hasTSIG := p.cfg.TSIGSecret != "" && p.cfg.TSIGKeyName != ""
	if hasTSIG {
		_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // ARCOUNT = 1 (TSIG)
	} else {
		_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // ARCOUNT = 0
	}

	// Zone section: Zone name (wire format), Type SOA, Class IN
	writeDNSName(&buf, zone)
	_ = binary.Write(&buf, binary.BigEndian, uint16(dnsTypeSOA))
	_ = binary.Write(&buf, binary.BigEndian, uint16(dnsClassIN))

	// Update section
	writeDNSName(&buf, recordName)
	if isAdd {
		// Add TXT record: Type TXT, Class IN, TTL 120, RDLENGTH, RDATA
		_ = binary.Write(&buf, binary.BigEndian, uint16(dnsTypeTXT))
		_ = binary.Write(&buf, binary.BigEndian, uint16(dnsClassIN))
		_ = binary.Write(&buf, binary.BigEndian, uint32(120))
		txtBytes := []byte(txtVal)
		if len(txtBytes) > 255 {
			return errors.New("acme rfc2136: txt record value exceeds 255 bytes")
		}
		rdLen := uint16(1 + len(txtBytes)) // #nosec G115 -- bounded by 255 check above
		_ = binary.Write(&buf, binary.BigEndian, rdLen)
		buf.WriteByte(byte(len(txtBytes))) // #nosec G115 -- bounded by 255 check above
		buf.Write(txtBytes)
	} else {
		// Delete RRset: Type TXT, Class ANY, TTL 0, RDLENGTH 0
		_ = binary.Write(&buf, binary.BigEndian, uint16(dnsTypeTXT))
		_ = binary.Write(&buf, binary.BigEndian, uint16(dnsClassANY))
		_ = binary.Write(&buf, binary.BigEndian, uint32(0))
		_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	}

	rawMsg := buf.Bytes()
	if hasTSIG {
		signedMsg, err := p.signTSIG(rawMsg)
		if err != nil {
			return err
		}
		rawMsg = signedMsg
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", p.cfg.Nameserver)
	if err != nil {
		return errors.Wrap(err, "acme rfc2136: dial nameserver")
	}
	defer func() {
		_ = conn.Close()
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	if _, err := conn.Write(rawMsg); err != nil {
		return errors.Wrap(err, "acme rfc2136: write update")
	}

	resp := make([]byte, 512)
	n, err := conn.Read(resp)
	if err != nil {
		return errors.Wrap(err, "acme rfc2136: read response")
	}
	if n < 12 {
		return errors.New("acme rfc2136: response truncated")
	}

	respFlags := binary.BigEndian.Uint16(resp[2:4])
	rcode := respFlags & 0x000F
	if rcode != 0 {
		return errors.Newf("acme rfc2136: server returned rcode %d", rcode)
	}
	return nil
}

func (p *RFC2136Provider) signTSIG(msg []byte) ([]byte, error) {
	secret, err := base64.StdEncoding.DecodeString(p.cfg.TSIGSecret)
	if err != nil {
		return nil, errors.Wrap(err, "acme rfc2136: decode tsig secret")
	}

	keyName := p.cfg.TSIGKeyName
	algoName := "hmac-sha256."

	unixSec := time.Now().Unix()
	if unixSec < 0 {
		unixSec = 0
	}
	now := uint64(unixSec)
	timeSignedHigh := uint16(now >> 32)       // #nosec G115 -- TSIG time upper 16 bits
	timeSignedLow := uint32(now & 0xFFFFFFFF) // #nosec G115 -- TSIG time lower 32 bits
	fudge := uint16(300)

	var macBuf bytes.Buffer
	macBuf.Write(msg)
	writeDNSName(&macBuf, keyName)
	_ = binary.Write(&macBuf, binary.BigEndian, uint16(dnsClassANY))
	_ = binary.Write(&macBuf, binary.BigEndian, uint32(0)) // TTL
	writeDNSName(&macBuf, algoName)
	_ = binary.Write(&macBuf, binary.BigEndian, timeSignedHigh)
	_ = binary.Write(&macBuf, binary.BigEndian, timeSignedLow)
	_ = binary.Write(&macBuf, binary.BigEndian, fudge)
	_ = binary.Write(&macBuf, binary.BigEndian, uint16(0)) // Error
	_ = binary.Write(&macBuf, binary.BigEndian, uint16(0)) // Other Len

	mac := hmac.New(sha256.New, secret)
	mac.Write(macBuf.Bytes())
	digest := mac.Sum(nil)

	var tsigRR bytes.Buffer
	writeDNSName(&tsigRR, keyName)
	_ = binary.Write(&tsigRR, binary.BigEndian, uint16(dnsTypeTSIG))
	_ = binary.Write(&tsigRR, binary.BigEndian, uint16(dnsClassANY))
	_ = binary.Write(&tsigRR, binary.BigEndian, uint32(0))

	var rdata bytes.Buffer
	writeDNSName(&rdata, algoName)
	_ = binary.Write(&rdata, binary.BigEndian, timeSignedHigh)
	_ = binary.Write(&rdata, binary.BigEndian, timeSignedLow)
	_ = binary.Write(&rdata, binary.BigEndian, fudge)
	_ = binary.Write(&rdata, binary.BigEndian, uint16(len(digest))) // #nosec G115 -- sha256 digest is always 32 bytes
	rdata.Write(digest)
	origID := binary.BigEndian.Uint16(msg[0:2])
	_ = binary.Write(&rdata, binary.BigEndian, origID)
	_ = binary.Write(&rdata, binary.BigEndian, uint16(0)) // Error
	_ = binary.Write(&rdata, binary.BigEndian, uint16(0)) // Other len

	_ = binary.Write(&tsigRR, binary.BigEndian, uint16(rdata.Len())) // #nosec G115 -- TSIG RDATA is bounded under 128 bytes
	tsigRR.Write(rdata.Bytes())

	res := make([]byte, len(msg)+tsigRR.Len())
	copy(res, msg)
	copy(res[len(msg):], tsigRR.Bytes())
	return res, nil
}

func writeDNSName(buf *bytes.Buffer, name string) {
	clean := strings.Trim(name, ".")
	if clean == "" {
		buf.WriteByte(0)
		return
	}
	parts := strings.Split(clean, ".")
	for _, p := range parts {
		if len(p) > 63 {
			p = p[:63]
		}
		buf.WriteByte(byte(len(p))) // #nosec G115 -- DNS label length is bounded by 63
		buf.WriteString(p)
	}
	buf.WriteByte(0)
}
