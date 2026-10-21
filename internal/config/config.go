// Package config loads and validates the server configuration from the
// environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
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
	// SecretKey seals the single sign-on state cookie, and will encrypt
	// stored secrets; sessions do not depend on it. Never log it.
	SecretKey []byte
	// DataDir holds the database, backups and upload archives.
	DataDir string
	// LogLevel is the minimum level that gets logged.
	LogLevel slog.Level
	// LogFormat is the log encoding.
	LogFormat LogFormat
	// Location is the instance's default time zone, from TZ.
	Location *time.Location
	// OIDC is the OpenID Connect provider users may sign in with; nil when
	// none is configured.
	OIDC *OIDC
	// TrustedProxies are the reverse proxies whose X-Forwarded-For names
	// the client; a request from anywhere else is its own client.
	TrustedProxies []netip.Prefix
}

// OIDC configures sign-in through an OpenID Connect provider such as
// Authentik.
type OIDC struct {
	// Issuer is the provider's issuer URL; its discovery document lives at
	// Issuer/.well-known/openid-configuration.
	Issuer string
	// ClientID and ClientSecret are tdeemon's credentials at the provider.
	// The secret may be empty for a public client; PKCE protects the code
	// either way. Never log the secret.
	ClientID     string
	ClientSecret string
	// Name is what the sign-in page calls the provider.
	Name string
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

	oidc, err := loadOIDC(get)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.OIDC = oidc

	if cfg.TrustedProxies, err = parseTrustedProxies(get("TRUSTED_PROXIES", "")); err != nil {
		errs = append(errs, err)
	}

	if cfg.Location, err = loadLocation(lookup); err != nil {
		errs = append(errs, err)
	}

	return cfg, errors.Join(errs...)
}

// loadLocation reads TZ, the instance's default time zone; time.Local
// without it.
func loadLocation(lookup LookupFunc) (*time.Location, error) {
	tz, ok := lookup("TZ")
	if !ok || strings.TrimSpace(tz) == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil {
		return time.Local, fmt.Errorf("TZ: %w", err)
	}
	return loc, nil
}

// loadOIDC reads the OIDC_* variables: none of them for no provider, or
// at least the issuer and the client ID.
func loadOIDC(get func(name, fallback string) string) (*OIDC, error) {
	o := &OIDC{
		Issuer:       get("OIDC_ISSUER", ""),
		ClientID:     get("OIDC_CLIENT_ID", ""),
		ClientSecret: get("OIDC_CLIENT_SECRET", ""),
		Name:         get("OIDC_NAME", "Single sign-on"),
	}
	switch {
	case o.Issuer == "" && o.ClientID == "" && o.ClientSecret == "":
		return nil, nil //nolint:nilnil // No provider is a valid configuration, not an error.
	case o.Issuer == "" || o.ClientID == "":
		return nil, fmt.Errorf("%sOIDC_ISSUER and %sOIDC_CLIENT_ID go together; set both for single sign-on, or neither", Prefix, Prefix)
	}
	u, err := url.Parse(o.Issuer)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%sOIDC_ISSUER %q: want the provider's issuer URL, e.g. https://auth.example.com/application/o/tdeemon/", Prefix, o.Issuer)
	}
	// Plain HTTP would let anyone on the path forge identities; only a
	// provider on this machine may use it.
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopback(u.Hostname())) {
		return nil, fmt.Errorf("%sOIDC_ISSUER %q: must use https (http only for localhost)", Prefix, o.Issuer)
	}
	return o, nil
}

// parseTrustedProxies reads addresses and CIDR ranges, separated by
// commas or spaces: 10.0.0.0/8, 192.168.1.10, fd00::/8.
func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for f := range strings.FieldsFuncSeq(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
		p, err := netip.ParsePrefix(f)
		if err != nil {
			a, aerr := netip.ParseAddr(f)
			if aerr != nil || a.Zone() != "" {
				return nil, fmt.Errorf("%sTRUSTED_PROXIES %q: want addresses or CIDR ranges, e.g. 10.42.0.0/16", Prefix, f)
			}
			p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
		}
		// Peers come unmapped, so an IPv4-mapped range must be an IPv4
		// one to ever match.
		if a := p.Addr(); a.Is4In6() && p.Bits() >= 96 {
			p = netip.PrefixFrom(a.Unmap(), p.Bits()-96)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// parseBaseURL accepts an absolute http(s) URL with a host and no path,
// query or fragment, and drops a trailing slash.
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
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("%q: must not contain a query or fragment", raw)
	}
	// The web app and the API are served from the root of the host; the
	// app's assets and API calls would miss a path prefix.
	if strings.Trim(u.Path, "/") != "" {
		return nil, fmt.Errorf("%q: must not have a path; tdeemon is served at the root of its host", raw)
	}
	u.Path = ""
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
		slog.Any("oidc", c.OIDC),
		slog.Any("trusted_proxies", c.TrustedProxies),
	)
}

// LogValue implements slog.LogValuer without the client secret.
func (o *OIDC) LogValue() slog.Value {
	if o == nil {
		return slog.StringValue("off")
	}
	secret := ""
	if o.ClientSecret != "" {
		secret = "[redacted]"
	}
	return slog.GroupValue(
		slog.String("issuer", o.Issuer),
		slog.String("client_id", o.ClientID),
		slog.String("client_secret", secret),
		slog.String("name", o.Name),
	)
}
