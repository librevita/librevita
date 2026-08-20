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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	err = database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

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
	require.NoError(t, err)

	owner := uuid.MustParse("01990000-0000-7000-8000-00000000000a")
	resource := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	id, err := uuid.NewV7()
	require.NoError(t, err)

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
	require.NoError(t, err)
	assert.Equal(t, "document.pdf", created.OriginalName)
	assert.Equal(t, int64(9), created.Size)
	assert.Equal(t, blob.ETag, created.Etag)

	byKey, err := client.StorageObject.Query().Where(storageobject.KeyEQ(key)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, created.ID, byKey.ID)

	listed, err := client.StorageObject.Query().
		Where(
			storageobject.DomainEQ("patient_document"),
			storageobject.ResourceIDEQ(resource.String()),
		).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, key, listed[0].Key)

	err = client.StorageObject.DeleteOneID(created.ID).Exec(ctx)
	require.NoError(t, err)

	_, err = client.StorageObject.Query().Where(storageobject.KeyEQ(key)).Only(ctx)
	assert.True(t, ent.IsNotFound(err))

	// The blob itself is still in the store; deleting the index row does
	// not touch the object.
	_, err = store.Stat(ctx, key)
	assert.NoError(t, err)
}

// TestStorageIndexRejectsNegativeSize exercises the size constraint.
func TestStorageIndexRejectsNegativeSize(t *testing.T) {
	ctx := context.Background()
	_, client := openIndexDB(t)
	badID, err := uuid.NewV7()
	require.NoError(t, err)

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
	assert.Error(t, err, "negative size must violate the CHECK constraint")
}
