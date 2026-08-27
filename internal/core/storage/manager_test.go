package storage

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
)

// ageBlob rewinds the file mtime of a locally stored blob, so the
// reconciler treats it as older than the grace period.
func ageBlob(t *testing.T, store *Local, key string, back time.Duration) {
	t.Helper()
	old := time.Now().Add(-back)
	err := os.Chtimes(filepath.Join(store.Root(), filepath.FromSlash(key)), old, old)
	require.NoError(t, err)
}

func testManager(t *testing.T) (*FileManager, *Local) {
	t.Helper()
	store := newTestLocal(t)
	_, client := openIndexDB(t)
	m, err := NewFileManager(NewIndexRepository(client), store, testLogger(t))
	require.NoError(t, err)
	return m, store
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.DiscardHandler)
}

var (
	testOwner    = uuid.MustParse("01990000-0000-7000-8000-00000000000a")
	testResource = uuid.MustParse("01990000-0000-7000-8000-0000000000b0")
)

// privateKey mirrors the FileManager key layout for private domains.
func privateKey(domain string, resourceID, id uuid.UUID) string {
	return "private/" + domain + "/" + resourceID.String() + "/" + id.String()
}

func uploadInput() UploadInput {
	return UploadInput{
		Domain:       "patient_document",
		ResourceID:   testResource,
		OriginalName: "prescription.pdf",
		ContentType:  "application/pdf",
		CreatedBy:    testOwner,
	}
}

// TestFileManagerUploadGetListDelete covers the happy saga: blob stored,
// index row registered, metadata resolvable, and delete removing both.
func TestFileManagerUploadGetListDelete(t *testing.T) {
	m, _ := testManager(t)
	ctx := storageCtx()

	meta, err := m.Upload(ctx, uploadInput(), strings.NewReader("pdf"), 3)
	require.NoError(t, err)
	assert.Equal(t, "prescription.pdf", meta.OriginalName)
	assert.Equal(t, int64(3), meta.Size)
	assert.NotEmpty(t, meta.ETag)
	assert.Equal(t, blake2b256Hex("pdf"), meta.Checksum)
	assert.True(t, strings.HasPrefix(meta.Key, "private/patient_document/"))

	got, err := m.Get(ctx, meta.ID)
	require.NoError(t, err)
	assert.Equal(t, "prescription.pdf", got.OriginalName)
	assert.Equal(t, testResource, got.ResourceID)

	_, obj, err := m.Open(ctx, meta.ID)
	require.NoError(t, err)
	body, err := io.ReadAll(obj.Data)
	require.NoError(t, err)
	obj.Data.Close()
	assert.Equal(t, "pdf", string(body))

	list, err := m.List(ctx, "patient_document", testResource)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, meta.ID, list[0].ID)

	err = m.Delete(ctx, meta.ID)
	require.NoError(t, err)

	_, err = m.Get(ctx, meta.ID)
	assert.True(t, IsNotFound(err))

	_, err = m.store.Stat(ctx, meta.Key)
	assert.True(t, IsNotFound(err))
}

func TestFileManagerEncryptedUploadOpen(t *testing.T) {
	m, store := testManager(t)
	ctx := storageCtx()
	key := bytes.Repeat([]byte{0x19}, crypto.SizeDEK)
	aad := []byte("urn:librevita:clinic:patient")
	plain := strings.Repeat("clinical-note-", 7000)

	meta, err := m.UploadEncrypted(ctx, uploadInput(), strings.NewReader(plain), int64(len(plain)), key, aad)
	require.NoError(t, err)
	assert.Equal(t, int64(len(plain)), meta.Size)
	assert.Equal(t, blake2b256Hex(plain), meta.Checksum)

	raw, err := store.Get(ctx, meta.Key)
	require.NoError(t, err)
	encoded, err := io.ReadAll(raw.Data)
	require.NoError(t, err)
	require.NoError(t, raw.Data.Close())
	assert.NotEqual(t, []byte(plain), encoded)

	openedMeta, opened, err := m.OpenEncryptedForResource(ctx, uploadInput().Domain, testResource, meta.ID, key, aad)
	require.NoError(t, err)
	assert.Equal(t, meta.ID, openedMeta.ID)
	got, err := io.ReadAll(opened.Data)
	require.NoError(t, err)
	require.NoError(t, opened.Data.Close())
	assert.Equal(t, plain, string(got))

	require.NoError(t, m.Delete(ctx, meta.ID))
	_, err = store.Stat(ctx, meta.Key)
	assert.True(t, IsNotFound(err))
}

// TestFileManagerUploadCompensation forces the index write to fail
// (duplicate key) and asserts the saga removes the orphan blob.
func TestFileManagerUploadCompensation(t *testing.T) {
	m, store := testManager(t)
	ctx := storageCtx()

	fixed := uuid.MustParse("01990000-0000-7000-8000-0000000000c1")
	m.newID = func() (uuid.UUID, error) { return fixed, nil }
	key := privateKey("patient_document", testResource, fixed)
	_, err := m.repo.Insert(ctx, StoredFile{
		ID:           uuid.MustParse("01990000-0000-7000-8000-0000000000c2"),
		Key:          key,
		Domain:       "patient_document",
		ResourceID:   testResource,
		OriginalName: "other.pdf",
		ContentType:  "application/pdf",
		Size:         1,
		ETag:         "x",
		Checksum:     "c",
		CreatedBy:    testOwner,
	})
	require.NoError(t, err)

	_, err = m.Upload(ctx, uploadInput(), strings.NewReader("x"), 1)
	assert.Error(t, err, "Upload must fail when index rejects row")

	// Compensation: the blob must not exist.
	_, err = store.Stat(ctx, key)
	assert.True(t, IsNotFound(err), "orphan blob after compensation should not exist")
}

