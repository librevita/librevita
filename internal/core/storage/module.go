package storage

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"go.uber.org/fx"

	"librevita.org/internal/core/config"
)

// Backend names for config.Storage.Backend.
const (
	BackendLocal = "local"
	BackendS3    = "s3"
)

// Module provides the file storage backend selected by
// config.Storage, the FileManager saga coordinator, and the periodic
// orphan reconciler. In s3 mode the startup hook verifies the bucket so
// a misconfigured backend fails fast.
var Module = fx.Module("storage",
	fx.Provide(NewStore),
	fx.Provide(NewFileManager),
	fx.Invoke(registerLifecycle, registerReconciler),
)

// reconcileInterval is how often the saga reconciler scans the blob
// store for orphaned objects.
const reconcileInterval = 15 * time.Minute

// registerReconciler runs Reconcile periodically: it removes blobs that
// have no master-index row, compensating uploads that crashed between
// the blob write and the index write. The first pass runs shortly after
// startup, then every reconcileInterval.
func registerReconciler(lc fx.Lifecycle, manager *FileManager, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				ticker := time.NewTicker(reconcileInterval)
				defer ticker.Stop()
				for {
					if _, err := manager.Reconcile(ctx); err != nil {
						log.Warn("storage reconcile failed", "error", err)
					}
					select {
					case <-ticker.C:
					case <-ctx.Done():
						return
					}
				}
			}()
			return nil
		},
	})
}

// NewStore builds the backend named by the configuration.
func NewStore(cfg *config.Config, log *slog.Logger) (Store, error) {
	switch cfg.Storage.Backend {
	case "", BackendLocal:
		dir := cfg.Storage.Local.Dir
		if dir == "" {
			dir = filepath.Join(cfg.DataDir, "files")
		}
		log.Info("file storage backend", "backend", BackendLocal, "dir", dir)
		return NewLocal(dir)
	case BackendS3:
		log.Info("file storage backend", "backend", BackendS3,
			"endpoint", cfg.Storage.S3.Endpoint, "bucket", cfg.Storage.S3.Bucket)
		return NewS3(S3Config{
			Endpoint:  cfg.Storage.S3.Endpoint,
			Bucket:    cfg.Storage.S3.Bucket,
			AccessKey: cfg.Storage.S3.AccessKey,
			SecretKey: cfg.Storage.S3.SecretKey,
			Region:    cfg.Storage.S3.Region,
			Secure:    cfg.Storage.S3.Secure,
			PathStyle: cfg.Storage.S3.PathStyle,
		})
	default:
		return nil, &configInvalidError{backend: cfg.Storage.Backend}
	}
}

type configInvalidError struct{ backend string }

func (e *configInvalidError) Error() string {
	return "storage: unknown backend " + e.backend + " (use " + BackendLocal + " or " + BackendS3 + ")"
}

// registerLifecycle verifies the backend at startup. The local backend
// creates its root during construction; the S3 backend checks the
// bucket. Verification failures abort the startup.
func registerLifecycle(lc fx.Lifecycle, store Store, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if v, ok := store.(*S3); ok {
				if err := v.Verify(ctx); err != nil {
					return err
				}
				log.Info("file storage backend verified", "backend", BackendS3)
			}
			return nil
		},
	})
}
