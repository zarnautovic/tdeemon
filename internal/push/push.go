// Package push sends a notification to a browser by Web Push: the
// message encrypted for the subscription (RFC 8291, aes128gcm of RFC
// 8188) and the request signed by the instance's VAPID key (RFC 8292),
// to the push service the browser subscribed with (Apple's, Google's,
// Mozilla's, Microsoft's).
package push

import (
	"bytes"
	"context"
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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MaxPayload bounds a message's payload: a push service takes 4096 bytes
// of a body, of which the header and the tag take 103.
const MaxPayload = 3993

// timeout bounds a request to a push service.
const timeout = 15 * time.Second

// tokenLifetime is how long a VAPID token holds; RFC 8292 allows a day.
const tokenLifetime = 12 * time.Hour

// Why a subscription takes no message.
var (
	// ErrGone: the push service no longer has the subscription (404 or
	// 410), which the browser dropped or the user unsubscribed from; it
	// is to be deleted.
	ErrGone = errors.New("the push subscription is gone")
	// ErrInvalidSubscription: the subscription's endpoint is no push
	// service's, or its keys are no keys.
	ErrInvalidSubscription = errors.New("the push subscription is not valid")
)

// pushServices are the hosts of the push services browsers subscribe
// with, a host or a suffix of one: a request goes nowhere else, as an
// endpoint is a URL the client gives.
var pushServices = []string{
	"fcm.googleapis.com",         // Chrome, Opera, Edge on Android
	"android.googleapis.com",     // Chrome, before FCM
	".push.apple.com",            // Safari, every browser on iOS
	"push.services.mozilla.com",  // Firefox
	".push.services.mozilla.com", // Firefox (updates.)
	".notify.windows.com",        // Edge on Windows
}

// Subscription is a browser's: where to send and the keys to encrypt
// for, as PushSubscription.toJSON() gives them.
type Subscription struct {
	// Endpoint is the push service's URL of the subscription.
	Endpoint string
	// P256dh is the browser's P-256 public key, uncompressed,
	// base64url.
	P256dh string
	// Auth is the browser's 16-byte authentication secret, base64url.
	Auth string
}

// Validate checks that s is a push service's subscription with keys.
func (s Subscription) Validate() error {
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !isPushService(u.Hostname()) {
		return fmt.Errorf("%w: the endpoint is no push service's", ErrInvalidSubscription)
	}
	if _, err := s.keys(); err != nil {
		return err
	}
	return nil
}

// isPushService reports whether host is a push service's name: a name,
// not an address (an IPv6 one's zone can end in a name).
func isPushService(host string) bool {
	host = strings.ToLower(host)
	for _, r := range host {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return false
		}
	}
	for _, s := range pushServices {
		if host == s || strings.HasPrefix(s, ".") && strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}

// subscriptionKeys are a subscription's keys, decoded.
type subscriptionKeys struct {
	public *ecdh.PublicKey
	auth   []byte
}

func (s Subscription) keys() (subscriptionKeys, error) {
	p, err := decode(s.P256dh)
	if err != nil {
		return subscriptionKeys{}, fmt.Errorf("%w: p256dh is not base64url", ErrInvalidSubscription)
	}
	public, err := ecdh.P256().NewPublicKey(p)
	if err != nil {
		return subscriptionKeys{}, fmt.Errorf("%w: p256dh is no P-256 key", ErrInvalidSubscription)
	}
	auth, err := decode(s.Auth)
	if err != nil || len(auth) != 16 {
		return subscriptionKeys{}, fmt.Errorf("%w: auth is not 16 bytes of base64url", ErrInvalidSubscription)
	}
	return subscriptionKeys{public: public, auth: auth}, nil
}

// decode reads base64url, padded or not, as browsers give either.
func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// Urgency is how soon a device should wake for a message (RFC 8030).
type Urgency string

// The urgencies a message is sent with.
const (
	UrgencyLow    Urgency = "low"
	UrgencyNormal Urgency = "normal"
	UrgencyHigh   Urgency = "high"
)

// Message is what is sent to a subscription.
type Message struct {
	// Payload is what the service worker's push event gets, at most
	// MaxPayload bytes.
	Payload []byte
	// TTL is how long the push service keeps the message for a device
	// that is off; zero sends it only to one that is on.
	TTL time.Duration
	// Urgency is normal when empty.
	Urgency Urgency
	// Topic, when not empty, makes the message replace one of the same
	// topic still waiting at the push service: at most 32 characters of
	// base64url's alphabet.
	Topic string
}

// Notification is the payload the web app's service worker shows.
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// URL is the app's page a tap on the notification opens, a path.
	URL string `json:"url"`
	// Tag replaces a notification of the same tag still on the device.
	Tag string `json:"tag,omitempty"`
	// Silent shows it without a sound or a vibration: news that asks
	// nothing of the user.
	Silent bool `json:"silent,omitempty"`
}

// Sender sends messages, signed by its VAPID key.
type Sender struct {
	key *ecdsa.PrivateKey
	// public is the key's public half, uncompressed, base64url: the
	// applicationServerKey a browser subscribes with.
	public string
	// subject is the contact a push service may reach about the sender:
	// the instance's URL.
	subject string
	// Client is the HTTP client; one with a timeout when nil.
	Client *http.Client
	// Now returns the current time; time.Now when nil.
	Now func() time.Time
}

