package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"relay/internal/cache"
	"relay/internal/errorhandler"
	relayerrors "relay/internal/errors"
	"relay/internal/logging"
)

const (
	headerXCache       = "X-Cache"
	headerXCacheDetail = "X-Cache-Detail"
	cacheHit           = "HIT"
	cacheMiss          = "MISS"
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

// HandlerOptions controls proxy behavior and performance.
type HandlerOptions struct {
	Origin                *url.URL
	Cache                 *cache.Store
	Logger                *logging.Logger
	ErrorHandler          *errorhandler.Handler
	RequestTimeout        time.Duration
	DialTimeout           time.Duration
	IdleConnTimeout       time.Duration
	ResponseHeaderTimeout time.Duration
	MaxResponseHeaderBytes int64
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	RetryCount            int
	RetryBackoff          time.Duration
	CacheMethods          []string
	CacheBypassPaths      []string
	CacheBypassHeaders    []string
	PolicyDefaults        cache.PolicyDefaults
}

// Handler proxies requests to an origin and caches configured methods.
type Handler struct {
	origin       *url.URL
	cache        *cache.Store
	client       *http.Client
	logger       *logging.Logger
	errorHandler *errorhandler.Handler
	retryCount   int
	retryBackoff time.Duration

	cacheMethods       map[string]struct{}
	cacheBypassPaths   []string
	cacheBypassHeaders map[string]struct{}
	policyDefaults     cache.PolicyDefaults

	inflight *coalescer
}

// NewHandler creates a backward-compatible proxy handler constructor.
func NewHandler(origin *url.URL, store *cache.Store, stdLogger *log.Logger) (*Handler, error) {
	_ = stdLogger
	logger := logging.New("info", false)
	return NewHandlerWithOptions(HandlerOptions{
		Origin:                origin,
		Cache:                 store,
		Logger:                logger,
		ErrorHandler:          errorhandler.New(logger, false),
		RequestTimeout:        30 * time.Second,
		DialTimeout:           10 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   128,
		MaxConnsPerHost:       256,
		RetryCount:            2,
		RetryBackoff:          100 * time.Millisecond,
		CacheMethods:          []string{http.MethodGet},
		CacheBypassHeaders:    []string{"Authorization"},
		PolicyDefaults: cache.PolicyDefaults{
			TTL:                  60 * time.Second,
			StaleWhileRevalidate: 30 * time.Second,
			StaleIfError:         5 * time.Minute,
		},
	})
}

// NewHandlerWithOptions creates a configurable proxy handler.
func NewHandlerWithOptions(opts HandlerOptions) (*Handler, error) {
	if opts.Origin == nil {
		return nil, relayerrors.New(relayerrors.CategoryConfig, "missing_origin", "origin is required")
	}
	if opts.Cache == nil {
		return nil, relayerrors.New(relayerrors.CategoryConfig, "missing_cache", "cache store is required")
	}
	if opts.Logger == nil {
		opts.Logger = logging.New("info", false)
	}
	if opts.ErrorHandler == nil {
		opts.ErrorHandler = errorhandler.New(opts.Logger, false)
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 30 * time.Second
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 10 * time.Second
	}
	if opts.IdleConnTimeout <= 0 {
		opts.IdleConnTimeout = 90 * time.Second
	}
	if opts.ResponseHeaderTimeout <= 0 {
		opts.ResponseHeaderTimeout = 15 * time.Second
	}
	if opts.MaxResponseHeaderBytes <= 0 {
		opts.MaxResponseHeaderBytes = 1 << 20
	}
	if opts.MaxIdleConns <= 0 {
		opts.MaxIdleConns = 512
	}
	if opts.MaxIdleConnsPerHost <= 0 {
		opts.MaxIdleConnsPerHost = 128
	}
	if opts.MaxConnsPerHost <= 0 {
		opts.MaxConnsPerHost = 256
	}
	if opts.RetryCount < 0 {
		opts.RetryCount = 0
	}
	if opts.RetryBackoff <= 0 {
		opts.RetryBackoff = 100 * time.Millisecond
	}
	if len(opts.CacheMethods) == 0 {
		opts.CacheMethods = []string{http.MethodGet}
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: opts.DialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          opts.MaxIdleConns,
		MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		MaxConnsPerHost:       opts.MaxConnsPerHost,
		IdleConnTimeout:       opts.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
		MaxResponseHeaderBytes: opts.MaxResponseHeaderBytes,
		ExpectContinueTimeout: 1 * time.Second,
	}

	h := &Handler{
		origin:             opts.Origin,
		cache:              opts.Cache,
		client:             &http.Client{Timeout: opts.RequestTimeout, Transport: transport},
		logger:             opts.Logger,
		errorHandler:       opts.ErrorHandler,
		retryCount:         opts.RetryCount,
		retryBackoff:       opts.RetryBackoff,
		cacheMethods:       make(map[string]struct{}, len(opts.CacheMethods)),
		cacheBypassPaths:   append([]string(nil), opts.CacheBypassPaths...),
		cacheBypassHeaders: make(map[string]struct{}, len(opts.CacheBypassHeaders)),
		policyDefaults:     opts.PolicyDefaults,
		inflight:           newCoalescer(),
	}

	for _, m := range opts.CacheMethods {
		h.cacheMethods[strings.ToUpper(strings.TrimSpace(m))] = struct{}{}
	}
	for _, header := range opts.CacheBypassHeaders {
		h.cacheBypassHeaders[http.CanonicalHeaderKey(strings.TrimSpace(header))] = struct{}{}
	}

	if h.policyDefaults.TTL <= 0 {
		h.policyDefaults.TTL = 60 * time.Second
	}

	return h, nil
}

// ServeHTTP handles incoming requests with cache-aware proxying.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.shouldBypassCache(r) || !h.isCacheMethod(r.Method) {
		h.forwardDirect(w, r)
		return
	}

	baseKey := cache.BuildBaseKey(r.Method, r.URL)
	entry, state, _ := h.cache.Lookup(baseKey, r.Header)
	now := time.Now()

	switch state {
	case cache.StateHit:
		writeCachedResponse(w, entry, cacheHit, "")
		return
	case cache.StateStale:
		if entry.CanServeStaleWhileRevalidate(now) {
			h.revalidateInBackground(r, baseKey, entry)
			writeCachedResponse(w, entry, cacheHit, "STALE")
			return
		}
	}

	result, err := h.inflight.Do(baseKey, func() (originResult, error) {
		return h.fetchAndCache(r, baseKey, entry, state == cache.StateStale)
	})
	if err != nil {
		if state == cache.StateStale && entry.CanServeStaleIfError(now) {
			writeCachedResponse(w, entry, cacheHit, "STALE_IF_ERROR")
			return
		}
		w.Header().Set(headerXCache, cacheMiss)
		h.errorHandler.WriteHTTP(w, err)
		return
	}

	writeOriginResult(w, result)
}

