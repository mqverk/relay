package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type configFormat string

const (
	configFormatJSON configFormat = "json"
	configFormatYAML configFormat = "yaml"
)

// ErrHelpRequested indicates that help output was requested.
var ErrHelpRequested = errors.New("help requested")

// Config contains runtime configuration for relay.
type Config struct {
	ConfigFile            string
	Port                  int
	Origin                *url.URL
	OriginRaw             string
	ClearCache            bool
	CacheStats            bool
	TTL                   time.Duration
	StaleWhileRevalidate  time.Duration
	StaleIfError          time.Duration
	CacheMaxEntries       int
	CacheMaxEntryBytes    int
	CacheMethods          []string
	CacheBypassPaths      []string
	CacheBypassHeaders    []string
	RequestTimeout        time.Duration
	DialTimeout           time.Duration
	IdleConnTimeout       time.Duration
	ResponseHeaderTimeout time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	RetryCount            int
	RetryBackoff          time.Duration
	LogLevel              string
	Debug                 bool
	MetricsPath           string
	AdminPrefix           string
	RateLimitRPS          float64
	RateLimitBurst        int
}

// Usage returns command usage text.
func Usage() string {
	return `relay - production-grade HTTP caching proxy

Usage:
  relay --port <number> --origin <url>
  relay --clear-cache
  relay --cache-stats

Config precedence:
  CLI flags > environment variables > config file

Common options:
	--config <path>                 JSON or YAML config file path
  --port <number>                 listening port
  --origin <url>                  origin base URL
  --ttl <duration>                default cache TTL (e.g. 60s)
  --cache-max-entries <n>         max in-memory entries for LRU
  --cache-max-entry-bytes <n>     max response body bytes eligible for cache
  --request-timeout <duration>    upstream request timeout
  --log-level <level>             debug|info|warn|error
  --debug                         include detailed debug output
  --metrics-path <path>           metrics endpoint path
  --admin-prefix <path>           admin endpoint prefix
`
}

// Parse loads and validates configuration from file, env, and flags.
func Parse(args []string) (Config, error) {
	if hasHelpArg(args) {
		return Config{}, ErrHelpRequested
	}

	cfg := defaultConfig()

	configPath := findConfigPath(args)
	if configPath == "" {
		configPath = os.Getenv("RELAY_CONFIG")
	}
	cfg.ConfigFile = configPath

	if configPath != "" {
		format, err := detectConfigFormat(configPath)
		if err != nil {
			return Config{}, err
		}

		var fc fileConfig
		switch format {
		case configFormatJSON:
			fc, err = loadJSONConfig(configPath)
		case configFormatYAML:
			fc, err = loadYAMLConfig(configPath)
		default:
			return Config{}, fmt.Errorf("unsupported config file format: %s", configPath)
		}
		if err != nil {
			return Config{}, err
		}
		applyFileConfig(&cfg, fc)
	}

	applyEnvConfig(&cfg)
	if err := parseFlags(&cfg, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, ErrHelpRequested
		}
		return Config{}, fmt.Errorf("parse flags: %w", err)
	}

	if cfg.ClearCache || cfg.CacheStats {
		if cfg.ClearCache && cfg.CacheStats {
			return Config{}, errors.New("--clear-cache and --cache-stats cannot be combined")
		}
		return cfg, nil
	}

	if cfg.Port <= 0 || cfg.Port > 65535 {
		return Config{}, errors.New("--port must be between 1 and 65535")
	}
	if cfg.OriginRaw == "" {
		return Config{}, errors.New("--origin is required")
	}
	u, err := url.Parse(cfg.OriginRaw)
	if err != nil {
		return Config{}, fmt.Errorf("parse --origin: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Config{}, errors.New("--origin must use http or https")
	}
	if u.Host == "" {
		return Config{}, errors.New("--origin must include a host")
	}
	cfg.Origin = u

	if cfg.CacheMaxEntries <= 0 {
		return Config{}, errors.New("--cache-max-entries must be greater than 0")
	}
	if cfg.CacheMaxEntryBytes <= 0 {
		return Config{}, errors.New("--cache-max-entry-bytes must be greater than 0")
	}
	if cfg.RateLimitRPS <= 0 {
		return Config{}, errors.New("--rate-limit-rps must be greater than 0")
	}
	if cfg.RateLimitBurst <= 0 {
		return Config{}, errors.New("--rate-limit-burst must be greater than 0")
	}
	if len(cfg.CacheMethods) == 0 {
		return Config{}, errors.New("at least one cache method is required")
	}

	cfg.CacheMethods = normalizeMethods(cfg.CacheMethods)
	cfg.CacheBypassHeaders = normalizeHeaders(cfg.CacheBypassHeaders)

	return cfg, nil
}

