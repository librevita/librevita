package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
)

// RequestLog returns middleware that logs one line per served request:
// the method, path, status, latency, client address and the request id
// produced by the RequestID middleware above. Probes (/healthz) are
// skipped — load balancers poll them continuously and the noise would
// bury real requests.
//
// The level follows the status code: Info for 2xx, Warn for 4xx, Error
// for 5xx. It sits outside Recover in the chain so a recovered panic
// surfaces in the log as a 500.
func RequestLog(log *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req.URL.Path == "/healthz" {
				return next(c)
			}

			start := time.Now()
			err := next(c)

			// The HTTPErrorHandler runs after the middleware chain, so a
			// handler that returns an error has not committed the response
			// yet; derive its status from the error itself. Recovered
			// panics, by contrast, are committed as 500 by Recover before
			// control reaches here.
			status := c.Response().Status
			var he *echo.HTTPError
			if !c.Response().Committed {
				switch {
				case err == nil:
					status = http.StatusOK
				case errors.As(err, &he):
					status = he.Code
				default:
					status = http.StatusInternalServerError
				}
			}

			attrs := []slog.Attr{
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.Int("status", status),
				slog.Duration("latency", time.Since(start)),
				slog.String("remote", c.RealIP()),
			}
			if rid := c.Response().Header().Get(echo.HeaderXRequestID); rid != "" {
				attrs = append(attrs, slog.String("request_id", rid))
			} else if rid := req.Header.Get(echo.HeaderXRequestID); rid != "" {
				attrs = append(attrs, slog.String("request_id", rid))
			}

			switch {
			case status >= http.StatusInternalServerError:
				log.LogAttrs(req.Context(), slog.LevelError, "request", attrs...)
			case status >= http.StatusBadRequest:
				log.LogAttrs(req.Context(), slog.LevelWarn, "request", attrs...)
			default:
				log.LogAttrs(req.Context(), slog.LevelInfo, "request", attrs...)
			}
			return err
		}
	}
}
