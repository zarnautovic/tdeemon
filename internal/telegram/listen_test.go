package telegram_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// Updates asks getUpdates for the messages from the offset on, waiting,
// and returns each with its chat, text and time; an update that is not a
// message has no chat.
func TestUpdates(t *testing.T) {
	t.Parallel()

	srv, last := fakeAPI(t, http.StatusOK, `{"ok":true,"result":[
		{"update_id":41,"message":{"message_id":7,"date":1792760700,"text":"/sazetak","chat":{"id":853,"type":"private"}}},
		{"update_id":42,"my_chat_member":{"chat":{"id":-100123}}},
		{"update_id":43,"message":{"date":1792760701,"migrate_to_chat_id":-1001234567,"chat":{"id":-100123,"type":"group"}}},
		{"update_id":44,"message":{"date":1792760701,"migrate_from_chat_id":-100123,"chat":{"id":-1001234567,"type":"supergroup"}}}]}`)
	got, err := (&telegram.Bot{Token: token, BaseURL: srv.URL}).Updates(t.Context(), 41, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if last.path != "/bot"+token+"/getUpdates" || last.body["offset"] != float64(41) || last.body["timeout"] != float64(30) ||
		last.body["limit"] != float64(20) {
		t.Errorf("request = %s %v", last.path, last.body)
	}
	if a, _ := last.body["allowed_updates"].([]any); len(a) != 1 || a[0] != "message" {
		t.Errorf("allowed_updates = %v", last.body["allowed_updates"])
	}
	want := []telegram.Update{
		{ID: 41, ChatID: 853, Text: "/sazetak", Sent: time.Unix(1792760700, 0)},
		{ID: 42},
		{ID: 43, ChatID: -100123, Sent: time.Unix(1792760701, 0), MigrateTo: -1001234567},
		{ID: 44, ChatID: -1001234567, Sent: time.Unix(1792760701, 0), MigrateFrom: -100123},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Updates = %+v, want %+v", got, want)
	}
}

// Updates returns none when none came, and a refusal, or a result that
// does not read, as an error without the token.
func TestUpdatesNone(t *testing.T) {
	t.Parallel()

	none, _ := fakeAPI(t, http.StatusOK, `{"ok":true,"result":[]}`)
	if got, err := (&telegram.Bot{Token: token, BaseURL: none.URL}).Updates(t.Context(), 0, 0); err != nil || len(got) != 0 {
		t.Errorf("none = %v, %v", got, err)
	}
	conflict, _ := fakeAPI(t, http.StatusConflict,
		`{"ok":false,"error_code":409,"description":"Conflict: terminated by other getUpdates request; make sure that only one bot instance is running"}`)
	_, err := (&telegram.Bot{Token: token, BaseURL: conflict.URL}).Updates(t.Context(), 0, 0)
	if te, ok := errors.AsType[*telegram.Error](err); !ok || te.Code != http.StatusConflict {
		t.Errorf("another program polling = %v", err)
	}
	bad, _ := fakeAPI(t, http.StatusOK, `{"ok":true,"result":{"not":"a list"}}`)
	if _, err := (&telegram.Bot{Token: token, BaseURL: bad.URL}).Updates(t.Context(), 0, 0); err == nil {
		t.Error("a result that is not a list read")
	}
	naming, _ := fakeAPI(t, http.StatusNotFound, `{"ok":false,"error_code":404,"description":"Not Found: /bot`+token+`/getUpdates"}`)
	if _, err := (&telegram.Bot{Token: token, BaseURL: naming.URL}).Updates(t.Context(), 0, 0); err == nil || strings.Contains(err.Error(), token) {
		t.Errorf("a refusal that names the path: %v", err)
	}
}

// SetCommands sets the menu's commands for a language, or for every
// other one.
func TestSetCommands(t *testing.T) {
	t.Parallel()

	srv, last := fakeAPI(t, http.StatusOK, `{"ok":true,"result":true}`)
	bot := telegram.Bot{Token: token, BaseURL: srv.URL}
	if err := bot.SetCommands(t.Context(), []telegram.Command{{Name: "pomoc", Description: "Naredbe"}}, "hr"); err != nil {
		t.Fatal(err)
	}
	cmds, _ := last.body["commands"].([]any)
	if last.path != "/bot"+token+"/setMyCommands" || last.body["language_code"] != "hr" || len(cmds) != 1 {
		t.Fatalf("request = %s %v", last.path, last.body)
	}
	if c, _ := cmds[0].(map[string]any); c["command"] != "pomoc" || c["description"] != "Naredbe" {
		t.Errorf("command = %v", cmds[0])
	}
	if err := bot.SetCommands(t.Context(), nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := last.body["language_code"]; ok {
		t.Errorf("every other language: %v", last.body)
	}
}

// Listen gives each message, in order and once, to handle, passing over
// updates that are not messages; it tries again after a failure, and
// confirms what it handled when it stops.
func TestListen(t *testing.T) {
	t.Parallel()

	url, offsets := fakeUpdates(t)
	ctx, cancel := context.WithCancel(t.Context())
	var got []telegram.Update
	cut := errors.New("the second message was not handled")
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&telegram.Bot{Token: token, BaseURL: url}).Listen(ctx, slog.New(slog.DiscardHandler), func(hctx context.Context, u telegram.Update) {
			got = append(got, u)
			if len(got) == 2 {
				cancel()
				cut = hctx.Err()
			}
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Listen did not stop")
	}
	want := []telegram.Update{{ID: 10, ChatID: 853, Text: "/pomoc", Sent: time.Unix(1, 0)}, {ID: 12, ChatID: -100123, Text: "/sazetak", Sent: time.Unix(2, 0)}}
	if !slices.Equal(got, want) {
		t.Errorf("handled = %+v", got)
	}
	// The message being handled when the listener stops is handled to
	// its end.
	if cut != nil {
		t.Errorf("the handling's context once the listener stopped: %v", cut)
	}
	// The first from 0, the next from after 11 (failed, then again), and
	// the confirmation from after 12.
	if o := offsets(); len(o) < 4 || o[0] != 0 || o[1] != 12 || o[2] != 12 || o[len(o)-1] != 13 {
		t.Errorf("offsets = %v", o)
	}
}

// A listener that stops in a batch handles the message it is on, and
// confirms only up to it: the rest come again after a restart.
func TestListenStopsInABatch(t *testing.T) {
	t.Parallel()

	srv, last := fakeAPI(t, http.StatusOK, `{"ok":true,"result":[
		{"update_id":1,"message":{"date":1,"text":"/pomoc","chat":{"id":853}}},
		{"update_id":2,"message":{"date":1,"text":"/sazetak","chat":{"id":853}}},
		{"update_id":3,"message":{"date":1,"text":"/help","chat":{"id":853}}}]}`)
	ctx, cancel := context.WithCancel(t.Context())
	var handled []int64
	(&telegram.Bot{Token: token, BaseURL: srv.URL}).Listen(ctx, slog.New(slog.DiscardHandler), func(_ context.Context, u telegram.Update) {
		handled = append(handled, u.ID)
		cancel()
	})
	if len(handled) != 1 || handled[0] != 1 {
		t.Errorf("handled = %v, want only the first", handled)
	}
	if last.body["offset"] != float64(2) || last.body["timeout"] != float64(0) {
		t.Errorf("the confirmation = %v, want from 2", last.body)
	}
}

// fakeUpdates is a Bot API whose getUpdates answers two messages and an
// edit, then fails once, then answers a message, then waits as Telegram
// does, until the request ends; one that does not wait (a confirmation)
// gets none. offsets returns the offsets asked from.
func fakeUpdates(t *testing.T) (url string, offsets func() []int64) {
	t.Helper()
	var mu sync.Mutex
	var got []int64
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Offset  int64 `json:"offset"`
			Timeout int   `json:"timeout"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		calls++
		n := calls
		got = append(got, body.Offset)
		mu.Unlock()
		switch n {
		case 1:
			_, _ = io.WriteString(w, `{"ok":true,"result":[
				{"update_id":10,"message":{"date":1,"text":"/pomoc","chat":{"id":853}}},
				{"update_id":11,"edited_message":{"date":1,"text":"/x","chat":{"id":853}}}]}`)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":502,"description":"Bad Gateway"}`)
		case 3:
			_, _ = io.WriteString(w, `{"ok":true,"result":[{"update_id":12,"message":{"date":2,"text":"/sazetak","chat":{"id":-100123}}}]}`)
		default:
			if body.Timeout == 0 { // the confirmation
				_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
				return
			}
			<-r.Context().Done() // waiting, as Telegram does, until the listener stops
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []int64 { mu.Lock(); defer mu.Unlock(); return append([]int64(nil), got...) }
}
