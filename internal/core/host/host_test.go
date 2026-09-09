package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		host   string
		base   string
		kind   Kind
		domain string
		err    bool
	}{
		{name: "apex", host: "lv.test", base: "lv.test", kind: KindApex},
		{name: "www", host: "www.lv.test", base: "lv.test", kind: KindApex},
		{name: "clinic custom domain", host: "clinicasaojose.com.br", base: "lv.test", kind: KindClinic, domain: "clinicasaojose.com.br"},
		{name: "clinic subdomain of base", host: "norte.lv.test", base: "lv.test", kind: KindClinic, domain: "norte.lv.test"},
		{name: "port", host: "clinica.med.br:8080", base: "lv.test", kind: KindClinic, domain: "clinica.med.br"},
		{name: "case", host: "Clinica.Org", base: "lv.test", kind: KindClinic, domain: "clinica.org"},
		{name: "invalid host chars", host: "bad@domain.com", base: "lv.test", err: true},
		{name: "invalid dashes", host: "-bad.com", base: "lv.test", err: true},
		{name: "empty", host: "", base: "lv.test", err: true},
		{name: "empty base", host: "lv.test", base: "", err: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Classify(tc.host, tc.base)
			if tc.err {
				assert.ErrorIs(t, err, ErrInvalidHost)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.kind, got.Kind)
			assert.Equal(t, tc.domain, got.Domain)
		})
	}
}
