package config_test

import (
	"bytes"
	"log/slog"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/config"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
)

// env returns a LookupFunc backed by m instead of the process environment,
// so tests can run in parallel.
func env(m map[string]string) config.LookupFunc {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func required() map[string]string {
	return map[string]string{
		"TDEEMON_BASE_URL":   "https://tdeemon.example.com/",
		"TDEEMON_SECRET_KEY": strings.Repeat("s", config.MinSecretKeyLen),
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(env(required()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Listen != ":8080" {
		t.Errorf("Listen = %q, want :8080", cfg.Listen)
	}
	if got := cfg.BaseURL.String(); got != "https://tdeemon.example.com" {
		t.Errorf("BaseURL = %q, want the URL without the trailing slash", got)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.LogFormat != config.LogFormatJSON {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
	if cfg.Location != time.Local {
		t.Errorf("Location = %v, want time.Local", cfg.Location)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Parallel()

	vars := required()
	vars["TDEEMON_LISTEN"] = "127.0.0.1:9000"
	vars["TDEEMON_BASE_URL"] = "http://localhost:9000/"
	vars["TDEEMON_DATA_DIR"] = " /var/lib/tdeemon "
	vars["TDEEMON_LOG_LEVEL"] = "DEBUG"
	vars["TDEEMON_LOG_FORMAT"] = "Text"
	vars["TZ"] = "Europe/Zagreb"

	cfg, err := config.Load(env(vars))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Listen != "127.0.0.1:9000" {
		t.Errorf("Listen = %q", cfg.Listen)
	}
	if got := cfg.BaseURL.String(); got != "http://localhost:9000" {
		t.Errorf("BaseURL = %q", got)
	}
	if cfg.DataDir != "/var/lib/tdeemon" {
		t.Errorf("DataDir = %q, want surrounding spaces trimmed", cfg.DataDir)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", cfg.LogLevel)
	}
	if cfg.LogFormat != config.LogFormatText {
		t.Errorf("LogFormat = %q, want text", cfg.LogFormat)
	}
	if got := cfg.Location.String(); got != "Europe/Zagreb" {
		t.Errorf("Location = %q, want Europe/Zagreb", got)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		set  map[string]string // applied on top of the required variables
		drop []string          // removed from the required variables
		want []string          // substrings the error must contain
	}{
		{
			name: "required variables missing are all reported",
			drop: []string{"TDEEMON_BASE_URL", "TDEEMON_SECRET_KEY"},
			want: []string{"TDEEMON_BASE_URL is required", "TDEEMON_SECRET_KEY is required"},
		},
		{
			name: "blank value counts as missing",
			set:  map[string]string{"TDEEMON_SECRET_KEY": "   "},
			want: []string{"TDEEMON_SECRET_KEY is required"},
		},
		{
			name: "listen without a port",
			set:  map[string]string{"TDEEMON_LISTEN": "8080"},
			want: []string{"TDEEMON_LISTEN"},
		},
		{
			name: "base URL with another scheme",
			set:  map[string]string{"TDEEMON_BASE_URL": "ftp://tdeemon.example.com"},
			want: []string{"TDEEMON_BASE_URL", "http or https"},
		},
		{
			name: "base URL without a host",
			set:  map[string]string{"TDEEMON_BASE_URL": "https://"},
			want: []string{"TDEEMON_BASE_URL", "host is missing"},
		},
		{
			name: "base URL with a query",
			set:  map[string]string{"TDEEMON_BASE_URL": "https://tdeemon.example.com/?a=1"},
			want: []string{"TDEEMON_BASE_URL", "query or fragment"},
		},
		{
			name: "base url with an empty query",
			set:  map[string]string{"TDEEMON_BASE_URL": "https://tdeemon.example.com?"},
			want: []string{"TDEEMON_BASE_URL", "query or fragment"},
		},
		{
			name: "base url with a path",
			set:  map[string]string{"TDEEMON_BASE_URL": "https://example.com/tdeemon/"},
			want: []string{"TDEEMON_BASE_URL", "must not have a path"},
		},
		{
			name: "short secret key",
			set:  map[string]string{"TDEEMON_SECRET_KEY": "too-short"},
			want: []string{"TDEEMON_SECRET_KEY must be at least 32 bytes"},
		},
		{
			name: "unknown log level",
			set:  map[string]string{"TDEEMON_LOG_LEVEL": "verbose"},
			want: []string{"TDEEMON_LOG_LEVEL"},
		},
		{
			name: "unknown log format",
			set:  map[string]string{"TDEEMON_LOG_FORMAT": "xml"},
			want: []string{"TDEEMON_LOG_FORMAT"},
		},
		{
			name: "unknown time zone",
			set:  map[string]string{"TZ": "Mars/Olympus_Mons"},
			want: []string{"TZ:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vars := required()
			for _, k := range tt.drop {
				delete(vars, k)
			}
			maps.Copy(vars, tt.set)

			_, err := config.Load(env(vars))
			if err == nil {
				t.Fatal("Load succeeded, want an error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestConfigLogValueRedactsSecret(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(env(required()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("starting", "config", cfg)

	out := buf.String()
	if strings.Contains(out, string(cfg.SecretKey)) {
		t.Errorf("log output leaks the secret key: %s", out)
	}
	if !strings.Contains(out, "config.secret_key=[redacted]") {
		t.Errorf("log output does not mark the secret key as redacted: %s", out)
	}
	if !strings.Contains(out, "config.base_url=https://tdeemon.example.com") {
		t.Errorf("log output is missing the base URL: %s", out)
	}
}

func TestLoadOIDC(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(env(required()))
	if err != nil || cfg.OIDC != nil {
		t.Fatalf("without OIDC_*: OIDC = %+v, err %v; want none", cfg.OIDC, err)
	}

	for _, tc := range []struct {
		name   string
		vars   map[string]string
		issuer string
		secret string
		label  string
	}{
		{"confidential client", map[string]string{
			"TDEEMON_OIDC_ISSUER": " https://auth.example.com/application/o/tdeemon/ ", "TDEEMON_OIDC_CLIENT_ID": "tdeemon",
			"TDEEMON_OIDC_CLIENT_SECRET": "s3cret", "TDEEMON_OIDC_NAME": "Authentik",
		}, "https://auth.example.com/application/o/tdeemon/", "s3cret", "Authentik"},
		{"public client, default name", map[string]string{
			"TDEEMON_OIDC_ISSUER": "https://auth.example.com", "TDEEMON_OIDC_CLIENT_ID": "tdeemon",
		}, "https://auth.example.com", "", "Single sign-on"},
		{"plain HTTP on localhost", map[string]string{
			"TDEEMON_OIDC_ISSUER": "http://127.0.0.1:9000/", "TDEEMON_OIDC_CLIENT_ID": "tdeemon",
		}, "http://127.0.0.1:9000/", "", "Single sign-on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vars := required()
			maps.Copy(vars, tc.vars)
			cfg, err := config.Load(env(vars))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			o := cfg.OIDC
			if o == nil || o.Issuer != tc.issuer || o.ClientID != "tdeemon" || o.ClientSecret != tc.secret || o.Name != tc.label {
				t.Errorf("OIDC = %+v", o)
			}
		})
	}
}

func TestLoadOIDCErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"issuer without client", map[string]string{"TDEEMON_OIDC_ISSUER": "https://auth.example.com"}, "go together"},
		{"client without issuer", map[string]string{"TDEEMON_OIDC_CLIENT_ID": "tdeemon"}, "go together"},
		{"only a secret", map[string]string{"TDEEMON_OIDC_CLIENT_SECRET": "s3cret"}, "go together"},
		{"not a URL", map[string]string{"TDEEMON_OIDC_ISSUER": "auth.example.com", "TDEEMON_OIDC_CLIENT_ID": "tdeemon"}, "issuer URL"},
		{"query", map[string]string{"TDEEMON_OIDC_ISSUER": "https://auth.example.com/?x=1", "TDEEMON_OIDC_CLIENT_ID": "tdeemon"}, "issuer URL"},
		{"plain HTTP", map[string]string{"TDEEMON_OIDC_ISSUER": "http://auth.example.com", "TDEEMON_OIDC_CLIENT_ID": "tdeemon"}, "must use https"},
		{"other scheme", map[string]string{"TDEEMON_OIDC_ISSUER": "ftp://localhost", "TDEEMON_OIDC_CLIENT_ID": "tdeemon"}, "must use https"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vars := required()
			maps.Copy(vars, tc.vars)
			_, err := config.Load(env(vars))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestOIDCLogValueRedactsSecret(t *testing.T) {
	t.Parallel()

	vars := required()
	maps.Copy(vars, map[string]string{
		"TDEEMON_OIDC_ISSUER": "https://auth.example.com", "TDEEMON_OIDC_CLIENT_ID": "tdeemon", "TDEEMON_OIDC_CLIENT_SECRET": "s3cret-value",
	})
	cfg, err := config.Load(env(vars))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("config", "config", cfg)
	if strings.Contains(buf.String(), "s3cret-value") || !strings.Contains(buf.String(), "config.oidc.client_secret=[redacted]") ||
		!strings.Contains(buf.String(), "config.oidc.issuer=https://auth.example.com") {
		t.Errorf("logged %s, want the issuer and the secret redacted", buf.String())
	}

	buf.Reset()
	cfg, _ = config.Load(env(required()))
	slog.New(slog.NewTextHandler(&buf, nil)).Info("config", "config", cfg)
	if !strings.Contains(buf.String(), "config.oidc=off") {
		t.Errorf("logged %s, want oidc=off", buf.String())
	}
}

func TestLoadTrustedProxies(t *testing.T) {
	t.Parallel()

	vars := required()
	vars["TDEEMON_TRUSTED_PROXIES"] = "10.42.0.0/16, 192.168.1.10 fd00::/8,::ffff:10.0.0.1 ::ffff:172.16.0.0/108"
	cfg, err := config.Load(env(vars))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("10.42.0.0/16"), netip.MustParsePrefix("192.168.1.10/32"),
		netip.MustParsePrefix("fd00::/8"), netip.MustParsePrefix("10.0.0.1/32"), netip.MustParsePrefix("172.16.0.0/12"),
	}
	if !slices.Equal(cfg.TrustedProxies, want) {
		t.Errorf("TrustedProxies = %v, want %v", cfg.TrustedProxies, want)
	}

	if cfg, err := config.Load(env(required())); err != nil || cfg.TrustedProxies != nil {
		t.Errorf("without the variable: %v, %v; want none", cfg.TrustedProxies, err)
	}
	for _, bad := range []string{"10.0.0.0/33", "proxy.local", "fe80::1%eth0"} {
		t.Run(bad, func(t *testing.T) {
			t.Parallel()
			vars := required()
			vars["TDEEMON_TRUSTED_PROXIES"] = bad
			if _, err := config.Load(env(vars)); err == nil || !strings.Contains(err.Error(), "TDEEMON_TRUSTED_PROXIES") {
				t.Errorf("Load = %v, want an error naming TDEEMON_TRUSTED_PROXIES", err)
			}
		})
	}
}

func TestLoadLimits(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(env(required()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Limits != limits.Instance() {
		t.Errorf("Limits = %+v, want the defaults", cfg.Limits)
	}

	m := required()
	maps.Copy(m, map[string]string{
		"TDEEMON_LIMIT_KCAL_FLOOR_FEMALE": "1000", // a clinician's patient
		"TDEEMON_LIMIT_CUT_RATE":          " -0.8 ",
		"TDEEMON_LIMIT_INTERVAL_DAYS":     "10",
		// A user's limit, which no variable sets.
		"TDEEMON_LIMIT_PROTEIN_FLOOR_GRAMS": "200",
	})
	cfg, err = config.Load(env(m))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := limits.Instance()
	want.KcalFloorFemale, want.CutRate, want.IntervalDays = 1000, -0.8, 10
	if cfg.Limits != want {
		t.Errorf("Limits = %+v, want %+v", cfg.Limits, want)
	}
}

func TestLoadLimitsErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, key, value, want string
	}{
		{"not a number", "TDEEMON_LIMIT_KCAL_FLOOR_MALE", "1200 kcal", `TDEEMON_LIMIT_KCAL_FLOOR_MALE "1200 kcal": want a number`},
		{"the code's floor", "TDEEMON_LIMIT_KCAL_FLOOR_MALE", "800", "TDEEMON_LIMIT_*: kcal_floor_male 800: want more than 800"},
		{"protein under the RDA", "TDEEMON_LIMIT_PROTEIN_FLOOR", "0.5", "protein_floor 0.5 g/kg"},
		{"a limit the instance only tightens", "TDEEMON_LIMIT_PROTEIN_CEILING", "4", "protein_ceiling 4: the instance may only tighten it"},
		{"a part of a day", "TDEEMON_LIMIT_FIRST_STEP_DAYS", "14.5", "first_step_days 14.5"},
		{"no number", "TDEEMON_LIMIT_DRIFT_PCT", "NaN", "drift_pct NaN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := required()
			m[tc.key] = tc.value
			cfg, err := config.Load(env(m))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load = %v, want an error with %q", err, tc.want)
			}
			if cfg.Limits != limits.Instance() {
				t.Errorf("Limits = %+v, want the defaults with an error", cfg.Limits)
			}
		})
	}
}

func TestConfigLogValueHasLimits(t *testing.T) {
	t.Parallel()

	m := required()
	m["TDEEMON_LIMIT_KCAL_FLOOR_FEMALE"] = "1100"
	cfg, err := config.Load(env(m))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("starting", "config", cfg)
	for _, want := range []string{"config.limits.kcal_floor_female=1100", "config.limits.cut_rate=-1"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log output is missing %s: %s", want, buf.String())
		}
	}
	if strings.Contains(buf.String(), "protein_floor_grams") {
		t.Errorf("log output has a user's limit: %s", buf.String())
	}
}