// NewSender returns a Sender whose VAPID key is derived from secret
// (HKDF): the same secret, the same key, so the subscriptions browsers
// made hold across restarts; another secret makes them all fail, and
// browsers subscribe again. subject is the instance's URL, which a push
// service may use to reach its admin.
func NewSender(secret []byte, subject string) (*Sender, error) {
	if len(secret) == 0 {
		return nil, errors.New("push: the secret is empty")
	}
	if u, err := url.Parse(subject); err != nil || u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto" {
		return nil, fmt.Errorf("push: the subject %q is no URL", subject)
	}
	// A scalar outside P-256's order is rare (about 2^-32); the next
	// counter derives another.
	for i := range 16 {
		d, err := hkdf.Key(sha256.New, secret, nil, "tdeemon push: vapid "+strconv.Itoa(i), 32)
		if err != nil {
			return nil, fmt.Errorf("push: derive the key: %w", err)
		}
		key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), d)
		if err != nil {
			continue
		}
		public, err := key.PublicKey.Bytes()
		if err != nil {
			return nil, fmt.Errorf("push: the public key: %w", err)
		}
		return &Sender{key: key, public: base64.RawURLEncoding.EncodeToString(public), subject: subject}, nil
	}
	return nil, errors.New("push: no key derives from the secret")
}

// PublicKey returns the VAPID public key, uncompressed and base64url,
// which a browser subscribes with (applicationServerKey).
func (s *Sender) PublicKey() string { return s.public }

// Send sends m to sub. It returns ErrGone for a subscription the push
// service no longer has, and ErrInvalidSubscription for one that is not
// valid.
func (s *Sender) Send(ctx context.Context, sub Subscription, m Message) error {
	if err := sub.Validate(); err != nil {
		return err
	}
	if len(m.Payload) > MaxPayload {
		return fmt.Errorf("push: the payload's %d bytes are over %d", len(m.Payload), MaxPayload)
	}
	keys, err := sub.keys()
	if err != nil {
		return err
	}
	body, err := encrypt(m.Payload, keys, rand.Reader)
	if err != nil {
		return err
	}
	token, err := s.token(sub.Endpoint)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int(m.TTL.Seconds())))
	req.Header.Set("Urgency", string(orNormal(m.Urgency)))
	if m.Topic != "" {
		req.Header.Set("Topic", m.Topic)
	}
	req.Header.Set("Authorization", "vapid t="+token+", k="+s.public)
	client := &http.Client{Timeout: timeout}
	if s.Client != nil {
		c := *s.Client
		client = &c
	}
	// A push service does not redirect; a redirect would lead past the
	// check of the endpoint.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	detail, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return ErrGone
	case res.StatusCode < 200 || res.StatusCode > 299:
		return fmt.Errorf("push: the push service answered %d: %s", res.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}

func orNormal(u Urgency) Urgency {
	if u == "" {
		return UrgencyNormal
	}
	return u
}

// token returns a VAPID token (a JWT, ES256) for endpoint's push service.
func (s *Sender) token(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	claims, err := json.Marshal(map[string]any{
		// The origin, serialized: its host in lower case.
		"aud": u.Scheme + "://" + strings.ToLower(u.Host),
		"exp": now().Add(tokenLifetime).Unix(),
		"sub": s.subject,
	})
	if err != nil {
		return "", fmt.Errorf("push: %w", err)
	}
	enc := base64.RawURLEncoding
	signed := enc.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signed))
	r, sig, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", fmt.Errorf("push: sign the token: %w", err)
	}
	// JWS's ES256 signature is r and s, 32 bytes each.
	raw := make([]byte, 64)
	r.FillBytes(raw[:32])
	sig.FillBytes(raw[32:])
	return signed + "." + enc.EncodeToString(raw), nil
}

// recordSize is the record size the body declares: one record, as a
// payload fits in one.
const recordSize = 4096

// encrypt encrypts payload for keys (RFC 8291) into an aes128gcm body
// (RFC 8188) of one record: the salt, the record size, the sender's
// ephemeral public key, and the ciphertext. random gives the ephemeral
// key and the salt.
func encrypt(payload []byte, keys subscriptionKeys, random io.Reader) ([]byte, error) {
	ephemeral, err := ecdh.P256().GenerateKey(random)
	if err != nil {
		return nil, fmt.Errorf("push: the ephemeral key: %w", err)
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(random, salt); err != nil {
		return nil, fmt.Errorf("push: the salt: %w", err)
	}
	return seal(payload, keys, ephemeral, salt)
}

// seal is encrypt with its ephemeral key and salt given.
func seal(payload []byte, keys subscriptionKeys, ephemeral *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	shared, err := ephemeral.ECDH(keys.public)
	if err != nil {
		return nil, fmt.Errorf("push: the shared secret: %w", err)
	}
	asPublic := ephemeral.PublicKey().Bytes()
	uaPublic := keys.public.Bytes()

	keyInfo := append(append([]byte("WebPush: info\x00"), uaPublic...), asPublic...)
	prk, err := hkdf.Extract(sha256.New, shared, keys.auth)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	ikm, err := hkdf.Expand(sha256.New, prk, string(keyInfo), 32)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}

	header := make([]byte, 0, 16+4+1+len(asPublic))
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, recordSize)
	// The key id's length: an uncompressed P-256 key's 65 bytes.
	header = append(header, 65)
	header = append(header, asPublic...)
	// The last record's delimiter, 2, and no padding.
	plain := append(append(make([]byte, 0, len(payload)+1), payload...), 2)
	return gcm.Seal(header, nonce, plain, nil), nil
}
