package storage

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/storageobject"
	"librevita.org/internal/core/database"
)

// openIndexDB opens an in-memory SQLite with every migration applied.
func openIndexDB(t *testing.T) (*sql.DB, *ent.Client) {
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

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	return db, client
}

// TestStorageIndexWire validates the master index end to end: the blob
// goes to the Store, the metadata row goes to storage_objects, and the
// index queries resolve it back.
func TestStorageIndexWire(t *testing.T) {
	ctx := context.Background()
	store := newTestLocal(t)
	_, client := openIndexDB(t)

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
	created, err := client.StorageObject.Create().
		SetID(id).
		SetKey(key).
		SetDomain("patient_document").
		SetResourceID(resource.String()).
		SetOriginalName("document.pdf").
		SetContentType(blob.ContentType).
		SetSize(blob.Size).
		SetEtag(blob.ETag).
		SetChecksum(blob.Checksum).
		SetCreatedBy(owner).
		Save(ctx)
	if err != nil {
		t.Fatalf("CreateStorageObject: %v", err)
	}
	if created.OriginalName != "document.pdf" || created.Size != 9 || created.Etag != blob.ETag {
		t.Errorf("index row = %+v", created)
	}

	byKey, err := client.StorageObject.Query().Where(storageobject.KeyEQ(key)).Only(ctx)
	if err != nil {
		t.Fatalf("GetStorageObjectByKey: %v", err)
	}
	if byKey.ID != created.ID {
		t.Errorf("byKey.ID = %s, want %s", byKey.ID, created.ID)
	}

	listed, err := client.StorageObject.Query().
		Where(
			storageobject.DomainEQ("patient_document"),
			storageobject.ResourceIDEQ(resource.String()),
		).
		All(ctx)
	if err != nil {
		t.Fatalf("ListStorageObjectsByResource: %v", err)
	}
	if len(listed) != 1 || listed[0].Key != key {
		t.Errorf("list = %+v, want the stored key", listed)
	}

	if err := client.StorageObject.DeleteOneID(created.ID).Exec(ctx); err != nil {
		t.Fatalf("DeleteStorageObject: %v", err)
	}
	if _, err := client.StorageObject.Query().Where(storageobject.KeyEQ(key)).Only(ctx); !ent.IsNotFound(err) {
		t.Errorf("GetStorageObjectByKey after delete = %v, want ent.IsNotFound", err)
	}
	// The blob itself is still in the store; deleting the index row does
	// not touch the object.
	if _, err := store.Stat(ctx, key); err != nil {
		t.Errorf("blob Stat after index delete = %v, want present", err)
	}
}

// TestStorageIndexRejectsNegativeSize exercises the size constraint.
func TestStorageIndexRejectsNegativeSize(t *testing.T) {
	ctx := context.Background()
	_, client := openIndexDB(t)
	badID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StorageObject.Create().
		SetID(badID).
		SetKey("k/1").
		SetDomain("d").
		SetResourceID(uuid.MustParse("01990000-0000-7000-8000-000000000001").String()).
		SetOriginalName("x").
		SetContentType("text/plain").
		SetSize(-1).
		SetEtag("e").
		SetChecksum("c").
		SetCreatedBy(uuid.MustParse("01990000-0000-7000-8000-00000000000a")).
		Save(ctx)
	if err == nil {
		t.Fatal("negative size must violate the CHECK constraint")
	}
}