type originResult struct {
	status      int
	header      http.Header
	body        []byte
	cacheStatus string
	detail      string
}

func (h *Handler) fetchAndCache(r *http.Request, baseKey string, staleEntry cache.Entry, hasStale bool) (originResult, error) {
	targetURL := joinURL(h.origin, r.URL)

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), nil)
	if err != nil {
		return originResult{}, relayerrors.Wrap(relayerrors.CategoryInternal, "create_request", "failed to create outbound request", err)
	}
	copyRequestHeaders(outReq.Header, r.Header)
	outReq.Host = h.origin.Host

	if hasStale {
		if staleEntry.ETag != "" {
			outReq.Header.Set("If-None-Match", staleEntry.ETag)
		}
		if staleEntry.LastModified != "" {
			outReq.Header.Set("If-Modified-Since", staleEntry.LastModified)
		}
	}

	resp, err := h.doWithRetry(outReq)
	if err != nil {
		return originResult{}, classifyOriginError(err)
	}
	defer resp.Body.Close()

	if hasStale && resp.StatusCode == http.StatusNotModified {
		mergedHeaders := mergeHeaders(staleEntry.Header, resp.Header)
		staleEntry.Header = mergedHeaders
		policy := cache.PolicyFromResponseHeaders(mergedHeaders, time.Now(), h.policyDefaults)
		_, _ = h.cache.SetWithRequest(baseKey, r.Header, staleEntry, policy)
		return originResult{
			status:      staleEntry.StatusCode,
			header:      sanitizeHeaders(mergedHeaders),
			body:        staleEntry.Body,
			cacheStatus: cacheHit,
			detail:      "REVALIDATED",
		}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return originResult{}, relayerrors.Wrap(relayerrors.CategoryNetwork, "read_origin_response", "failed to read origin response", err)
	}

	headers := sanitizeHeaders(resp.Header)
	if hasStale && resp.StatusCode >= 500 {
		return originResult{}, relayerrors.New(relayerrors.CategoryNetwork, "origin_server_error", fmt.Sprintf("origin returned %d", resp.StatusCode))
	}

	result := originResult{
		status:      resp.StatusCode,
		header:      headers,
		body:        body,
		cacheStatus: cacheMiss,
	}

	policy := cache.PolicyFromResponseHeaders(headers, time.Now(), h.policyDefaults)
	if isCacheableStatus(resp.StatusCode) {
		_, _ = h.cache.SetWithRequest(baseKey, r.Header, cache.Entry{
			StatusCode: resp.StatusCode,
			Header:     headers,
			Body:       body,
		}, policy)
	}

	return result, nil
}

