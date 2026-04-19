package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"relay/internal/cache"
	"relay/internal/config"
	"relay/internal/errorhandler"
	"relay/internal/logging"
	"relay/internal/metrics"
	"relay/internal/middleware"
	"relay/internal/proxy"
	"relay/internal/server"
)

// Run executes the relay application and returns a process exit code.
func Run(args []string) int {
	cfg, err := config.Parse(args)
	if err != nil {
		if errors.Is(err, config.ErrHelpRequested) {
			fmt.Print(config.Usage())
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, config.Usage())
		return 2
	}

	logger := logging.New(cfg.LogLevel, cfg.Debug)
	errHandler := errorhandler.New(logger, cfg.Debug)

	store := cache.ConfigureDefaultStore(cache.Options{
		DefaultTTL:           cfg.TTL,
		StaleWhileRevalidate: cfg.StaleWhileRevalidate,
		StaleIfError:         cfg.StaleIfError,
		MaxEntries:           cfg.CacheMaxEntries,
		MaxEntryBytes:        int64(cfg.CacheMaxEntryBytes),
		MaxBytes:             int64(cfg.CacheMaxEntries * cfg.CacheMaxEntryBytes),
	})

	if cfg.ClearCache {
		cache.ClearDefault()
		logger.Info("cache cleared", nil)
		return 0
	}

	if cfg.CacheStats {
		stats := store.Stats()
		fmt.Printf("entries=%d bytes=%d hits=%d misses=%d hit_ratio=%.4f\n", stats.Entries, stats.SizeBytes, stats.Hits, stats.Misses, stats.HitRatio)
		return 0
	}

	metricsReg := metrics.New()
	metricsReg.SetCacheSnapshot(metrics.CacheSnapshot{
		Entries:   store.Stats().Entries,
		SizeBytes: store.Stats().SizeBytes,
		Hits:      store.Stats().Hits,
		Misses:    store.Stats().Misses,
		HitRatio:  store.Stats().HitRatio,
	})

	h, err := proxy.NewHandlerWithOptions(proxy.HandlerOptions{
		Origin:                cfg.Origin,
		Cache:                 store,
		Logger:                logger,
		ErrorHandler:          errHandler,
		RequestTimeout:        cfg.RequestTimeout,
		DialTimeout:           cfg.DialTimeout,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		MaxResponseHeaderBytes: cfg.MaxResponseHeaderBytes,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		RetryCount:            cfg.RetryCount,
		RetryBackoff:          cfg.RetryBackoff,
		CacheMethods:          cfg.CacheMethods,
		CacheBypassPaths:      cfg.CacheBypassPaths,
		CacheBypassHeaders:    cfg.CacheBypassHeaders,
		PolicyDefaults: cache.PolicyDefaults{
			TTL:                  cfg.TTL,
			StaleWhileRevalidate: cfg.StaleWhileRevalidate,
			StaleIfError:         cfg.StaleIfError,
		},
	})
	if err != nil {
		logger.Error("failed to create proxy handler", map[string]any{"error": err.Error()})
		return 1
	}

	adminPrefix := normalizePathPrefix(cfg.AdminPrefix)
	metricsPath := normalizeRoute(cfg.MetricsPath)
	cacheAdminPath := path.Join(adminPrefix, "cache")

	mux := http.NewServeMux()
	mux.Handle(metricsPath, metricsReg.Handler())
	mux.Handle(cacheAdminPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			stats := store.Stats()
			metricsReg.SetCacheSnapshot(metrics.CacheSnapshot{Entries: stats.Entries, SizeBytes: stats.SizeBytes, Hits: stats.Hits, Misses: stats.Misses, HitRatio: stats.HitRatio})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"entries":    stats.Entries,
				"size_bytes": stats.SizeBytes,
				"hits":       stats.Hits,
				"misses":     stats.Misses,
				"stale_hits": stats.StaleHits,
				"evictions":  stats.Evictions,
				"hit_ratio":  stats.HitRatio,
			})
		case http.MethodDelete:
			store.Clear()
			metricsReg.SetCacheSnapshot(metrics.CacheSnapshot{})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "cache_cleared"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	mux.Handle("/", h)

	limiter := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	handler := middleware.Chain(
		mux,
		middleware.Logging(logger),
		middleware.Metrics(metricsReg),
		limiter.Middleware(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := store.Stats()
				metricsReg.SetCacheSnapshot(metrics.CacheSnapshot{Entries: stats.Entries, SizeBytes: stats.SizeBytes, Hits: stats.Hits, Misses: stats.Misses, HitRatio: stats.HitRatio})
			}
		}
	}()

	if err := server.Run(ctx, cfg.Port, handler, log.Default()); err != nil {
		logger.Error("server failed", map[string]any{"error": err.Error()})
		return 1
	}

	return 0
}

func normalizePathPrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/_relay"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func normalizeRoute(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/_relay/metrics"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}
