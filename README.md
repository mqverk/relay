# relay

`relay` is an HTTP caching proxy server written in Go.

It accepts incoming requests, forwards them to a configured origin, caches origin responses, and returns cached responses for repeated identical requests.

## Features

- CLI-first operation
- Reverse proxy forwarding to a user-provided origin URL
- Thread-safe in-memory cache with optional TTL support in code
- Cache key based on HTTP method + full request URI (path + query)
- `X-Cache` response header on every response (`HIT` or `MISS`)
- Cache clear command (`--clear-cache`)
- Graceful shutdown on SIGINT/SIGTERM
- Unit tests for CLI, cache, and proxy behavior

## Requirements

- Go (latest stable)

## Build

```bash
go build ./...
```

## Run

```bash
go run main.go --port 3000 --origin http://dummyjson.com
```

Example request:

```bash
curl -i http://localhost:3000/products
```

The first request returns `X-Cache: MISS`; repeated identical requests return `X-Cache: HIT`.

## CLI

Start server:

```bash
relay --port <number> --origin <url>
```

Clear cache and exit:

```bash
relay --clear-cache
```

Show help:

```bash
relay --help
```

## Caching behavior

- Cached by method + full request URI (`path + query`)
- Currently caches `GET` responses
- Non-`GET` requests are proxied without caching
- Cached response preserves status code, headers, and body

## Project structure

- `main.go` - CLI entry point and runtime orchestration
- `internal/cli` - flag parsing and validation
- `internal/cache` - in-memory cache store and runtime cache helpers
- `internal/proxy` - proxy handler, cache hit/miss flow, header handling
- `internal/server` - HTTP server lifecycle and graceful shutdown

## Notes

- Cache storage is in-memory and process-local.
- `--clear-cache` clears the runtime cache in the current process and exits.
