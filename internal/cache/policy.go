package cache

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Policy controls how a response should be cached.
type Policy struct {
	Cacheable                 bool
	ExpiresAt                 time.Time
	StaleWhileRevalidateUntil time.Time
	StaleIfErrorUntil         time.Time
	Vary                      []string
}

// PolicyDefaults controls fallback values when origin headers are absent.
type PolicyDefaults struct {
	TTL                  time.Duration
	StaleWhileRevalidate time.Duration
	StaleIfError         time.Duration
}

// PolicyFromResponseHeaders builds a cache policy from response headers.
func PolicyFromResponseHeaders(header http.Header, now time.Time, defaults PolicyDefaults) Policy {
	policy := Policy{Cacheable: true}

	cacheControl := parseCacheControl(header.Get("Cache-Control"))
	if hasDirective(cacheControl, "no-store") || hasDirective(cacheControl, "private") {
		return Policy{Cacheable: false}
	}

	varyValues := splitCSV(header.Values("Vary"))
	if hasWildcard(varyValues) {
		return Policy{Cacheable: false}
	}
	policy.Vary = normalizeHeaders(varyValues)

	ttl := defaults.TTL
	if v, ok := getDirectiveInt(cacheControl, "s-maxage"); ok {
		ttl = time.Duration(v) * time.Second
	} else if v, ok := getDirectiveInt(cacheControl, "max-age"); ok {
		ttl = time.Duration(v) * time.Second
	} else if expires := strings.TrimSpace(header.Get("Expires")); expires != "" {
		if parsed, err := http.ParseTime(expires); err == nil {
			ttl = parsed.Sub(now)
		}
	}
	if ttl < 0 {
		ttl = 0
	}

	if ttl > 0 {
		policy.ExpiresAt = now.Add(ttl)
	} else {
		policy.ExpiresAt = now
	}

	swr := defaults.StaleWhileRevalidate
	if v, ok := getDirectiveInt(cacheControl, "stale-while-revalidate"); ok {
		swr = time.Duration(v) * time.Second
	}
	sie := defaults.StaleIfError
	if v, ok := getDirectiveInt(cacheControl, "stale-if-error"); ok {
		sie = time.Duration(v) * time.Second
	}
	if swr > 0 {
		policy.StaleWhileRevalidateUntil = policy.ExpiresAt.Add(swr)
	}
	if sie > 0 {
		policy.StaleIfErrorUntil = policy.ExpiresAt.Add(sie)
	}

	if hasDirective(cacheControl, "no-cache") {
		policy.ExpiresAt = now
	}

	return policy
}

func parseCacheControl(v string) map[string]string {
	parts := strings.Split(strings.ToLower(v), ",")
	out := make(map[string]string, len(parts))
	for _, raw := range parts {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		if strings.Contains(token, "=") {
			pair := strings.SplitN(token, "=", 2)
			out[strings.TrimSpace(pair[0])] = strings.Trim(strings.TrimSpace(pair[1]), "\"")
			continue
		}
		out[token] = ""
	}
	return out
}

func hasDirective(directives map[string]string, name string) bool {
	_, ok := directives[name]
	return ok
}

func getDirectiveInt(directives map[string]string, name string) (int, bool) {
	value, ok := directives[name]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func splitCSV(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			if item := strings.TrimSpace(part); item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

func hasWildcard(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "*" {
			return true
		}
	}
	return false
}