func (h *Handler) forwardDirect(w http.ResponseWriter, r *http.Request) {
	targetURL := joinURL(h.origin, r.URL)
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		w.Header().Set(headerXCache, cacheMiss)
		h.errorHandler.WriteHTTP(w, relayerrors.Wrap(relayerrors.CategoryInternal, "create_request", "failed to create outbound request", err))
		return
	}
	copyRequestHeaders(outReq.Header, r.Header)
	outReq.Host = h.origin.Host

	resp, err := h.doWithRetry(outReq)
	if err != nil {
		w.Header().Set(headerXCache, cacheMiss)
		h.errorHandler.WriteHTTP(w, classifyOriginError(err))
		return
	}
	defer resp.Body.Close()

	for k, values := range sanitizeHeaders(resp.Header) {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.Header().Set(headerXCache, cacheMiss)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *Handler) revalidateInBackground(r *http.Request, baseKey string, staleEntry cache.Entry) {
	cloned := r.Clone(context.Background())
	go func() {
		_, err := h.inflight.Do(baseKey, func() (originResult, error) {
			return h.fetchAndCache(cloned, baseKey, staleEntry, true)
		})
		if err != nil {
			h.logger.Warn("background revalidation failed", map[string]any{"path": r.URL.Path, "error": err.Error()})
		}
	}()
}

func (h *Handler) doWithRetry(req *http.Request) (*http.Response, error) {
	attempts := h.retryCount + 1
	idempotent := isIdempotentMethod(req.Method)

	for attempt := 1; attempt <= attempts; attempt++ {
		attemptReq, err := cloneRequest(req)
		if err != nil {
			return nil, relayerrors.Wrap(relayerrors.CategoryInternal, "clone_request", "failed to clone request", err)
		}
		resp, err := h.client.Do(attemptReq)
		if err == nil {
			if resp.StatusCode >= 500 && idempotent && attempt < attempts {
				resp.Body.Close()
				time.Sleep(backoffDuration(h.retryBackoff, attempt))
				continue
			}
			return resp, nil
		}

		if !idempotent || attempt >= attempts || !isRetriableError(err) {
			return nil, err
		}
		time.Sleep(backoffDuration(h.retryBackoff, attempt))
	}

	return nil, relayerrors.New(relayerrors.CategoryInternal, "retry_exhausted", "request retries exhausted")
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Body != nil && req.GetBody == nil && req.ContentLength != 0 {
		return nil, errors.New("cannot clone non-rewindable request body")
	}

	var body io.ReadCloser
	if req.GetBody != nil {
		replayed, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		body = replayed
	}

	cloned, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), body)
	if err != nil {
		return nil, err
	}
	copyRequestHeaders(cloned.Header, req.Header)
	cloned.Host = req.Host
	cloned.ContentLength = req.ContentLength
	return cloned, nil
}

