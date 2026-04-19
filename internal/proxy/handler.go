package proxy

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"relay/internal/cache"
)

const (
	headerXCache = "X-Cache"
	cacheHit     = "HIT"
	cacheMiss    = "MISS"
)

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

// Handler proxies requests to an origin and caches GET responses.
type Handler struct {
	origin *url.URL
	cache  *cache.Store
	client *http.Client
	logger *log.Logger
}

// NewHandler creates a proxy handler.
func NewHandler(origin *url.URL, store *cache.Store, logger *log.Logger) (*Handler, error) {
	if origin == nil {
		return nil, errors.New("origin is required")
	}
	if store == nil {
		return nil, errors.New("cache store is required")
	}
	if logger == nil {
		logger = log.Default()
	}

	return &Handler{
		origin: origin,
		cache:  store,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}, nil
}

// ServeHTTP handles and proxies incoming HTTP requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if entry, ok := h.cache.Get(cacheKey(r)); ok {
			writeCachedResponse(w, entry)
			return
		}
	}

	if err := h.forwardAndRespond(w, r); err != nil {
		h.logger.Printf("proxy error method=%s path=%s err=%v", r.Method, r.URL.RequestURI(), err)
		h.writeGatewayError(w)
	}
}

func (h *Handler) forwardAndRespond(w http.ResponseWriter, r *http.Request) error {
	targetURL := joinURL(h.origin, r.URL)

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		return fmt.Errorf("create outbound request: %w", err)
	}
	copyRequestHeaders(outReq.Header, r.Header)
	outReq.Host = h.origin.Host

	resp, err := h.client.Do(outReq)
	if err != nil {
		return fmt.Errorf("request origin: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read origin response: %w", err)
	}

	sanitizedHeaders := sanitizeHeaders(resp.Header)
	for k, values := range sanitizedHeaders {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.Header().Set(headerXCache, cacheMiss)
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write response body: %w", err)
	}

	if r.Method == http.MethodGet {
		h.cache.Set(cacheKey(r), cache.Entry{
			StatusCode: resp.StatusCode,
			Header:     sanitizedHeaders,
			Body:       body,
		})
	}

	return nil
}

func (h *Handler) writeGatewayError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set(headerXCache, cacheMiss)
	w.WriteHeader(http.StatusBadGateway)
	_, _ = w.Write([]byte("bad gateway\n"))
}

func writeCachedResponse(w http.ResponseWriter, entry cache.Entry) {
	for k, values := range sanitizeHeaders(entry.Header) {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.Header().Set(headerXCache, cacheHit)
	w.WriteHeader(entry.StatusCode)
	_, _ = w.Write(entry.Body)
}

func copyRequestHeaders(dst, src http.Header) {
	for k, values := range src {
		if _, skip := hopByHopHeaders[http.CanonicalHeaderKey(k)]; skip {
			continue
		}
		for _, value := range values {
			dst.Add(k, value)
		}
	}
}

func sanitizeHeaders(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, values := range src {
		if _, skip := hopByHopHeaders[http.CanonicalHeaderKey(k)]; skip {
			continue
		}
		copiedValues := make([]string, len(values))
		copy(copiedValues, values)
		dst[k] = copiedValues
	}
	return dst
}

func cacheKey(r *http.Request) string {
	return r.Method + " " + r.URL.RequestURI()
}

func joinURL(origin, incoming *url.URL) *url.URL {
	target := *origin
	incomingPath := incoming.Path
	if incomingPath == "" {
		incomingPath = "/"
	}
	target.Path = singleJoiningSlash(origin.Path, incomingPath)
	target.RawPath = ""
	target.RawQuery = incoming.RawQuery
	target.Fragment = ""
	return &target
}

func singleJoiningSlash(a, b string) string {
	if a == "" {
		return b
	}
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}
