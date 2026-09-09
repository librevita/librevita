package server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/acme"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/pkg/errors"
	"librevita.org/pkg/log"
)

// Module manages the HTTP server lifecycle through Fx.
var Module = fx.Module("server",
	fx.Provide(New),
	fx.Invoke(registerLifecycle, registerNotFound),
)

type serverParams struct {
	fx.In
	Lifecycle   fx.Lifecycle
	Echo        *echo.Echo
	Config      *config.Config
	Logger      log.Logger
	Shutdown    fx.Shutdowner
	ACMEManager *acme.Manager `optional:"true"`
}

// registerLifecycle starts Echo (and HTTPS / redirect listeners if enabled) and shuts them down gracefully.
func registerLifecycle(p serverParams) {
	if p.ACMEManager != nil {
		p.Echo.GET("/.well-known/acme-challenge/:token", p.ACMEManager.HTTP01Registry().Handler())
	}

	if p.Config.TLS.Enabled {
		registerTLSLifecycle(p)
		return
	}
	registerPlainHTTPLifecycle(p)
}

func registerPlainHTTPLifecycle(p serverParams) {
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				httpAddr := net.JoinHostPort(p.Config.HTTPBind, strconv.Itoa(p.Config.HTTPPort))
				p.Logger.Info("HTTP server listening", log.String("addr", httpAddr))
				if err := p.Echo.Start(httpAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
					p.Logger.Error("HTTP server failed", log.Error(err))
					_ = p.Shutdown.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("shutting down HTTP server")
			return p.Echo.Shutdown(ctx)
		},
	})
}

func registerTLSLifecycle(p serverParams) {
	tlsConfig, err := buildTLSConfig(p)
	if err != nil {
		p.Logger.Error("failed to build TLS config", log.Error(err))
		_ = p.Shutdown.Shutdown()
		return
	}

	httpsAddr := net.JoinHostPort(p.Config.TLS.HTTPSBind, strconv.Itoa(p.Config.TLS.HTTPSPort))
	httpsServer := &http.Server{
		Addr:              httpsAddr,
		Handler:           p.Echo,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
	}

	httpAddr := net.JoinHostPort(p.Config.HTTPBind, strconv.Itoa(p.Config.HTTPPort))
	httpServer := buildHTTPRedirectServer(p, httpAddr)

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				p.Logger.Info("HTTPS server listening", log.String("addr", httpsAddr))
				ln, err := net.Listen("tcp", httpsAddr)
				if err != nil {
					p.Logger.Error("HTTPS listen failed", log.Error(err))
					_ = p.Shutdown.Shutdown()
					return
				}
				tlsListener := tls.NewListener(ln, tlsConfig)
				if err := httpsServer.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
					p.Logger.Error("HTTPS server failed", log.Error(err))
					_ = p.Shutdown.Shutdown()
				}
			}()

			go func() {
				p.Logger.Info("HTTP listener running", log.String("addr", httpAddr))
				if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					p.Logger.Error("HTTP server failed", log.Error(err))
					_ = p.Shutdown.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("shutting down HTTPS and HTTP servers")
			_ = httpServer.Shutdown(ctx)
			return httpsServer.Shutdown(ctx)
		},
	})
}

func buildTLSConfig(p serverParams) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if p.Config.TLS.CertFile != "" && p.Config.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(p.Config.TLS.CertFile, p.Config.TLS.KeyFile)
		if err != nil {
			return nil, errors.Wrap(err, "server: load static tls keypair")
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	} else if p.ACMEManager != nil {
		tlsConfig.GetCertificate = p.ACMEManager.GetCertificate
	} else {
		return nil, errors.New("server: tls is enabled but neither static cert nor acme manager is available")
	}
	return tlsConfig, nil
}

func buildHTTPRedirectServer(p serverParams, addr string) *http.Server {
	var handler http.Handler
	if p.Config.TLS.RedirectHTTP {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") && p.ACMEManager != nil {
				token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
				if keyAuth, ok := p.ACMEManager.HTTP01Registry().Lookup(token); ok {
					w.Header().Set("Content-Type", "application/octet-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(keyAuth)) // #nosec G705 -- ACME key authorization token is ASCII digest.
					return
				}
				http.NotFound(w, r)
				return
			}
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if p.Config.TLS.HTTPSPort != 443 {
				host = net.JoinHostPort(host, strconv.Itoa(p.Config.TLS.HTTPSPort))
			}
			http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusMovedPermanently) // #nosec G710 -- Internal redirect from HTTP to HTTPS preserving request path.
		})
	} else {
		handler = p.Echo
	}

	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
	}
}

// registerNotFound sends unauthenticated navigation on unknown routes to
// the login page (remembering the destination), so every URL — valid or
// not — returns to where the user was going after signing in. Unknown
// routes remain 404 for authenticated users and non-GET methods.
func registerNotFound(e *echo.Echo, sessions *auth.SessionManager) {
	echo.NotFoundHandler = func(c echo.Context) error {
		req := c.Request()
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			return echo.ErrNotFound
		}
		path := req.URL.Path
		if path == LoginPath || path == "/setup" || strings.HasPrefix(path, "/static/") || path == healthzPath {
			return echo.ErrNotFound
		}
		if cookie, err := c.Cookie(auth.SessionCookieName); err == nil {
			if _, err := sessions.Authenticate(req.Context(), cookie.Value); err == nil {
				return echo.ErrNotFound
			}
		}
		return redirectLogin(c)
	}
}
