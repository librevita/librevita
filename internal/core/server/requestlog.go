package server

import (
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"

	"librevita.org/pkg/log"
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
func RequestLog(logger log.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			rid := c.Response().Header().Get(echo.HeaderXRequestID)
			if rid == "" {
				rid = req.Header.Get(echo.HeaderXRequestID)
			}
			if rid != "" {
				c.SetRequest(req.WithContext(log.WithRequestID(req.Context(), rid)))
				req = c.Request()
			}
			if req.URL.Path == healthzPath {
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

			fields := []log.Field{
				log.String("method", req.Method),
				log.String("path", req.URL.Path),
				log.Int("status", status),
				log.Duration("latency", time.Since(start)),
				log.String("remote", c.RealIP()),
			}

			switch {
			case status >= http.StatusInternalServerError:
				logger.ErrorContext(req.Context(), "request", fields...)
			case status >= http.StatusBadRequest:
				logger.WarnContext(req.Context(), "request", fields...)
			default:
				logger.InfoContext(req.Context(), "request", fields...)
			}
			return err
		}
	}
}
