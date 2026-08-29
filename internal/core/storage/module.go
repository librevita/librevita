package storage

import (
	"context"
	"librevita.org/pkg/log"
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
	fx.Provide(NewIndexRepository),
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
func registerReconciler(lc fx.Lifecycle, manager *FileManager, logger log.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				ticker := time.NewTicker(reconcileInterval)
				defer ticker.Stop()
				for {
					if _, err := manager.Reconcile(ctx); err != nil {
						logger.Warn("storage reconcile failed", log.Error(err))
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
func NewStore(cfg *config.Config, logger log.Logger) (Store, error) {
	switch cfg.Storage.Backend {
	case "", BackendLocal:
		dir := cfg.Storage.Local.Dir
		if dir == "" {
			dir = filepath.Join(cfg.DataDir, "files")
		}
		logger.Info("file storage backend",
			log.String("backend", BackendLocal),
			log.String("dir", dir),
		)
		return NewLocal(dir)
	case BackendS3:
		logger.Info("file storage backend",
			log.String("backend", BackendS3),
			log.String("endpoint", cfg.Storage.S3.Endpoint),
			log.String("bucket", cfg.Storage.S3.Bucket),
		)
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
func registerLifecycle(lc fx.Lifecycle, store Store, logger log.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if v, ok := store.(*S3); ok {
				if err := v.Verify(ctx); err != nil {
					return err
				}
				logger.Info("file storage backend verified",
					log.String("backend", BackendS3),
				)
			}
			return nil
		},
	})
}