// TestFileManagerReconcile removes blobs whose index row disappeared
// (the crash window between blob write and index write).
func TestFileManagerReconcile(t *testing.T) {
	m, store := testManager(t)
	ctx := storageCtx()

	meta, err := m.Upload(ctx, uploadInput(), strings.NewReader("keep"), 4)
	require.NoError(t, err)

	orphanKey := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	_, err = store.Put(ctx, orphanKey, strings.NewReader("lost"), 4, "text/plain")
	require.NoError(t, err)
	ageBlob(t, store, orphanKey, 2*time.Hour)

	removed, err := m.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	_, err = store.Stat(ctx, orphanKey)
	assert.True(t, IsNotFound(err), "orphan should be removed")

	// The indexed file survives.
	_, err = m.Get(ctx, meta.ID)
	assert.NoError(t, err)

	_, err = store.Stat(ctx, meta.Key)
	assert.NoError(t, err)
}

// TestFileManagerGetUnknown exercises the not-found mapping.
func TestFileManagerGetUnknown(t *testing.T) {
	m, _ := testManager(t)
	ctx := storageCtx()

	_, err := m.Get(ctx, uuid.Must(uuid.NewV7()))
	assert.True(t, IsNotFound(err))

	err = m.Delete(ctx, uuid.Must(uuid.NewV7()))
	assert.True(t, IsNotFound(err))
}

// TestFileManagerReconcileGracePeriod covers the time buffer: a blob
// with no index row but a recent LastModified is an upload in flight
// and must survive the reconciler; only orphans older than the grace
// period are removed.
func TestFileManagerReconcileGracePeriod(t *testing.T) {
	m, store := testManager(t)
	ctx := storageCtx()

	freshKey := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	oldKey := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	_, err := store.Put(ctx, freshKey, strings.NewReader("fresh"), 5, "text/plain")
	require.NoError(t, err)
	_, err = store.Put(ctx, oldKey, strings.NewReader("old"), 3, "text/plain")
	require.NoError(t, err)
	ageBlob(t, store, oldKey, 2*time.Hour)

	removed, err := m.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	_, err = store.Stat(ctx, freshKey)
	assert.NoError(t, err, "fresh blob should be kept")

	_, err = store.Stat(ctx, oldKey)
	assert.True(t, IsNotFound(err), "aged orphan should be removed")
}

// TestFileManagerReconcileZeroTimestamp treats a missing LastModified
// as fresh (the safe direction): it must never be deleted.
func TestFileManagerReconcileZeroTimestamp(t *testing.T) {
	m, store := testManager(t)
	ctx := storageCtx()

	key := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	_, err := store.Put(ctx, key, strings.NewReader("x"), 1, "text/plain")
	require.NoError(t, err)

	old := time.Time{}
	err = os.Chtimes(filepath.Join(store.Root(), filepath.FromSlash(key)), old, old)
	require.NoError(t, err)

	removed, err := m.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)

	_, err = store.Stat(ctx, key)
	assert.NoError(t, err, "zero-timestamp blob should be kept")
}

// TestFileManagerAccessClass asserts the class is derived from the
// domain: avatars are public, clinical attachments private.
func TestFileManagerAccessClass(t *testing.T) {
	m, _ := testManager(t)
	ctx := storageCtx()

	avatar, err := m.Upload(ctx, UploadInput{
		Domain: "avatar", ResourceID: testResource,
		OriginalName: "photo.png", ContentType: "image/png", CreatedBy: testOwner,
	}, strings.NewReader("img"), 3)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(avatar.Key, "public/avatar/"))

	doc, err := m.Upload(ctx, uploadInput(), strings.NewReader("pdf"), 3)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(doc.Key, "private/patient_document/"))
}

// TestFileManagerGetForResource enforces belonging: an object id alone
// must never resolve a file of another resource (IDOR protection).
func TestFileManagerGetForResource(t *testing.T) {
	m, _ := testManager(t)
	ctx := storageCtx()

	other := uuid.MustParse("01990000-0000-7000-8000-0000000000c0")
	meta, err := m.Upload(ctx, uploadInput(), strings.NewReader("pdf"), 3)
	require.NoError(t, err)

	// The owning resource resolves the file.
	got, err := m.GetForResource(ctx, "patient_document", testResource, meta.ID)
	require.NoError(t, err)
	assert.Equal(t, meta.ID, got.ID)

	// A different resource never resolves it.
	_, err = m.GetForResource(ctx, "patient_document", other, meta.ID)
	assert.True(t, IsNotFound(err))

	// A different domain never resolves it either.
	_, err = m.GetForResource(ctx, "avatar", testResource, meta.ID)
	assert.True(t, IsNotFound(err))

	// OpenForResource follows the same belonging rule.
	_, _, err = m.OpenForResource(ctx, "patient_document", other, meta.ID)
	assert.True(t, IsNotFound(err))
}