// IsCacheMethod reports whether caching is enabled for method.
func (c Config) IsCacheMethod(method string) bool {
	m := strings.ToUpper(strings.TrimSpace(method))
	for _, allowed := range c.CacheMethods {
		if allowed == m {
			return true
		}
	}
	return false
}

func defaultConfig() Config {
	return Config{
		Port:                  3000,
		TTL:                   60 * time.Second,
		StaleWhileRevalidate:  30 * time.Second,
		StaleIfError:          5 * time.Minute,
		CacheMaxEntries:       5000,
		CacheMaxEntryBytes:    1024 * 1024,
		CacheMethods:          []string{httpMethodGet},
		CacheBypassPaths:      nil,
		CacheBypassHeaders:    []string{"Authorization"},
		RequestTimeout:        30 * time.Second,
		DialTimeout:           10 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   128,
		MaxConnsPerHost:       256,
		RetryCount:            2,
		RetryBackoff:          100 * time.Millisecond,
		LogLevel:              "info",
		Debug:                 false,
		MetricsPath:           "/_relay/metrics",
		AdminPrefix:           "/_relay",
		RateLimitRPS:          200,
		RateLimitBurst:        400,
	}
}

const httpMethodGet = "GET"

type fileConfig struct {
	Port                  *int     `json:"port" yaml:"port"`
	Origin                *string  `json:"origin" yaml:"origin"`
	TTL                   *string  `json:"ttl" yaml:"ttl"`
	StaleWhileRevalidate  *string  `json:"stale_while_revalidate" yaml:"stale_while_revalidate"`
	StaleIfError          *string  `json:"stale_if_error" yaml:"stale_if_error"`
	CacheMaxEntries       *int     `json:"cache_max_entries" yaml:"cache_max_entries"`
	CacheMaxEntryBytes    *int     `json:"cache_max_entry_bytes" yaml:"cache_max_entry_bytes"`
	CacheMethods          []string `json:"cache_methods" yaml:"cache_methods"`
	CacheBypassPaths      []string `json:"cache_bypass_paths" yaml:"cache_bypass_paths"`
	CacheBypassHeaders    []string `json:"cache_bypass_headers" yaml:"cache_bypass_headers"`
	RequestTimeout        *string  `json:"request_timeout" yaml:"request_timeout"`
	DialTimeout           *string  `json:"dial_timeout" yaml:"dial_timeout"`
	IdleConnTimeout       *string  `json:"idle_conn_timeout" yaml:"idle_conn_timeout"`
	ResponseHeaderTimeout *string  `json:"response_header_timeout" yaml:"response_header_timeout"`
	MaxIdleConns          *int     `json:"max_idle_conns" yaml:"max_idle_conns"`
	MaxIdleConnsPerHost   *int     `json:"max_idle_conns_per_host" yaml:"max_idle_conns_per_host"`
	MaxConnsPerHost       *int     `json:"max_conns_per_host" yaml:"max_conns_per_host"`
	RetryCount            *int     `json:"retry_count" yaml:"retry_count"`
	RetryBackoff          *string  `json:"retry_backoff" yaml:"retry_backoff"`
	LogLevel              *string  `json:"log_level" yaml:"log_level"`
	Debug                 *bool    `json:"debug" yaml:"debug"`
	MetricsPath           *string  `json:"metrics_path" yaml:"metrics_path"`
	AdminPrefix           *string  `json:"admin_prefix" yaml:"admin_prefix"`
	RateLimitRPS          *float64 `json:"rate_limit_rps" yaml:"rate_limit_rps"`
	RateLimitBurst        *int     `json:"rate_limit_burst" yaml:"rate_limit_burst"`
}

