package api_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// pushServices stand in for the push services: they take every request
// and answer 201, or the status of an endpoint's in status.
type pushServices struct {
	mu     sync.Mutex
	status map[string]int
	// sent are the endpoints that got a request, in order.
	sent []string
}

func (p *pushServices) RoundTrip(r *http.Request) (*http.Response, error) {
	_, _ = io.Copy(io.Discard, r.Body)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, r.URL.String())
	status, ok := p.status[r.URL.String()]
	if !ok {
		status = http.StatusCreated
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

// answer makes the endpoint's push service answer status.
func (p *pushServices) answer(endpoint string, status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status == nil {
		p.status = map[string]int{}
	}
	p.status[endpoint] = status
}

// endpoints returns the endpoints that got a request, and forgets them.
func (p *pushServices) endpoints() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.sent
	p.sent = nil
	return out
}

// pushBody is the body of save-push-subscription for endpoint, with a
// browser's fresh keys.
func pushBody(t *testing.T, endpoint string) string {
	t.Helper()
	p256dh, auth := browserKeys(t)
	return fmt.Sprintf(`{"endpoint": %q, "expirationTime": null, "keys": {"p256dh": %q, "auth": %q}}`, endpoint, p256dh, auth)
}

// browserKeys returns a browser's fresh keys of a subscription, base64url.
func browserKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(key.PublicKey().Bytes()), enc.EncodeToString(secret)
}

const iPhoneSafari = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.6 Mobile/15E148 Safari/604.1"
