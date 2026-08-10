package storage

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/database"
	"librevita.org/internal/core/storage/repository"
)

// openIndexDB opens an in-memory SQLite with every migration applied.
func openIndexDB(t *testing.T) *sql.DB {
	t.Helper()
	name := "storage-test-" + uuid.NewString()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestStorageIndexWire validates the master index end to end: the blob
// goes to the Store, the metadata row goes to storage_objects, and the
// index queries resolve it back.
func TestStorageIndexWire(t *testing.T) {
	ctx := context.Background()
	store := newTestLocal(t)
	db := openIndexDB(t)
	q := repository.New(db)

	key := "patient_document/01990000-0000-7000-8000-000000000001/doc.pdf"
	blob, err := store.Put(ctx, key, strings.NewReader("pdf-bytes"), 9, "application/pdf")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	owner := uuid.MustParse("01990000-0000-7000-8000-00000000000a")
	resource := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	created, err := q.CreateStorageObject(ctx, repository.CreateStorageObjectParams{
		ID:           id,
		Key:          key,
		Domain:       "patient_document",
		ResourceID:   resource,
		OriginalName: "document.pdf",
		ContentType:  blob.ContentType,
		Size:         blob.Size,
		Etag:         blob.ETag,
		CreatedBy:    owner,
	})
	if err != nil {
		t.Fatalf("CreateStorageObject: %v", err)
	}
	if created.OriginalName != "document.pdf" || created.Size != 9 || created.Etag != blob.ETag {
		t.Errorf("index row = %+v", created)
	}

	byKey, err := q.GetStorageObjectByKey(ctx, key)
	if err != nil {
		t.Fatalf("GetStorageObjectByKey: %v", err)
	}
	if byKey.ID != created.ID {
		t.Errorf("byKey.ID = %s, want %s", byKey.ID, created.ID)
	}

	listed, err := q.ListStorageObjectsByResource(ctx, repository.ListStorageObjectsByResourceParams{
		Domain: "patient_document", ResourceID: resource,
	})
	if err != nil {
		t.Fatalf("ListStorageObjectsByResource: %v", err)
	}
	if len(listed) != 1 || listed[0].Key != key {
		t.Errorf("list = %+v, want the stored key", listed)
	}

	if err := q.DeleteStorageObject(ctx, created.ID); err != nil {
		t.Fatalf("DeleteStorageObject: %v", err)
	}
	if _, err := q.GetStorageObjectByKey(ctx, key); err != sql.ErrNoRows {
		t.Errorf("GetStorageObjectByKey after delete = %v, want sql.ErrNoRows", err)
	}
	// The blob itself is still in the store; deleting the index row does
	// not touch the object.
	if _, err := store.Stat(ctx, key); err != nil {
		t.Errorf("blob Stat after index delete = %v, want present", err)
	}
}

// TestStorageIndexRejectsNegativeSize exercises the CHECK constraint.
func TestStorageIndexRejectsNegativeSize(t *testing.T) {
	ctx := context.Background()
	db := openIndexDB(t)
	q := repository.New(db)
	badID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.CreateStorageObject(ctx, repository.CreateStorageObjectParams{
		ID:           badID,
		Key:          "k/1",
		Domain:       "d",
		ResourceID:   uuid.MustParse("01990000-0000-7000-8000-000000000001"),
		OriginalName: "x",
		ContentType:  "text/plain",
		Size:         -1,
		Etag:         "e",
		CreatedBy:    uuid.MustParse("01990000-0000-7000-8000-00000000000a"),
	})
	if err == nil {
		t.Fatal("negative size must violate the CHECK constraint")
	}
}
