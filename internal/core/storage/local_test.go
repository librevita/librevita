package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// TestValidKey covers the key layout rules that both backends enforce.
func TestValidKey(t *testing.T) {
	valid := []string{
		"patients/abc/document.pdf",
		"a", "a/b/c",
		"export-2026-08-10.csv",
	}
	for _, key := range valid {
		if err := ValidKey(key); err != nil {
			t.Errorf("ValidKey(%q) = %v, want nil", key, err)
		}
	}
	invalid := []string{"", "/abs", "a/../b", "a/./b", "a//b", `a\b`, "../x", "a/.."}
	for _, key := range invalid {
		if err := ValidKey(key); err == nil {
			t.Errorf("ValidKey(%q) = nil, want error", key)
		}
	}
}

func TestLocalPutGetStat(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()

	info, err := s.Put(ctx, "patients/p1/doc.pdf", strings.NewReader("hello"), 5, "application/pdf")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Key != "patients/p1/doc.pdf" || info.Size != 5 || info.ContentType != "application/pdf" || info.ETag == "" {
		t.Errorf("Put info = %+v", info)
	}
	wantChecksum := blake2b256Hex("hello")
	if info.Checksum != wantChecksum {
		t.Errorf("checksum = %q, want %q", info.Checksum, wantChecksum)
	}

	obj, err := s.Get(ctx, "patients/p1/doc.pdf")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer obj.Data.Close()
	got, _ := io.ReadAll(obj.Data)
	if string(got) != "hello" {
		t.Errorf("Get body = %q, want hello", got)
	}
	if obj.Size != 5 || obj.ContentType != "application/pdf" || obj.ETag != info.ETag {
		t.Errorf("Get info = %+v", obj.ObjectInfo)
	}

	st, err := s.Stat(ctx, "patients/p1/doc.pdf")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if st.Size != 5 || st.ContentType != "application/pdf" || st.Checksum != wantChecksum {
		t.Errorf("Stat = %+v", st)
	}
}

// blake2b256Hex computes the canonical checksum of the payload.
func blake2b256Hex(payload string) string {
	sum := blake2b.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func TestLocalNotFound(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()

	if _, err := s.Get(ctx, "missing"); !IsNotFound(err) {
		t.Errorf("Get missing = %v, want ErrNotFound", err)
	}
	if _, err := s.Stat(ctx, "missing"); !IsNotFound(err) {
		t.Errorf("Stat missing = %v, want ErrNotFound", err)
	}
	// Delete is idempotent.
	if err := s.Delete(ctx, "missing"); err != nil {
		t.Errorf("Delete missing = %v, want nil", err)
	}
}

func TestLocalOverwriteAndDelete(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()

	if _, err := s.Put(ctx, "k", strings.NewReader("one"), 3, "text/plain"); err != nil {
		t.Fatal(err)
	}
	info, err := s.Put(ctx, "k", strings.NewReader("two!"), 4, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 4 {
		t.Errorf("overwrite size = %d, want 4", info.Size)
	}
	obj, _ := s.Get(ctx, "k")
	body, _ := io.ReadAll(obj.Data)
	obj.Data.Close()
	if string(body) != "two!" {
		t.Errorf("overwrite body = %q, want two!", body)
	}

	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "k"); !IsNotFound(err) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestLocalList(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()
	for _, key := range []string{"a/x", "a/y", "b/z", "a/sub/w"} {
		if _, err := s.Put(ctx, key, strings.NewReader("data"), 4, "text/plain"); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("List() = %d objects, want 4", len(all))
	}
	prefix, err := s.List(ctx, "a/")
	if err != nil {
		t.Fatal(err)
	}
	if len(prefix) != 3 {
		t.Fatalf("List(a/) = %d objects, want 3", len(prefix))
	}
	for i := 1; i < len(prefix); i++ {
		if prefix[i-1].Key >= prefix[i].Key {
			t.Errorf("List not sorted: %q before %q", prefix[i-1].Key, prefix[i].Key)
		}
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	s := newTestLocal(t)
	ctx := context.Background()
	for _, key := range []string{"../escape", "/abs", "a/../../b", `a\b`} {
		if _, err := s.Put(ctx, key, bytes.NewReader(nil), 0, ""); err == nil {
			t.Errorf("Put(%q) = nil, want error", key)
		}
	}
	// The escape attempt must not have created anything outside.
	dir := filepath.Join(t.TempDir(), "root")
	s = newTestLocalAt(t, dir)
	if _, err := s.Put(ctx, "a/../../escape", bytes.NewReader([]byte("x")), 1, ""); err == nil {
		t.Fatalf("traversal Put = nil, want error")
	}
	// Nothing escaped: the root contains only .meta.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != metaDir {
			t.Errorf("escape created %q outside the root", e.Name())
		}
	}
}

func newTestLocal(t *testing.T) *Local {
	t.Helper()
	return newTestLocalAt(t, t.TempDir())
}

func newTestLocalAt(t *testing.T, dir string) *Local {
	t.Helper()
	s, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	return s
}

var _ Store = (*Local)(nil)

func TestLocalStoreInterface(t *testing.T) {
	if errors.Is(ErrNotFound, errors.New("x")) {
		t.Fatal("IsNotFound must not match unrelated errors")
	}
}
