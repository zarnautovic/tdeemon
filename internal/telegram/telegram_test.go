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
	for name, b := range map[string]telegram.Bot{"no token": {BaseURL: bad.URL}, "a token that breaks the URL": {Token: "a b\x7f", BaseURL: bad.URL}} {
		if err := b.Send(t.Context(), 853, "x"); err == nil || strings.Contains(err.Error(), "\x7f") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := (&telegram.Bot{Token: token, BaseURL: bad.URL}).Send(t.Context(), 0, "x"); err == nil {
		t.Error("a send to no chat succeeded")
	}
}

// A text longer than Telegram takes is cut, ending with an ellipsis.
func TestSendCutsALongText(t *testing.T) {
	t.Parallel()

	srv, last := fakeAPI(t, http.StatusOK, `{"ok":true}`)
	if err := (&telegram.Bot{Token: token, BaseURL: srv.URL}).Send(t.Context(), 853, strings.Repeat("ž", telegram.MaxText+10)); err != nil {
		t.Fatal(err)
	}
	text, _ := last.body["text"].(string)
	if utf8.RuneCountInString(text) != telegram.MaxText || !strings.HasSuffix(text, "…") {
		t.Errorf("a text of %d characters", utf8.RuneCountInString(text))
	}
	if err := (&telegram.Bot{Token: token, BaseURL: srv.URL}).Send(t.Context(), 853, strings.Repeat("ž", telegram.MaxText)); err != nil {
		t.Fatal(err)
	}
	if text, _ := last.body["text"].(string); strings.HasSuffix(text, "…") {
		t.Error("a text of exactly MaxText was cut")
	}
}