func loadJSONConfig(path string) (fileConfig, error) {
	bytes, err := readConfigFile(path)
	if err != nil {
		return fileConfig{}, err
	}

	var fc fileConfig
	if err := json.Unmarshal(bytes, &fc); err != nil {
		return fileConfig{}, fmt.Errorf("parse config file JSON: %w", err)
	}
	return fc, nil
}

func loadYAMLConfig(path string) (fileConfig, error) {
	bytes, err := readConfigFile(path)
	if err != nil {
		return fileConfig{}, err
	}

	var fc fileConfig
	if err := yaml.Unmarshal(bytes, &fc); err != nil {
		return fileConfig{}, fmt.Errorf("parse config file YAML: %w", err)
	}
	return fc, nil
}

func readConfigFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	bytes, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	return bytes, nil
}

func detectConfigFormat(path string) (configFormat, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	switch ext {
	case ".json":
		return configFormatJSON, nil
	case ".yaml", ".yml":
		return configFormatYAML, nil
	default:
		return "", fmt.Errorf("unsupported config file extension: %s", ext)
	}
}

func applyFileConfig(cfg *Config, fc fileConfig) {
	if fc.Port != nil {
		cfg.Port = *fc.Port
	}
	if fc.Origin != nil {
		cfg.OriginRaw = strings.TrimSpace(*fc.Origin)
	}
	applyDuration(&cfg.TTL, fc.TTL)
	applyDuration(&cfg.StaleWhileRevalidate, fc.StaleWhileRevalidate)
	applyDuration(&cfg.StaleIfError, fc.StaleIfError)
	if fc.CacheMaxEntries != nil {
		cfg.CacheMaxEntries = *fc.CacheMaxEntries
	}
	if fc.CacheMaxEntryBytes != nil {
		cfg.CacheMaxEntryBytes = *fc.CacheMaxEntryBytes
	}
	if len(fc.CacheMethods) > 0 {
		cfg.CacheMethods = copyStringSlice(fc.CacheMethods)
	}
	if len(fc.CacheBypassPaths) > 0 {
		cfg.CacheBypassPaths = copyStringSlice(fc.CacheBypassPaths)
	}
	if len(fc.CacheBypassHeaders) > 0 {
		cfg.CacheBypassHeaders = copyStringSlice(fc.CacheBypassHeaders)
	}
	applyDuration(&cfg.RequestTimeout, fc.RequestTimeout)
	applyDuration(&cfg.DialTimeout, fc.DialTimeout)
	applyDuration(&cfg.IdleConnTimeout, fc.IdleConnTimeout)
	applyDuration(&cfg.ResponseHeaderTimeout, fc.ResponseHeaderTimeout)
	if fc.MaxIdleConns != nil {
		cfg.MaxIdleConns = *fc.MaxIdleConns
	}
	if fc.MaxIdleConnsPerHost != nil {
		cfg.MaxIdleConnsPerHost = *fc.MaxIdleConnsPerHost
	}
	if fc.MaxConnsPerHost != nil {
		cfg.MaxConnsPerHost = *fc.MaxConnsPerHost
	}
	if fc.RetryCount != nil {
		cfg.RetryCount = *fc.RetryCount
	}
	applyDuration(&cfg.RetryBackoff, fc.RetryBackoff)
	if fc.LogLevel != nil {
		cfg.LogLevel = strings.ToLower(strings.TrimSpace(*fc.LogLevel))
	}
	if fc.Debug != nil {
		cfg.Debug = *fc.Debug
	}
	if fc.MetricsPath != nil {
		cfg.MetricsPath = strings.TrimSpace(*fc.MetricsPath)
	}
	if fc.AdminPrefix != nil {
		cfg.AdminPrefix = strings.TrimSpace(*fc.AdminPrefix)
	}
	if fc.RateLimitRPS != nil {
		cfg.RateLimitRPS = *fc.RateLimitRPS
	}
	if fc.RateLimitBurst != nil {
		cfg.RateLimitBurst = *fc.RateLimitBurst
	}
}

