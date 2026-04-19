package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const requestIDHeader = "X-Request-Id"

// RequestID injects a stable request id into request and response headers.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := strings.TrimSpace(r.Header.Get(requestIDHeader))
			if id == "" {
				id = generateRequestID()
			}
			r.Header.Set(requestIDHeader, id)
			w.Header().Set(requestIDHeader, id)
			next.ServeHTTP(w, r)
		})
	}
}

func generateRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
