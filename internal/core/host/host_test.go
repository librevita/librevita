package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		host string
		base string
		kind Kind
		slug string
		err  bool
	}{
		{name: "apex", host: "lv.test", base: "lv.test", kind: KindApex},
		{name: "www", host: "www.lv.test", base: "lv.test", kind: KindApex},
		{name: "clinic", host: "norte.lv.test", base: "lv.test", kind: KindClinic, slug: "norte"},
		{name: "port", host: "norte.lv.test:8080", base: "lv.test", kind: KindClinic, slug: "norte"},
		{name: "case", host: "Norte.LV.TEST", base: "lv.test", kind: KindClinic, slug: "norte"},
		{name: "foreign", host: "evil.com", base: "lv.test", err: true},
		{name: "nested", host: "a.b.lv.test", base: "lv.test", err: true},
		{name: "reserved api", host: "api.lv.test", base: "lv.test", err: true},
		{name: "reserved admin", host: "admin.lv.test", base: "lv.test", err: true},
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
			assert.Equal(t, tc.slug, got.Slug)
		})
	}
}