func applyEnvConfig(cfg *Config) {
	applyIntEnv("RELAY_PORT", &cfg.Port)
	applyStringEnv("RELAY_ORIGIN", &cfg.OriginRaw)
	applyDurationEnv("RELAY_TTL", &cfg.TTL)
	applyDurationEnv("RELAY_STALE_WHILE_REVALIDATE", &cfg.StaleWhileRevalidate)
	applyDurationEnv("RELAY_STALE_IF_ERROR", &cfg.StaleIfError)
	applyIntEnv("RELAY_CACHE_MAX_ENTRIES", &cfg.CacheMaxEntries)
	applyIntEnv("RELAY_CACHE_MAX_ENTRY_BYTES", &cfg.CacheMaxEntryBytes)
	applyCSVEnv("RELAY_CACHE_METHODS", &cfg.CacheMethods)
	applyCSVEnv("RELAY_CACHE_BYPASS_PATHS", &cfg.CacheBypassPaths)
	applyCSVEnv("RELAY_CACHE_BYPASS_HEADERS", &cfg.CacheBypassHeaders)
	applyDurationEnv("RELAY_REQUEST_TIMEOUT", &cfg.RequestTimeout)
	applyDurationEnv("RELAY_DIAL_TIMEOUT", &cfg.DialTimeout)
	applyDurationEnv("RELAY_IDLE_CONN_TIMEOUT", &cfg.IdleConnTimeout)
	applyDurationEnv("RELAY_RESPONSE_HEADER_TIMEOUT", &cfg.ResponseHeaderTimeout)
	applyIntEnv("RELAY_MAX_IDLE_CONNS", &cfg.MaxIdleConns)
	applyIntEnv("RELAY_MAX_IDLE_CONNS_PER_HOST", &cfg.MaxIdleConnsPerHost)
	applyIntEnv("RELAY_MAX_CONNS_PER_HOST", &cfg.MaxConnsPerHost)
	applyIntEnv("RELAY_RETRY_COUNT", &cfg.RetryCount)
	applyDurationEnv("RELAY_RETRY_BACKOFF", &cfg.RetryBackoff)
	applyStringEnv("RELAY_LOG_LEVEL", &cfg.LogLevel)
	applyBoolEnv("RELAY_DEBUG", &cfg.Debug)
	applyStringEnv("RELAY_METRICS_PATH", &cfg.MetricsPath)
	applyStringEnv("RELAY_ADMIN_PREFIX", &cfg.AdminPrefix)
	applyFloatEnv("RELAY_RATE_LIMIT_RPS", &cfg.RateLimitRPS)
	applyIntEnv("RELAY_RATE_LIMIT_BURST", &cfg.RateLimitBurst)
}

