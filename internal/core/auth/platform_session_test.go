package auth_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/kv"
	"librevita.org/pkg/ident"
	"librevita.org/pkg/log"
)

func TestPlatformSessionRepository(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "platform_session.db"))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, database.Migrate(context.Background(), db, log.Nop()))

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	sessKV, err := kv.OpenBBolt(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sessKV.Close() })

	repo := auth.NewPlatformSessionRepository(sessKV, client)

	// Create platform user
	pUserID := ident.New[ident.PlatformUserID]()
	ctx := context.Background()
	_, err = client.PlatformUser.Create().
		SetID(pUserID).
		SetEmail("platform_admin@librevita.org").
		SetDisplayName("Platform Admin").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)

	sessID := uuid.New().String()
	now := time.Now().UTC()
	expiresAt := now.Add(time.Hour)

	// 1. Create session
	require.NoError(t, repo.Create(ctx, sessID, pUserID.UUID(), expiresAt))

	// 2. GetActive - active
	record, err := repo.GetActive(ctx, sessID, now)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, sessID, record.ID)
	assert.Equal(t, pUserID.UUID(), record.UserID)
	assert.Equal(t, "platform_admin@librevita.org", record.User.Email)

	// 3. GetActive - expired
	recordExpired, err := repo.GetActive(ctx, sessID, now.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Nil(t, recordExpired)

	// 4. CleanupExpired
	require.NoError(t, repo.CleanupExpired(ctx, now.Add(2*time.Hour)))
	recordCleaned, err := repo.GetActive(ctx, sessID, now)
	require.NoError(t, err)
	assert.Nil(t, recordCleaned)

	// 5. Create again and Delete
	require.NoError(t, repo.Create(ctx, sessID, pUserID.UUID(), expiresAt))
	require.NoError(t, repo.Delete(ctx, sessID))
	recordDeleted, err := repo.GetActive(ctx, sessID, now)
	require.NoError(t, err)
	assert.Nil(t, recordDeleted)
}
