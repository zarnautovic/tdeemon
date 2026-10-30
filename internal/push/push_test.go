package push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"
)

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return b
}

// TestSealMatchesRFC8291 encrypts the example of RFC 8291, Appendix A,
// with its keys and salt, and gets its body.
func TestSealMatchesRFC8291(t *testing.T) {
	asPrivate, err := ecdh.P256().NewPrivateKey(mustDecode(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	sub := Subscription{
		Endpoint: "https://fcm.googleapis.com/fcm/send/x",
		P256dh:   "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		Auth:     "BTBZMqHH6r4Tts7J_aSIgg",
	}
	keys, err := sub.keys()
	if err != nil {
		t.Fatal(err)
	}
	got, err := seal([]byte("When I grow up, I want to be a watermelon"), keys, asPrivate, mustDecode(t, "DGv6ra1nlYgDCS1FRnbzlw"))
	if err != nil {
		t.Fatal(err)
	}
	// The RFC's record size is 4096 too.
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if g := base64.RawURLEncoding.EncodeToString(got); g != want {
		t.Errorf("body\n got %s\nwant %s", g, want)
	}
}

// browser is a subscriber: its keys, and the decryption a browser does.
type browser struct {
	private *ecdh.PrivateKey
	auth    []byte
}

func newBrowser(t *testing.T) browser {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return browser{private: k, auth: auth}
}

func (b browser) subscription(endpoint string) Subscription {
	enc := base64.RawURLEncoding
	return Subscription{Endpoint: endpoint, P256dh: enc.EncodeToString(b.private.PublicKey().Bytes()), Auth: enc.EncodeToString(b.auth)}
}

// open decrypts an aes128gcm body as a browser does.
func (b browser) open(t *testing.T, body []byte) []byte {
	t.Helper()
	salt, rs, idlen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	if rs != recordSize || idlen != 65 {
		t.Fatalf("record size %d, key id %d bytes", rs, idlen)
	}
	asPublic, err := ecdh.P256().NewPublicKey(body[21 : 21+idlen])
	if err != nil {
		t.Fatal(err)
	}
	shared, err := b.private.ECDH(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	prk, _ := hkdf.Extract(sha256.New, shared, b.auth)
	info := append(append([]byte("WebPush: info\x00"), b.private.PublicKey().Bytes()...), asPublic.Bytes()...)
	ikm, _ := hkdf.Expand(sha256.New, prk, string(info), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[21+idlen:], nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain[len(plain)-1] != 2 {
		t.Fatalf("the last record's delimiter is %d", plain[len(plain)-1])
	}
	return plain[:len(plain)-1]
}

// service is a push service: it keeps the requests and answers status.
type service struct {
	status int
	// redirect, when not empty, is the answer's Location.
	redirect string
	got      []*http.Request
	bodies   [][]byte
}

func (s *service) RoundTrip(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	s.got = append(s.got, r)
	s.bodies = append(s.bodies, b)
	h := http.Header{}
	if s.redirect != "" {
		h.Set("Location", s.redirect)
	}
	return &http.Response{StatusCode: s.status, Body: io.NopCloser(strings.NewReader("")), Header: h, Request: r}, nil
}

func newTestSender(t *testing.T, svc *service, now time.Time) *Sender {
	t.Helper()
	s, err := NewSender([]byte("a secret of at least thirty-two bytes"), "https://tdeemon.example.com")
	if err != nil {
		t.Fatal(err)
	}
	s.Client = &http.Client{Transport: svc}
	s.Now = func() time.Time { return now }
	return s
}

func TestSendDeliversWhatTheBrowserDecrypts(t *testing.T) {
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	svc := &service{status: http.StatusCreated}
	s := newTestSender(t, svc, now)
	b := newBrowser(t)
	sub := b.subscription("https://web.push.apple.com/QGx1f8kXyz")
	payload, _ := json.Marshal(Notification{Title: "Jutarnji sažetak", Body: "Jučer 2140/2300 kcal", URL: "/?summary=2026-10-30", Tag: "morning"})
	err := s.Send(t.Context(), sub, Message{Payload: payload, TTL: 4 * time.Hour, Urgency: UrgencyLow, Topic: "morning"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(svc.got) != 1 {
		t.Fatalf("%d requests, want 1", len(svc.got))
	}
	r := svc.got[0]
	if r.Method != http.MethodPost || r.URL.String() != sub.Endpoint {
		t.Errorf("%s %s, want POST to the endpoint", r.Method, r.URL)
	}
	for h, want := range map[string]string{"Content-Encoding": "aes128gcm", "TTL": "14400", "Urgency": "low", "Topic": "morning"} {
		if got := r.Header.Get(h); got != want {
			t.Errorf("%s: %q, want %q", h, got, want)
		}
	}
	if got := b.open(t, svc.bodies[0]); !bytes.Equal(got, payload) {
		t.Errorf("the browser decrypts %q, want %q", got, payload)
	}
	checkToken(t, r.Header.Get("Authorization"), s.PublicKey(), "https://web.push.apple.com", now)
}

// checkToken checks a VAPID Authorization header: its key, and a token
// for aud signed by it that expires within a day.
func checkToken(t *testing.T, header, public, aud string, now time.Time) {
	t.Helper()
	rest, ok := strings.CutPrefix(header, "vapid t=")
	token, k, ok2 := strings.Cut(rest, ", k=")
	if !ok || !ok2 || k != public {
		t.Fatalf("Authorization %q", header)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", token)
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(mustDecode(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != aud || claims.Sub != "https://tdeemon.example.com" ||
		claims.Exp <= now.Unix() || claims.Exp > now.Add(24*time.Hour).Unix() {
		t.Errorf("claims %+v", claims)
	}
	key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), mustDecode(t, public))
	if err != nil {
		t.Fatal(err)
	}
	sig := mustDecode(t, parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("the token's signature does not verify")
	}
}

func TestSendAnswers(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusCreated: nil, http.StatusNotFound: ErrGone, http.StatusGone: ErrGone,
	} {
		svc := &service{status: status}
		err := newTestSender(t, svc, time.Now()).Send(t.Context(), newBrowser(t).subscription("https://fcm.googleapis.com/fcm/send/abc"), Message{Payload: []byte("{}")})
		if !errors.Is(err, want) || want == nil && err != nil {
			t.Errorf("%d: %v, want %v", status, err, want)
		}
	}
	svc := &service{status: http.StatusTooManyRequests}
	err := newTestSender(t, svc, time.Now()).Send(t.Context(), newBrowser(t).subscription("https://fcm.googleapis.com/fcm/send/abc"), Message{Payload: []byte("{}")})
	if err == nil || errors.Is(err, ErrGone) {
		t.Errorf("429: %v, want an error that is not ErrGone", err)
	}
}

func TestSendRefusesWhatIsNoPushServicesOrTooLong(t *testing.T) {
	b := newBrowser(t)
	for _, endpoint := range []string{
		"http://fcm.googleapis.com/fcm/send/abc",
		"https://example.com/push",
		"https://fcm.googleapis.com.example.com/x",
		"https://evilpush.apple.com/x",
		"https://fcm.googleapis.com:8443/x",
		"https://user@fcm.googleapis.com/x",
		"https://127.0.0.1/x",
		"https://[::ffff:127.0.0.1%25x.push.apple.com]/x",
		"https://[::1]/x",
		"not a URL",
	} {
		svc := &service{status: http.StatusCreated}
		err := newTestSender(t, svc, time.Now()).Send(t.Context(), b.subscription(endpoint), Message{Payload: []byte("{}")})
		if !errors.Is(err, ErrInvalidSubscription) || len(svc.got) != 0 {
			t.Errorf("%s: %v and %d requests, want ErrInvalidSubscription and none", endpoint, err, len(svc.got))
		}
	}
	bad := b.subscription("https://fcm.googleapis.com/fcm/send/abc")
	bad.Auth = "c2hvcnQ"
	if err := bad.Validate(); !errors.Is(err, ErrInvalidSubscription) {
		t.Errorf("a short auth: %v", err)
	}
	bad = b.subscription("https://fcm.googleapis.com/fcm/send/abc")
	bad.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	if err := bad.Validate(); !errors.Is(err, ErrInvalidSubscription) {
		t.Errorf("a key off the curve: %v", err)
	}
	padded := b.subscription("https://updates.push.services.mozilla.com/wpush/v2/abc")
	padded.Auth += "=="
	if err := padded.Validate(); err != nil {
		t.Errorf("Firefox's endpoint with a padded auth: %v", err)
	}
	svc := &service{status: http.StatusCreated}
	err := newTestSender(t, svc, time.Now()).Send(t.Context(), b.subscription("https://fcm.googleapis.com/fcm/send/abc"), Message{Payload: make([]byte, MaxPayload+1)})
	if err == nil || len(svc.got) != 0 {
		t.Errorf("a payload too long: %v and %d requests", err, len(svc.got))
	}
}

// A redirect is not followed: the 3xx is the push service's answer.
func TestSendFollowsNoRedirect(t *testing.T) {
	svc := &service{status: http.StatusFound}
	s := newTestSender(t, svc, time.Now())
	svc.redirect = "http://127.0.0.1:8080/internal"
	err := s.Send(t.Context(), newBrowser(t).subscription("https://fcm.googleapis.com/fcm/send/abc"), Message{Payload: []byte("{}")})
	if err == nil || len(svc.got) != 1 {
		t.Errorf("a redirect: %v and %d requests, want an error and one", err, len(svc.got))
	}
}

// The token's audience is the origin serialized, its host in lower case.
func TestTokenAudienceIsLowerCase(t *testing.T) {
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	svc := &service{status: http.StatusCreated}
	s := newTestSender(t, svc, now)
	if err := s.Send(t.Context(), newBrowser(t).subscription("https://FCM.GoogleAPIs.com/fcm/send/abc"), Message{Payload: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	checkToken(t, svc.got[0].Header.Get("Authorization"), s.PublicKey(), "https://fcm.googleapis.com", now)
}

func TestNewSenderDerivesTheKeyFromTheSecret(t *testing.T) {
	a, _ := NewSender([]byte("a secret of at least thirty-two bytes"), "https://a.example")
	b, _ := NewSender([]byte("a secret of at least thirty-two bytes"), "https://b.example")
	c, _ := NewSender([]byte("another secret of thirty-two bytes!"), "https://a.example")
	if a.PublicKey() != b.PublicKey() || a.PublicKey() == c.PublicKey() {
		t.Errorf("keys %s, %s, %s: want the same for one secret, another for another", a.PublicKey(), b.PublicKey(), c.PublicKey())
	}
	if k := mustDecode(t, a.PublicKey()); len(k) != 65 || k[0] != 4 {
		t.Errorf("the public key is %d bytes, want 65 uncompressed", len(k))
	}
	if _, err := NewSender(nil, "https://a.example"); err == nil {
		t.Error("no secret: want an error")
	}
	if _, err := NewSender([]byte("x"), "ftp://a.example"); err == nil {
		t.Error("a subject that is no URL: want an error")
	}
}