func parseFlags(cfg *Config, args []string) error {
	fs := flag.NewFlagSet("relay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	cacheMethods := strings.Join(cfg.CacheMethods, ",")
	bypassPaths := strings.Join(cfg.CacheBypassPaths, ",")
	bypassHeaders := strings.Join(cfg.CacheBypassHeaders, ",")

	fs.StringVar(&cfg.ConfigFile, "config", cfg.ConfigFile, "path to JSON config file")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "port to listen on")
	fs.StringVar(&cfg.OriginRaw, "origin", cfg.OriginRaw, "origin base URL")
	fs.BoolVar(&cfg.ClearCache, "clear-cache", cfg.ClearCache, "clear cache and exit")
	fs.BoolVar(&cfg.CacheStats, "cache-stats", cfg.CacheStats, "print cache stats and exit")
	fs.DurationVar(&cfg.TTL, "ttl", cfg.TTL, "default cache TTL")
	fs.DurationVar(&cfg.StaleWhileRevalidate, "stale-while-revalidate", cfg.StaleWhileRevalidate, "stale while revalidate duration")
	fs.DurationVar(&cfg.StaleIfError, "stale-if-error", cfg.StaleIfError, "stale if error duration")
	fs.IntVar(&cfg.CacheMaxEntries, "cache-max-entries", cfg.CacheMaxEntries, "max cache entries")
	fs.IntVar(&cfg.CacheMaxEntryBytes, "cache-max-entry-bytes", cfg.CacheMaxEntryBytes, "max cacheable body bytes")
	fs.StringVar(&cacheMethods, "cache-methods", cacheMethods, "comma-separated cache methods")
	fs.StringVar(&bypassPaths, "cache-bypass-paths", bypassPaths, "comma-separated path prefixes to bypass cache")
	fs.StringVar(&bypassHeaders, "cache-bypass-headers", bypassHeaders, "comma-separated headers to bypass cache")
	fs.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "upstream request timeout")
	fs.DurationVar(&cfg.DialTimeout, "dial-timeout", cfg.DialTimeout, "upstream dial timeout")
	fs.DurationVar(&cfg.IdleConnTimeout, "idle-conn-timeout", cfg.IdleConnTimeout, "upstream idle connection timeout")
	fs.DurationVar(&cfg.ResponseHeaderTimeout, "response-header-timeout", cfg.ResponseHeaderTimeout, "upstream response header timeout")
	fs.IntVar(&cfg.MaxIdleConns, "max-idle-conns", cfg.MaxIdleConns, "max idle conns")
	fs.IntVar(&cfg.MaxIdleConnsPerHost, "max-idle-conns-per-host", cfg.MaxIdleConnsPerHost, "max idle conns per host")
	fs.IntVar(&cfg.MaxConnsPerHost, "max-conns-per-host", cfg.MaxConnsPerHost, "max conns per host")
	fs.IntVar(&cfg.RetryCount, "retry-count", cfg.RetryCount, "retry count for upstream requests")
	fs.DurationVar(&cfg.RetryBackoff, "retry-backoff", cfg.RetryBackoff, "base retry backoff duration")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug|info|warn|error")
	fs.BoolVar(&cfg.Debug, "debug", cfg.Debug, "enable debug mode")
	fs.StringVar(&cfg.MetricsPath, "metrics-path", cfg.MetricsPath, "metrics endpoint path")
	fs.StringVar(&cfg.AdminPrefix, "admin-prefix", cfg.AdminPrefix, "admin endpoint prefix")
	fs.Float64Var(&cfg.RateLimitRPS, "rate-limit-rps", cfg.RateLimitRPS, "rate limit requests per second per client")
	fs.IntVar(&cfg.RateLimitBurst, "rate-limit-burst", cfg.RateLimitBurst, "rate limit burst per client")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg.CacheMethods = splitCSV(cacheMethods)
	cfg.CacheBypassPaths = splitCSV(bypassPaths)
	cfg.CacheBypassHeaders = splitCSV(bypassHeaders)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))

	return nil
}

func findConfigPath(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if strings.HasPrefix(args[i], "--config=") {
			return strings.TrimSpace(strings.TrimPrefix(args[i], "--config="))
		}
	}
	return ""
}

func hasHelpArg(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if item := strings.TrimSpace(p); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func copyStringSlice(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func normalizeMethods(methods []string) []string {
	seen := make(map[string]struct{}, len(methods))
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		m := strings.ToUpper(strings.TrimSpace(method))
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

func normalizeHeaders(headers []string) []string {
	seen := make(map[string]struct{}, len(headers))
	out := make([]string, 0, len(headers))
	for _, header := range headers {
		h := httpCanonicalHeaderKey(strings.TrimSpace(header))
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func httpCanonicalHeaderKey(h string) string {
	if h == "" {
		return ""
	}
	segments := strings.Split(strings.ToLower(h), "-")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		segments[i] = strings.ToUpper(seg[:1]) + seg[1:]
	}
	return strings.Join(segments, "-")
}

func applyDuration(dst *time.Duration, v *string) {
	if v == nil {
		return
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(*v))
	if err == nil {
		*dst = parsed
	}
}

func applyIntEnv(name string, dst *int) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		*dst = parsed
	}
}

func applyFloatEnv(name string, dst *float64) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err == nil {
		*dst = parsed
	}
}

func applyDurationEnv(name string, dst *time.Duration) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err == nil {
		*dst = parsed
	}
}

func applyStringEnv(name string, dst *string) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		*dst = trimmed
	}
}

func applyBoolEnv(name string, dst *bool) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err == nil {
		*dst = parsed
	}
}

func applyCSVEnv(name string, dst *[]string) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	*dst = splitCSV(value)
}
