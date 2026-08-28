package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"librevita.org/internal/core/crypto"
)

// TestValidKey covers the key layout rules that both backends enforce.
func TestValidKey(t *testing.T) {
	valid := []string{
		"patients/abc/document.pdf",
		"a", "a/b/c",
		"export-2026-08-10.csv",
	}
	for _, key := range valid {
		assert.NoError(t, ValidKey(key), "ValidKey(%q) should be valid", key)
	}
	invalid := []string{"", "/abs", "a/../b", "a/./b", "a//b", `a\b`, "../x", "a/.."}
	for _, key := range invalid {
		assert.Error(t, ValidKey(key), "ValidKey(%q) should be invalid", key)
	}
}

func TestLocalPutGetStat(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()

	info, err := s.Put(ctx, "patients/p1/doc.pdf", strings.NewReader("hello"), 5, "application/pdf")
	require.NoError(t, err)
	assert.Equal(t, "patients/p1/doc.pdf", info.Key)
	assert.Equal(t, int64(5), info.Size)
	assert.Equal(t, "application/pdf", info.ContentType)
	assert.NotEmpty(t, info.ETag)

	wantChecksum := blake2b256Hex("hello")
	assert.Equal(t, wantChecksum, info.Checksum)

	obj, err := s.Get(ctx, "patients/p1/doc.pdf")
	require.NoError(t, err)
	defer obj.Data.Close()
	got, err := io.ReadAll(obj.Data)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
	assert.Equal(t, int64(5), obj.Size)
	assert.Equal(t, "application/pdf", obj.ContentType)
	assert.Equal(t, info.ETag, obj.ETag)

	st, err := s.Stat(ctx, "patients/p1/doc.pdf")
	require.NoError(t, err)
	assert.Equal(t, int64(5), st.Size)
	assert.Equal(t, "application/pdf", st.ContentType)
	assert.Equal(t, wantChecksum, st.Checksum)
}

// blake2b256Hex computes the canonical checksum of the payload via crypto.
func blake2b256Hex(payload string) string {
	return crypto.Digest256([]byte(payload))
}

func TestLocalNotFound(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()

	_, err := s.Get(ctx, "missing")
	assert.True(t, IsNotFound(err))

	_, err = s.Stat(ctx, "missing")
	assert.True(t, IsNotFound(err))

	// Delete is idempotent.
	err = s.Delete(ctx, "missing")
	assert.NoError(t, err)
}

func TestLocalOverwriteAndDelete(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()

	_, err := s.Put(ctx, "k", strings.NewReader("one"), 3, "text/plain")
	require.NoError(t, err)

	info, err := s.Put(ctx, "k", strings.NewReader("two!"), 4, "text/plain")
	require.NoError(t, err)
	assert.Equal(t, int64(4), info.Size)

	obj, err := s.Get(ctx, "k")
	require.NoError(t, err)
	body, err := io.ReadAll(obj.Data)
	require.NoError(t, err)
	obj.Data.Close()
	assert.Equal(t, "two!", string(body))

	err = s.Delete(ctx, "k")
	require.NoError(t, err)

	_, err = s.Get(ctx, "k")
	assert.True(t, IsNotFound(err))
}

func TestLocalList(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()
	for _, key := range []string{"a/x", "a/y", "b/z", "a/sub/w"} {
		_, err := s.Put(ctx, key, strings.NewReader("data"), 4, "text/plain")
		require.NoError(t, err)
	}
	all, err := s.List(ctx, "")
	require.NoError(t, err)
	assert.Len(t, all, 4)

	prefix, err := s.List(ctx, "a/")
	require.NoError(t, err)
	assert.Len(t, prefix, 3)

	for i := 1; i < len(prefix); i++ {
		assert.Less(t, prefix[i-1].Key, prefix[i].Key, "List not sorted")
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()
	for _, key := range []string{"../escape", "/abs", "a/../../b", `a\b`} {
		_, err := s.Put(ctx, key, bytes.NewReader(nil), 0, "")
		assert.Error(t, err, "Put(%q) should fail", key)
	}
	// The escape attempt must not have created anything outside.
	dir := filepath.Join(t.TempDir(), "root")
	s = newTestLocalAt(t, dir)
	_, err := s.Put(ctx, "a/../../escape", bytes.NewReader([]byte("x")), 1, "")
	assert.Error(t, err, "traversal Put should fail")

	// Nothing escaped: the root contains only .meta.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.Equal(t, metaDir, e.Name(), "escape created unexpected file outside root")
	}
}

func newTestLocal(t *testing.T) *Local {
	t.Helper()
	return newTestLocalAt(t, t.TempDir())
}

func newTestLocalAt(t *testing.T, dir string) *Local {
	t.Helper()
	s, err := NewLocal(dir)
	require.NoError(t, err)
	return s
}

var _ Store = (*Local)(nil)

func TestLocalStoreInterface(t *testing.T) {
	assert.False(t, errors.Is(ErrNotFound, errors.New("x")), "IsNotFound must not match unrelated errors")
}
