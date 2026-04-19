package middleware

import "net/http"

// Middleware wraps an HTTP handler.
type Middleware func(http.Handler) http.Handler

// Chain composes middleware around a final handler.
func Chain(final http.Handler, middlewares ...Middleware) http.Handler {
	wrapped := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
