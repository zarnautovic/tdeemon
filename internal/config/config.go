// Package config loads and validates the server configuration from the
// environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata" // TZ must resolve even in images without system tzdata.
)

// Prefix is prepended to the name of every variable Load reads, except TZ.
const Prefix = "TDEEMON_"

// MinSecretKeyLen is the minimum length of the secret key in bytes.
const MinSecretKeyLen = 32

// LogFormat selects the log encoding.
type LogFormat string

// Supported log formats.
const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// LookupFunc reports the value of an environment variable and whether it
// is set. os.LookupEnv satisfies it.
type LookupFunc func(key string) (string, bool)

// Config is the validated server configuration.
type Config struct {
	// Listen is the TCP address the HTTP server binds, as host:port.
	Listen string
	// BaseURL is the public URL of the instance, without a trailing slash.
	BaseURL *url.URL
	// SecretKey signs sessions and encrypts stored secrets. Never log it.
	SecretKey []byte
	// DataDir holds the database, backups and upload archives.
	DataDir string
	// LogLevel is the minimum level that gets logged.
	LogLevel slog.Level
	// LogFormat is the log encoding.
	LogFormat LogFormat
	// Location is the instance's default time zone, from TZ.
	Location *time.Location
}

// Load reads the configuration through lookup and validates it. It
// reports every invalid or missing variable at once, joined into one
// error, so a misconfigured instance can be fixed in a single pass.
func Load(lookup LookupFunc) (Config, error) {
	get := func(name, fallback string) string {
		if v, ok := lookup(Prefix + name); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return fallback
	}

	var errs []error
	cfg := Config{
		Listen:    get("LISTEN", ":8080"),
		DataDir:   get("DATA_DIR", "./data"),
		LogFormat: LogFormat(strings.ToLower(get("LOG_FORMAT", string(LogFormatJSON)))),
		Location:  time.Local,
	}

	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		errs = append(errs, fmt.Errorf("%sLISTEN %q: want host:port, e.g. :8080: %w", Prefix, cfg.Listen, err))
	}

	if raw := get("BASE_URL", ""); raw == "" {
		errs = append(errs, fmt.Errorf("%sBASE_URL is required: the public URL of this instance, e.g. https://tdeemon.example.com", Prefix))
	} else if u, err := parseBaseURL(raw); err != nil {
		errs = append(errs, fmt.Errorf("%sBASE_URL %w", Prefix, err))
	} else {
		cfg.BaseURL = u
	}

	cfg.SecretKey = []byte(get("SECRET_KEY", ""))
	switch {
	case len(cfg.SecretKey) == 0:
		errs = append(errs, fmt.Errorf("%sSECRET_KEY is required; generate one with: openssl rand -base64 32", Prefix))
	case len(cfg.SecretKey) < MinSecretKeyLen:
		errs = append(errs, fmt.Errorf("%sSECRET_KEY must be at least %d bytes, got %d", Prefix, MinSecretKeyLen, len(cfg.SecretKey)))
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("%sLOG_LEVEL: want debug, info, warn or error: %w", Prefix, err))
	}

	if cfg.LogFormat != LogFormatJSON && cfg.LogFormat != LogFormatText {
		errs = append(errs, fmt.Errorf("%sLOG_FORMAT %q: want json or text", Prefix, cfg.LogFormat))
	}

	if tz, ok := lookup("TZ"); ok && strings.TrimSpace(tz) != "" {
		loc, err := time.LoadLocation(strings.TrimSpace(tz))
		if err != nil {
			errs = append(errs, fmt.Errorf("TZ: %w", err))
		} else {
			cfg.Location = loc
		}
	}

	return cfg, errors.Join(errs...)
}

// parseBaseURL accepts an absolute http(s) URL with a host and no query
// or fragment, and drops a trailing slash from its path.
func parseBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%q: host is missing", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%q: must not contain a query or fragment", raw)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

// LogValue implements slog.LogValuer, so the configuration can be logged
// at startup without leaking the secret key.
func (c Config) LogValue() slog.Value {
	var baseURL string
	if c.BaseURL != nil {
		baseURL = c.BaseURL.String()
	}
	var tz string
	if c.Location != nil {
		tz = c.Location.String()
	}
	return slog.GroupValue(
		slog.String("listen", c.Listen),
		slog.String("base_url", baseURL),
		slog.String("data_dir", c.DataDir),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("log_format", string(c.LogFormat)),
		slog.String("tz", tz),
		slog.String("secret_key", "[redacted]"),
	)
}
