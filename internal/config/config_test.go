package config_test

import (
	"bytes"
	"log/slog"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/config"
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
	vars["TDEEMON_BASE_URL"] = "http://localhost:9000/tdeemon/"
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
	if got := cfg.BaseURL.String(); got != "http://localhost:9000/tdeemon" {
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
