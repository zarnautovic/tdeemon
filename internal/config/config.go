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
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // TZ must resolve even in images without system tzdata.

	"github.com/zarnautovic/tdeemon/internal/domain/limits"
)

// Prefix is prepended to the name of every variable Load reads, except TZ.
const Prefix = "TDEEMON_"

// MaxBackupKeep is the most daily snapshots TDEEMON_BACKUP_KEEP keeps: a
// year, each a copy of the whole database.
const MaxBackupKeep = 366

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
	// BackupKeep is how many daily snapshots of the database are kept in
	// DataDir/backups; 0 takes none.
	BackupKeep int
	// TelegramBotToken is the token of the bot that sends the morning
	// summary and the watchdog's alarms; empty for none. Never log it.
	TelegramBotToken string
	// TelegramAPIURL is the Bot API's base URL: Telegram's, or a Bot API
	// server of one's own.
	TelegramAPIURL string
	// OpenFoodFactsURL is the Open Food Facts server a barcode none of a
	// user's foods has is looked up at; empty for none, so no request
	// leaves the instance for it.
	OpenFoodFactsURL string
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
	// Limits are the instance's limits of a plan, the outer bound of every
	// user's (docs/limits.md); limits.Instance() unless TDEEMON_LIMIT_*
	// variables change them.
	Limits limits.Limits
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

	// Each loader sets its part of cfg; errors.Join drops the nil ones.
	for _, load := range []func() error{
		func() (err error) { cfg.OIDC, err = loadOIDC(get); return err },
		func() (err error) {
			cfg.TrustedProxies, err = parseTrustedProxies(get("TRUSTED_PROXIES", ""))
			return err
		},
		func() (err error) { cfg.Location, err = loadLocation(lookup); return err },
		func() (err error) { cfg.Limits, err = loadLimits(get); return err },
		func() (err error) { cfg.BackupKeep, err = loadBackupKeep(get); return err },
		func() (err error) { cfg.TelegramBotToken, err = loadTelegramBotToken(get); return err },
		func() (err error) { cfg.TelegramAPIURL, err = loadTelegramAPIURL(get); return err },
		func() (err error) { cfg.OpenFoodFactsURL, err = loadOpenFoodFactsURL(get); return err },
	} {
		errs = append(errs, load())
	}

	return cfg, errors.Join(errs...)
}

// telegramBotToken is the shape of a bot's token from BotFather: the
// bot's id, a colon and a secret.
var telegramBotToken = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{30,}$`)

// loadTelegramBotToken reads the Telegram bot's token, which may be unset.
func loadTelegramBotToken(get func(name, fallback string) string) (string, error) {
	t := get("TELEGRAM_BOT_TOKEN", "")
	if t != "" && !telegramBotToken.MatchString(t) {
		// The value stays out of the error: it is a secret, if a wrong one.
		return "", fmt.Errorf("%sTELEGRAM_BOT_TOKEN: want a bot token from BotFather, <bot id>:<secret>", Prefix)
	}
	return t, nil
}

// loadTelegramAPIURL reads the Bot API's base URL, Telegram's unless set.
func loadTelegramAPIURL(get func(name, fallback string) string) (string, error) {
	raw := get("TELEGRAM_API_URL", "https://api.telegram.org")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%sTELEGRAM_API_URL %q: want an http or https URL of the Bot API", Prefix, raw)
	}
	return strings.TrimSuffix(raw, "/"), nil
}

// loadOpenFoodFactsURL reads the Open Food Facts server to look barcodes
// up at, the world's unless set, or none for off.
func loadOpenFoodFactsURL(get func(name, fallback string) string) (string, error) {
	raw := get("OPEN_FOOD_FACTS_URL", "https://world.openfoodfacts.org")
	if raw == "off" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%sOPEN_FOOD_FACTS_URL %q: want an http or https URL of an Open Food Facts server, or off", Prefix, raw)
	}
	return strings.TrimSuffix(raw, "/"), nil
}

// loadBackupKeep reads how many daily snapshots to keep, 7 unless set.
func loadBackupKeep(get func(name, fallback string) string) (int, error) {
	raw := get("BACKUP_KEEP", "7")
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > MaxBackupKeep {
		return 0, fmt.Errorf("%sBACKUP_KEEP %q: want a number of daily snapshots from 0 (none) to %d", Prefix, raw, MaxBackupKeep)
	}
	return n, nil
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

// LimitPrefix comes before a limit's name, in upper case, in the name of
// the variable that sets it: TDEEMON_LIMIT_KCAL_FLOOR_FEMALE.
const LimitPrefix = "LIMIT_"

// loadLimits reads the instance's limits: the defaults, each changed by
// its variable, such as TDEEMON_LIMIT_KCAL_FLOOR_FEMALE=1000 for a
// clinician's patient. The admin may set them within the floor in the
// code, and tighten the limits whose default is no looser than the
// user's (limits.ValidateInstance).
func loadLimits(get func(name, fallback string) string) (limits.Limits, error) {
	l := limits.Instance()
	var errs []error
	for _, f := range limits.Fields() {
		name := LimitPrefix + strings.ToUpper(f.Name)
		raw := get(name, "")
		switch {
		case raw == "":
			continue
		case f.UserOnly:
			errs = append(errs, fmt.Errorf("%s%s: a user's limit, which only the user sets", Prefix, name))
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s%s %q: want a number", Prefix, name, raw))
			continue
		}
		f.Set(&l, v)
	}
	// The values that read are checked too, so every error shows at once.
	if err := limits.ValidateInstance(l); err != nil {
		errs = append(errs, fmt.Errorf("%s%s*: %w", Prefix, LimitPrefix, err))
	}
	if err := errors.Join(errs...); err != nil {
		return limits.Instance(), err
	}
	return l, nil
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
		slog.Int("backup_keep", c.BackupKeep),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("log_format", string(c.LogFormat)),
		slog.String("tz", tz),
		slog.String("secret_key", "[redacted]"),
		slog.String("telegram_bot_token", redactedIfSet(c.TelegramBotToken)),
		slog.String("telegram_api_url", c.TelegramAPIURL),
		slog.String("open_food_facts_url", c.OpenFoodFactsURL),
		slog.Any("oidc", c.OIDC),
		slog.Any("trusted_proxies", c.TrustedProxies),
		slog.Any("limits", limitsValue(c.Limits)),
	)
}

// redactedIfSet says whether a secret is set, without it.
func redactedIfSet(s string) string {
	if s == "" {
		return ""
	}
	return "[redacted]"
}

// limitsValue logs the instance's limits by their names.
func limitsValue(l limits.Limits) slog.Value {
	var attrs []slog.Attr
	for _, f := range limits.Fields() {
		if !f.UserOnly {
			attrs = append(attrs, slog.Float64(f.Name, f.Get(l)))
		}
	}
	return slog.GroupValue(attrs...)
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
