package server_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/zarnautovic/tdeemon/internal/server"
)

// start runs server.Run in the background and returns the base URL, a
// cancel func that triggers shutdown, and a channel with Run's result.
func start(t *testing.T, h http.Handler) (string, context.CancelFunc, <-chan error) {
	t.Helper()

	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, ln, h, slog.New(slog.DiscardHandler)) }()
	return "http://" + ln.Addr().String(), cancel, done
}

// get fetches url and returns the status and the body, which it closes.
// It returns errors instead of failing the test, so a goroutine can use it.
func get(ctx context.Context, url string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}

func waitDone(t *testing.T, done <-chan error) {
	t.Helper()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil after a clean shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunServesUntilCancelled(t *testing.T) {
	t.Parallel()

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello")
	})
	base, cancel, done := start(t, h)

	status, body, err := get(t.Context(), base+"/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if status != http.StatusOK || body != "hello" {
		t.Fatalf("got %d %q, want 200 \"hello\"", status, body)
	}

	cancel()
	waitDone(t, done)
}

func TestRunWaitsForInFlightRequests(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce) // a failing test must not leave the handler waiting
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})
	base, cancel, done := start(t, h)

	type result struct {
		status int
		body   string
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		status, body, err := get(t.Context(), base+"/slow")
		resCh <- result{status, body, err}
	}()
	select {
	case <-started:
	case res := <-resCh:
		t.Fatalf("the request ended before the handler ran: %d %q, %v", res.status, res.body, res.err)
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not start")
	}

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned %v while a request was still in flight", err)
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce()
	var res result
	select {
	case res = <-resCh:
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight request did not finish")
	}
	if res.err != nil {
		t.Fatalf("in-flight request failed: %v", res.err)
	}
	if res.status != http.StatusOK || res.body != "finished" {
		t.Fatalf("in-flight request got %d %q, want 200 \"finished\"", res.status, res.body)
	}
	waitDone(t, done)
}
