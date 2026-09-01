package auth

import (
	"context"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	"go.uber.org/fx"

	"librevita.org/ent"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/kv"
	"librevita.org/pkg/log"
)

type sessionStore struct {
	kv.Store
}

// Module provides session management, password hashing, and authentication services.
var Module = fx.Module("auth",
	fx.Provide(
		provideSessionStore,
		provideSessionRepository,
		providePlatformSessionRepository,
		provideSessionManager,
		NewCSRF,
	),
	fx.Invoke(registerSessionCleaner),
)

func provideSessionStore(cfg *config.Config, lc fx.Lifecycle, logger log.Logger) (sessionStore, error) {
	kvCfg := cfg.Sessions
	if kvCfg.BBolt.Path == "" {
		kvCfg.BBolt.Path = filepath.Join(cfg.DataDir, "sessions.db")
	}
	logger.Info("initializing sessions store",
		log.String("backend", kvCfg.Backend),
		log.String("bbolt_path", kvCfg.BBolt.Path),
	)
	store, err := kv.Open(kvCfg)
	if err != nil {
		return sessionStore{}, errors.Wrap(err, "auth: open sessions")
	}
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("closing sessions store")
			return store.Close()
		},
	})
	return sessionStore{Store: store}, nil
}

func provideSessionRepository(store sessionStore, client *ent.Client) SessionRepository {
	return NewSessionRepository(store.Store, client)
}

func providePlatformSessionRepository(store sessionStore, client *ent.Client) PlatformSessionRepository {
	return NewPlatformSessionRepository(store.Store, client)
}

func provideSessionManager(repo SessionRepository, platform PlatformSessionRepository, cfg *config.Config, logger log.Logger) (*SessionManager, error) {
	m, err := NewSessionManager(repo, cfg, logger)
	if err != nil {
		return nil, err
	}
	m.SetPlatformRepository(platform)
	return m, nil
}

func registerSessionCleaner(lc fx.Lifecycle, sessions *SessionManager, logger log.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				ticker := time.NewTicker(time.Hour)
				defer ticker.Stop()
				for {
					if err := sessions.CleanupExpired(ctx); err != nil && ctx.Err() == nil {
						logger.Warn("auth: cleanup expired sessions", log.Error(err))
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
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
}
