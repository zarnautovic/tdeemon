package server_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
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

func get(t *testing.T, url string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
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

	resp := get(t, base+"/")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "hello" {
		t.Fatalf("got %d %q, want 200 \"hello\"", resp.StatusCode, body)
	}

	cancel()
	waitDone(t, done)
}

func TestRunWaitsForInFlightRequests(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})
	base, cancel, done := start(t, h)

	type result struct {
		resp *http.Response
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		// t.Fatal must not be called from this goroutine; report the error.
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/slow", nil)
		if err != nil {
			resCh <- result{err: err}
			return
		}
		resp, err := http.DefaultClient.Do(req)
		resCh <- result{resp, err}
	}()
	<-started

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run returned %v while a request was still in flight", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	res := <-resCh
	if res.err != nil {
		t.Fatalf("in-flight request failed: %v", res.err)
	}
	resp := res.resp
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "finished" {
		t.Fatalf("in-flight request got %d %q, want 200 \"finished\"", resp.StatusCode, body)
	}
	waitDone(t, done)
}
