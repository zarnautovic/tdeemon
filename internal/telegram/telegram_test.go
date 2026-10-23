package telegram_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/zarnautovic/tdeemon/internal/telegram"
)

const token = "123456:made-up-token-for-tests-only"

// sent is the last request a fake API took.
type sent struct {
	path string
	body map[string]any
}

// fakeAPI answers sendMessage as Telegram does, with answer, and keeps
// the last request.
func fakeAPI(t *testing.T, status int, answer string) (*httptest.Server, *sent) {
	t.Helper()
	last := &sent{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last.path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		last.body = nil
		_ = json.Unmarshal(b, &last.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return srv, last
}

// Send posts the text to the bot's sendMessage as plain text, without
// link previews.
func TestSend(t *testing.T) {
	t.Parallel()

	srv, last := fakeAPI(t, http.StatusOK, `{"ok":true,"result":{"message_id":1}}`)
	bot := telegram.Bot{Token: token, BaseURL: srv.URL + "/"}
	if err := bot.Send(t.Context(), 853, "Dobro jutro"); err != nil {
		t.Fatal(err)
	}
	if last.path != "/bot"+token+"/sendMessage" {
		t.Errorf("path = %q", last.path)
	}
	b := last.body
	if b["chat_id"] != float64(853) || b["text"] != "Dobro jutro" || b["parse_mode"] != nil {
		t.Errorf("body = %v", b)
	}
	if lp, _ := b["link_preview_options"].(map[string]any); lp["is_disabled"] != true {
		t.Errorf("link previews = %v", b["link_preview_options"])
	}
}

// A refusal is an *Error with Telegram's code, description and wait; no
// error carries the token.
func TestSendRefused(t *testing.T) {
	t.Parallel()

	srv, _ := fakeAPI(t, http.StatusTooManyRequests,
		`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 7","parameters":{"retry_after":7}}`)
	err := (&telegram.Bot{Token: token, BaseURL: srv.URL}).Send(t.Context(), 853, "x")
	var te *telegram.Error
	if !errors.As(err, &te) || te.Code != 429 || te.RetryAfter != 7*time.Second || !strings.Contains(te.Error(), "Too Many Requests") {
		t.Errorf("err = %v", err)
	}

	// A server that is not there: the transport's error, without the URL.
	srv.Close()
	err = (&telegram.Bot{Token: token, BaseURL: srv.URL}).Send(t.Context(), 853, "x")
	if err == nil || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("err = %v, want one without the URL", err)
	}

	bad, _ := fakeAPI(t, http.StatusBadGateway, `<html>bad gateway</html>`)
	if err := (&telegram.Bot{Token: token, BaseURL: bad.URL}).Send(t.Context(), 853, "x"); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("an answer that does not read: %v", err)
	}
	if err := (&telegram.Bot{BaseURL: bad.URL}).Send(t.Context(), 853, "x"); err == nil {
		t.Error("a send without a token succeeded")
	}
	broken := telegram.Bot{Token: "123456:made-up secret\x7f", BaseURL: bad.URL}
	if err := broken.Send(t.Context(), 853, "x"); err == nil || err.Error() != "telegram: a request that does not build" {
		t.Errorf("a token that breaks the URL: %v", err)
	}
	if err := (&telegram.Bot{Token: token, BaseURL: bad.URL}).Send(t.Context(), 0, "x"); err == nil {
		t.Error("a send to no chat succeeded")
	}
}

// units counts text's UTF-16 code units, as Telegram does.
func units(text string) int { return len(utf16.Encode([]rune(text))) }

// A text longer than Telegram takes, in UTF-16 code units, is cut at a
// character's end, ending with an ellipsis.
func TestSendCutsALongText(t *testing.T) {
	t.Parallel()

	srv, last := fakeAPI(t, http.StatusOK, `{"ok":true}`)
	bot := telegram.Bot{Token: token, BaseURL: srv.URL}
	for _, tc := range []struct {
		name string
		text string
		cut  bool
	}{
		{"letters, exactly the limit", strings.Repeat("ž", telegram.MaxText), false},
		{"letters past it", strings.Repeat("ž", telegram.MaxText+10), true},
		{"emoji, exactly the limit", strings.Repeat("🥗", telegram.MaxText/2), false},
		{"emoji past it, in units though not in characters", strings.Repeat("🥗", telegram.MaxText/2+1), true},
	} {
		if err := bot.Send(t.Context(), 853, tc.text); err != nil {
			t.Fatal(err)
		}
		text, _ := last.body["text"].(string)
		if got := strings.HasSuffix(text, "…"); got != tc.cut || units(text) > telegram.MaxText || !utf8.ValidString(text) {
			t.Errorf("%s: cut %v, %d units", tc.name, got, units(text))
		}
	}
}

// No error carries the token: not one of a transport that names the URL,
// nor a redirect's, which is refused.
func TestSendKeepsTheToken(t *testing.T) {
	t.Parallel()

	naming := &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("tracing: POST " + r.URL.String() + ": boom")
	})}
	err := (&telegram.Bot{Token: token, BaseURL: "http://example.invalid", Client: naming}).Send(t.Context(), 853, "x")
	if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "[redacted]") {
		t.Errorf("a transport that names the URL: %v", err)
	}

	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the redirect was followed")
	}))
	t.Cleanup(elsewhere.Close)
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/elsewhere", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirecting.Close)
	err = (&telegram.Bot{Token: token, BaseURL: redirecting.URL}).Send(t.Context(), 853, "x")
	if err == nil || strings.Contains(err.Error(), token) {
		t.Errorf("a redirect: %v", err)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
