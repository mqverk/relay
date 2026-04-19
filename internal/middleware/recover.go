package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"relay/internal/errorhandler"
	relayerrors "relay/internal/errors"
	"relay/internal/logging"
)

// Recover converts panics into structured internal error responses.
func Recover(errHandler *errorhandler.Handler, logger *logging.Logger) Middleware {
	if logger == nil {
		logger = logging.New("error", false)
	}
	if errHandler == nil {
		errHandler = errorhandler.New(logger, false)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered", map[string]any{
						"panic":      fmt.Sprint(recovered),
						"path":       r.URL.Path,
						"method":     r.Method,
						"request_id": r.Header.Get(requestIDHeader),
						"stack":      string(debug.Stack()),
					})

					if rw.wroteHeader {
						return
					}

					err := relayerrors.New(relayerrors.CategoryInternal, "panic_recovered", "internal server error")
					if panicErr, ok := recovered.(error); ok {
						err = relayerrors.Wrap(relayerrors.CategoryInternal, "panic_recovered", "internal server error", panicErr)
					}
					errHandler.WriteHTTP(rw, err)
				}
			}()

			next.ServeHTTP(rw, r)
		})
	}
}