func backoffDuration(base time.Duration, attempt int) time.Duration {
	multiplier := 1 << (attempt - 1)
	return time.Duration(multiplier) * base
}

func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
}

func isIdempotentMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func classifyOriginError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return relayerrors.Wrap(relayerrors.CategoryTimeout, "origin_timeout", "origin request timed out", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return relayerrors.Wrap(relayerrors.CategoryTimeout, "origin_timeout", "origin request timed out", err)
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return relayerrors.Wrap(relayerrors.CategoryNetwork, "dns_failure", "dns resolution failed for origin", err)
	}
	return relayerrors.Wrap(relayerrors.CategoryNetwork, "origin_network_error", "origin request failed", err)
}

func (h *Handler) isCacheMethod(method string) bool {
	_, ok := h.cacheMethods[strings.ToUpper(strings.TrimSpace(method))]
	return ok
}

func (h *Handler) shouldBypassCache(r *http.Request) bool {
	for _, prefix := range h.cacheBypassPaths {
		if prefix != "" && strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}
	for header := range h.cacheBypassHeaders {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return true
		}
	}
	cacheControl := strings.ToLower(strings.TrimSpace(r.Header.Get("Cache-Control")))
	if strings.Contains(cacheControl, "no-store") || strings.Contains(cacheControl, "no-cache") {
		return true
	}
	pragma := strings.ToLower(strings.TrimSpace(r.Header.Get("Pragma")))
	if strings.Contains(pragma, "no-cache") {
		return true
	}
	return false
}

func writeOriginResult(w http.ResponseWriter, result originResult) {
	for k, values := range result.header {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.Header().Set(headerXCache, result.cacheStatus)
	if result.detail != "" {
		w.Header().Set(headerXCacheDetail, result.detail)
	}
	w.WriteHeader(result.status)
	_, _ = w.Write(result.body)
}

func writeCachedResponse(w http.ResponseWriter, entry cache.Entry, cacheStatus, detail string) {
	for k, values := range sanitizeHeaders(entry.Header) {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.Header().Set(headerXCache, cacheStatus)
	if detail != "" {
		w.Header().Set(headerXCacheDetail, detail)
	}
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

func mergeHeaders(base, delta http.Header) http.Header {
	merged := sanitizeHeaders(base)
	for k, values := range sanitizeHeaders(delta) {
		copiedValues := make([]string, len(values))
		copy(copiedValues, values)
		merged[k] = copiedValues
	}
	return merged
}

func isCacheableStatus(status int) bool {
	switch status {
	case http.StatusOK,
		http.StatusNonAuthoritativeInfo,
		http.StatusNoContent,
		http.StatusPartialContent,
		http.StatusMultipleChoices,
		http.StatusMovedPermanently,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusGone,
		http.StatusRequestURITooLong,
		http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

type coalescer struct {
	mu    sync.Mutex
	calls map[string]*coalescedCall
}

type coalescedCall struct {
	wg  sync.WaitGroup
	res originResult
	err error
}

func newCoalescer() *coalescer {
	return &coalescer{calls: make(map[string]*coalescedCall)}
}

func (c *coalescer) Do(key string, fn func() (originResult, error)) (originResult, error) {
	c.mu.Lock()
	if call, ok := c.calls[key]; ok {
		c.mu.Unlock()
		call.wg.Wait()
		return call.res, call.err
	}
	call := &coalescedCall{}
	call.wg.Add(1)
	c.calls[key] = call
	c.mu.Unlock()

	call.res, call.err = fn()
	call.wg.Done()

	c.mu.Lock()
	delete(c.calls, key)
	c.mu.Unlock()

	return call.res, call.err
}
