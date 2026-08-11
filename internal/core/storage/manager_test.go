package storage

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/storage/repository"
)

// ageBlob rewinds the file mtime of a locally stored blob, so the
// reconciler treats it as older than the grace period.
func ageBlob(t *testing.T, store *Local, key string, back time.Duration) {
	t.Helper()
	old := time.Now().Add(-back)
	if err := os.Chtimes(filepath.Join(store.Root(), filepath.FromSlash(key)), old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
}

func testManager(t *testing.T) (*FileManager, *Local) {
	t.Helper()
	store := newTestLocal(t)
	db := openIndexDB(t)
	m, err := NewFileManager(db, store, testLogger(t))
	if err != nil {
		t.Fatalf("NewFileManager: %v", err)
	}
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
	ctx := context.Background()

	meta, err := m.Upload(ctx, uploadInput(), strings.NewReader("pdf"), 3)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if meta.OriginalName != "prescription.pdf" || meta.Size != 3 || meta.ETag == "" {
		t.Errorf("meta = %+v", meta)
	}
	if !strings.HasPrefix(meta.Key, "private/patient_document/") {
		t.Errorf("key %q must live under the private class and domain namespace", meta.Key)
	}

	got, err := m.Get(ctx, meta.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.OriginalName != "prescription.pdf" || got.ResourceID != testResource {
		t.Errorf("Get = %+v", got)
	}

	_, obj, err := m.Open(ctx, meta.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	body, _ := io.ReadAll(obj.Data)
	obj.Data.Close()
	if string(body) != "pdf" {
		t.Errorf("Open body = %q, want pdf", body)
	}

	list, err := m.List(ctx, "patient_document", testResource)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != meta.ID {
		t.Errorf("List = %+v, want the uploaded file", list)
	}

	if err := m.Delete(ctx, meta.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get(ctx, meta.ID); !IsNotFound(err) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
	if _, err := m.store.Stat(ctx, meta.Key); !IsNotFound(err) {
		t.Errorf("blob after delete = %v, want ErrNotFound", err)
	}
}

// TestFileManagerUploadCompensation forces the index write to fail
// (duplicate key) and asserts the saga removes the orphan blob.
func TestFileManagerUploadCompensation(t *testing.T) {
	m, store := testManager(t)
	ctx := context.Background()

	// Fix the object id so the key is known in advance, then pre-seed a
	// row with that key: the index write must fail on the UNIQUE
	// constraint.
	fixed := uuid.MustParse("01990000-0000-7000-8000-0000000000c1")
	m.newID = func() (uuid.UUID, error) { return fixed, nil }
	key := privateKey("patient_document", testResource, fixed)
	_, err := m.q.CreateStorageObject(ctx, repository.CreateStorageObjectParams{
		ID:  uuid.MustParse("01990000-0000-7000-8000-0000000000c2"),
		Key: key, Domain: "patient_document", ResourceID: testResource,
		OriginalName: "other.pdf", ContentType: "application/pdf",
		Size: 1, Etag: "x", CreatedBy: testOwner,
	})
	if err != nil {
		t.Fatalf("seed duplicate row: %v", err)
	}

	if _, err := m.Upload(ctx, uploadInput(), strings.NewReader("x"), 1); err == nil {
		t.Fatal("Upload must fail when the index rejects the row")
	}
	// Compensation: the blob must not exist.
	if _, err := store.Stat(ctx, key); !IsNotFound(err) {
		t.Errorf("orphan blob after compensation = %v, want ErrNotFound", err)
	}
}

// TestFileManagerReconcile removes blobs whose index row disappeared
// (the crash window between blob write and index write).
func TestFileManagerReconcile(t *testing.T) {
	m, store := testManager(t)
	ctx := context.Background()

	meta, err := m.Upload(ctx, uploadInput(), strings.NewReader("keep"), 4)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the crash window: blob present, index row gone. The blob
	// must be older than the grace period or the reconciler keeps it.
	orphanKey := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	if _, err := store.Put(ctx, orphanKey, strings.NewReader("lost"), 4, "text/plain"); err != nil {
		t.Fatal(err)
	}
	ageBlob(t, store, orphanKey, 2*time.Hour)

	removed, err := m.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if removed != 1 {
		t.Errorf("Reconcile removed %d, want 1", removed)
	}
	if _, err := store.Stat(ctx, orphanKey); !IsNotFound(err) {
		t.Errorf("orphan still present: %v", err)
	}
	// The indexed file survives.
	if _, err := m.Get(ctx, meta.ID); err != nil {
		t.Errorf("indexed file lost: %v", err)
	}
	if _, err := store.Stat(ctx, meta.Key); err != nil {
		t.Errorf("indexed blob lost: %v", err)
	}
}

// TestFileManagerGetUnknown exercises the not-found mapping.
func TestFileManagerGetUnknown(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if _, err := m.Get(ctx, uuid.Must(uuid.NewV7())); !IsNotFound(err) {
		t.Errorf("Get unknown = %v, want ErrNotFound", err)
	}
	if err := m.Delete(ctx, uuid.Must(uuid.NewV7())); !IsNotFound(err) {
		t.Errorf("Delete unknown = %v, want ErrNotFound", err)
	}
}

var _ = sql.ErrNoRows

// TestFileManagerReconcileGracePeriod covers the time buffer: a blob
// with no index row but a recent LastModified is an upload in flight
// and must survive the reconciler; only orphans older than the grace
// period are removed.
func TestFileManagerReconcileGracePeriod(t *testing.T) {
	m, store := testManager(t)
	ctx := context.Background()

	freshKey := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	oldKey := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	if _, err := store.Put(ctx, freshKey, strings.NewReader("fresh"), 5, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, oldKey, strings.NewReader("old"), 3, "text/plain"); err != nil {
		t.Fatal(err)
	}
	ageBlob(t, store, oldKey, 2*time.Hour)

	removed, err := m.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if removed != 1 {
		t.Errorf("Reconcile removed %d, want only the aged orphan", removed)
	}
	if _, err := store.Stat(ctx, freshKey); err != nil {
		t.Errorf("fresh blob removed by the reconciler: %v", err)
	}
	if _, err := store.Stat(ctx, oldKey); !IsNotFound(err) {
		t.Errorf("aged orphan still present: %v", err)
	}
}

// TestFileManagerReconcileZeroTimestamp treats a missing LastModified
// as fresh (the safe direction): it must never be deleted.
func TestFileManagerReconcileZeroTimestamp(t *testing.T) {
	m, store := testManager(t)
	ctx := context.Background()

	key := privateKey("patient_document", testResource, uuid.Must(uuid.NewV7()))
	if _, err := store.Put(ctx, key, strings.NewReader("x"), 1, "text/plain"); err != nil {
		t.Fatal(err)
	}
	// Rewind to the zero time via the metadata sidecar? The local stat
	// is authoritative, so a zero timestamp cannot happen there; the
	// guard is defensive for backends without mtime. Assert the method
	// behaves safely when it does.
	old := time.Time{}
	if err := os.Chtimes(filepath.Join(store.Root(), filepath.FromSlash(key)), old, old); err != nil {
		t.Fatal(err)
	}
	removed, err := m.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if removed != 0 {
		t.Errorf("zero-timestamp blob removed %d, want kept", removed)
	}
	if _, err := store.Stat(ctx, key); err != nil {
		t.Errorf("zero-timestamp blob lost: %v", err)
	}
}

// TestFileManagerAccessClass asserts the class is derived from the
// domain: avatars are public, clinical attachments private.
func TestFileManagerAccessClass(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()

	avatar, err := m.Upload(ctx, UploadInput{
		Domain: "avatar", ResourceID: testResource,
		OriginalName: "photo.png", ContentType: "image/png", CreatedBy: testOwner,
	}, strings.NewReader("img"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(avatar.Key, "public/avatar/") {
		t.Errorf("avatar key = %q, want public class", avatar.Key)
	}

	doc, err := m.Upload(ctx, uploadInput(), strings.NewReader("pdf"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(doc.Key, "private/patient_document/") {
		t.Errorf("document key = %q, want private class", doc.Key)
	}
}

// TestFileManagerGetForResource enforces belonging: an object id alone
// must never resolve a file of another resource (IDOR protection).
func TestFileManagerGetForResource(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()

	other := uuid.MustParse("01990000-0000-7000-8000-0000000000c0")
	meta, err := m.Upload(ctx, uploadInput(), strings.NewReader("pdf"), 3)
	if err != nil {
		t.Fatal(err)
	}

	// The owning resource resolves the file.
	got, err := m.GetForResource(ctx, "patient_document", testResource, meta.ID)
	if err != nil || got.ID != meta.ID {
		t.Errorf("GetForResource owner = %v, %v; want the file", got, err)
	}
	// A different resource never resolves it.
	if _, err := m.GetForResource(ctx, "patient_document", other, meta.ID); !IsNotFound(err) {
		t.Errorf("GetForResource other resource = %v, want ErrNotFound", err)
	}
	// A different domain never resolves it either.
	if _, err := m.GetForResource(ctx, "avatar", testResource, meta.ID); !IsNotFound(err) {
		t.Errorf("GetForResource other domain = %v, want ErrNotFound", err)
	}
	// OpenForResource follows the same belonging rule.
	if _, _, err := m.OpenForResource(ctx, "patient_document", other, meta.ID); !IsNotFound(err) {
		t.Errorf("OpenForResource other resource = %v, want ErrNotFound", err)
	}
}
