# relay

relay is a production-grade HTTP caching proxy written in Go.

It forwards requests to an origin, caches eligible responses with policy-aware behavior, serves stale content safely when appropriate, and exposes operational endpoints for metrics and cache administration.

## Highlights

- LRU in-memory cache with configurable limits and thread-safe access
- Cache policy support for `Cache-Control` and `Expires`
- `stale-while-revalidate` and `stale-if-error` support
- Conditional revalidation using `ETag` and `Last-Modified`
- Cache key normalization (method + normalized path/query)
- `Vary`-aware variant caching
- Request coalescing for concurrent identical misses
- Configurable retries with exponential backoff for idempotent methods
- HTTPS origin support and HTTP/2-capable upstream transport
- Structured JSON logging
- Prometheus-compatible metrics endpoint
- Admin HTTP endpoints for cache stats and cache clear
- CLI config layering: flags > environment > config file
- Middleware pipeline (request id, panic recovery, logging, metrics, rate limiting, hooks)
- Rate limit response headers (`X-RateLimit-*`) and `Retry-After` on 429s
- Optional trusted-proxy rate limiting via `X-Forwarded-For` / `X-Real-Ip`

## Requirements

- Go latest stable

## Build

```bash
go build ./...
```

## Run

Start with direct flags:

```bash
go run main.go --port 3000 --origin http://dummyjson.com
```

Start with config file:

```bash
go run main.go --config relay.example.json
```

Alternative entrypoint:

```bash
go run ./cmd/relay --config relay.example.json
```

## CLI Commands

Start proxy:

```bash
relay --port <number> --origin <url>
```

Clear cache and exit:

```bash
relay --clear-cache
```

Print cache stats and exit:

```bash
relay --cache-stats
```

Help:

```bash
relay --help
```

## Configuration Precedence

1. CLI flags
2. Environment variables (`RELAY_*`)
3. Config file (`--config` JSON or YAML)

Sample config file is provided at `relay.example.json`.

## Admin & Metrics Endpoints

Default endpoints:

- `GET /_relay/cache` returns cache stats as JSON
- `DELETE /_relay/cache` clears cache entries
- `GET /_relay/metrics` returns Prometheus metrics

Metrics include:

- Total requests (`relay_requests_total`)
- Request latency histogram (`relay_request_duration_seconds`)
- Cache decisions by state (`relay_cache_decisions_total`)
- Cache decision details (`relay_cache_decision_details_total`)
- Cache entries and size
- Cache hits/misses and hit ratio

## Caching Behavior

- Cacheable methods are configurable (`cache_methods`)
- Bypass rules can be configured by path prefix and header presence
- Response caching respects:
	- `Cache-Control` directives (`max-age`, `s-maxage`, `no-store`, `private`, `stale-while-revalidate`, `stale-if-error`)
	- `Expires`
	- `Vary`
- On stale entries:
	- serve stale and revalidate in background when allowed
	- serve stale on upstream errors when `stale-if-error` allows it

## Architecture

- `cmd/relay` entrypoint
- `internal/app` runtime orchestration
- `internal/config` layered configuration management
- `internal/cache` LRU cache and policy handling
- `internal/proxy` forwarding, caching, retries, revalidation, coalescing
- `internal/middleware` request-id/recovery/logging/metrics/rate-limit/hooks pipeline
- `internal/metrics` Prometheus metrics registry
- `internal/logging` structured JSON logging
- `internal/errors` error taxonomy
- `internal/errorhandler` centralized HTTP error mapping
- `internal/erroradvisor` actionable error suggestions
- `internal/server` HTTP server lifecycle and graceful shutdown

## Developer Commands

```bash
make build
make test
make run
```

If `make` is unavailable on your platform, run the equivalent Go commands directly.
