package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/config"
)

// listen sets the bot's commands, in English and in Croatian, and reads
// its messages until its context ends; then its wait returns.
func TestListen(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var calls []string
	polling := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		mu.Lock()
		if lang, _ := body["language_code"].(string); method == "setMyCommands" {
			method += ":" + lang
		}
		calls = append(calls, method)
		mu.Unlock()
		if method == "getUpdates" {
			select {
			case polling <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(t.Context())
	bot := telegramBot(config.Config{TelegramBotToken: testBotToken, TelegramAPIURL: srv.URL})
	wait := listen(ctx, bot, api.Config{Logger: slog.New(slog.DiscardHandler)})
	select {
	case <-polling:
	case <-time.After(10 * time.Second):
		t.Fatal("the bot's messages are not read")
	}
	cancel()
	stopped := make(chan struct{})
	go func() { wait(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("listen did not stop")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 3 || !slices.Contains(calls[:2], "setMyCommands:") || !slices.Contains(calls[:2], "setMyCommands:hr") || calls[2] != "getUpdates" {
		t.Errorf("calls = %v", calls)
	}
}
