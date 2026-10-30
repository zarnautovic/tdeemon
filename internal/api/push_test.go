package api_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/zarnautovic/tdeemon/internal/api"
)

type pushSubscriptionAnswer struct {
	ID         string  `json:"id"`
	Device     string  `json:"device"`
	LastSentAt *string `json:"last_sent_at"`
}

func listPush(t *testing.T, in *instance, session *http.Cookie) []pushSubscriptionAnswer {
	t.Helper()
	rec := in.do(t, http.MethodGet, "/api/v1/push/subscriptions", "", []*http.Cookie{session}, nil)
	var got struct {
		Subscriptions []pushSubscriptionAnswer `json:"subscriptions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	return got.Subscriptions
}

func subscribe(t *testing.T, in *instance, session *http.Cookie, endpoint, userAgent string) pushSubscriptionAnswer {
	t.Helper()
	var header map[string]string
	if userAgent != "" {
		header = map[string]string{"User-Agent": userAgent}
	}
	rec := in.do(t, http.MethodPut, "/api/v1/push/subscriptions", pushBody(t, endpoint), []*http.Cookie{session}, header)
	var got struct {
		Subscription pushSubscriptionAnswer `json:"subscription"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("subscribe %s: %d %s", endpoint, rec.Code, rec.Body.String())
	}
	return got.Subscription
}

// A device subscribes with the instance's key and is listed by what it
// is; the same endpoint again is the same device.
func TestPushSubscriptions(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	key := in.do(t, http.MethodGet, "/api/v1/push/key", "", []*http.Cookie{session}, nil)
	var k struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(key.Body.Bytes(), &k); key.Code != http.StatusOK || err != nil || len(k.PublicKey) != 87 {
		t.Fatalf("the key: %d %s", key.Code, key.Body.String())
	}
	phone := subscribe(t, in, session, "https://web.push.apple.com/QGx1f8kXyz", iPhoneSafari)
	if phone.Device != "iPhone · Safari" {
		t.Errorf("the phone is %q", phone.Device)
	}
	// Its keys renewed, the same device.
	if again := subscribe(t, in, session, "https://web.push.apple.com/QGx1f8kXyz", iPhoneSafari); again.ID != phone.ID {
		t.Errorf("again: %s, want %s", again.ID, phone.ID)
	}
	laptop := subscribe(t, in, session, "https://fcm.googleapis.com/fcm/send/dXk3", "")
	if subs := listPush(t, in, session); len(subs) != 2 || subs[0] != laptop || subs[1] != phone {
		t.Errorf("listed %+v, want the laptop and the phone", subs)
	}
	if laptop.Device != "Unknown device" || laptop.LastSentAt != nil {
		t.Errorf("the laptop: %+v", laptop)
	}
}

// The test goes to every device; a gone one is deleted, one that is down
// stays.
func TestTestPush(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	phone, laptop := "https://web.push.apple.com/QGx1f8kXyz", "https://fcm.googleapis.com/fcm/send/dXk3"
	subscribe(t, in, session, phone, iPhoneSafari)
	subscribe(t, in, session, laptop, "")
	rec := in.do(t, http.MethodPost, "/api/v1/push/test", "", cookies, nil)
	if strings.TrimSpace(rec.Body.String()) != `{"sent":2,"gone":0,"failed":0}` {
		t.Errorf("test: %d %s", rec.Code, rec.Body.String())
	}
	if got := in.pushes.endpoints(); len(got) != 2 || !slices.Contains(got, phone) || !slices.Contains(got, laptop) {
		t.Errorf("sent to %v", got)
	}
	in.pushes.answer(laptop, http.StatusGone)
	in.pushes.answer(phone, http.StatusBadGateway)
	rec = in.do(t, http.MethodPost, "/api/v1/push/test", "", cookies, nil)
	if strings.TrimSpace(rec.Body.String()) != `{"sent":0,"gone":1,"failed":1}` {
		t.Errorf("test with one gone, one down: %d %s", rec.Code, rec.Body.String())
	}
	if subs := listPush(t, in, session); len(subs) != 1 || subs[0].Device != "iPhone · Safari" || subs[0].LastSentAt == nil {
		t.Errorf("after the laptop went: %+v", subs)
	}
}

// A device's subscription is deleted once.
func TestDeletePushSubscription(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	phone := subscribe(t, in, session, "https://web.push.apple.com/QGx1f8kXyz", iPhoneSafari)
	if rec := in.do(t, http.MethodDelete, "/api/v1/push/subscriptions/"+phone.ID, "", cookies, nil); rec.Code != http.StatusNoContent {
		t.Errorf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := in.do(t, http.MethodDelete, "/api/v1/push/subscriptions/"+phone.ID, "", cookies, nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete again: %d, want 404", rec.Code)
	}
	if subs := listPush(t, in, session); len(subs) != 0 {
		t.Errorf("left %+v", subs)
	}
}

// A subscription that is no push service's, or has no keys, is refused,
// and nothing is sent to it.
func TestPushSubscriptionRefused(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	for name, body := range map[string]string{
		"a host of the network": pushBody(t, "https://192.168.1.10/push"),
		"plain http":            pushBody(t, "http://fcm.googleapis.com/fcm/send/x"),
		"no keys":               `{"endpoint": "https://fcm.googleapis.com/fcm/send/x", "keys": {"p256dh": "", "auth": ""}}`,
	} {
		if rec := in.do(t, http.MethodPut, "/api/v1/push/subscriptions", body, cookies, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s, want 422", name, rec.Code, rec.Body.String())
		}
	}
	if subs := listPush(t, in, session); len(subs) != 0 {
		t.Errorf("stored %+v", subs)
	}
}

// API tokens are kept out of every push operation.
func TestPushNeedsASession(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	tok, created := in.createToken(t, session, `{"name":"phone","scopes":["ingest","read","tools"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	for _, r := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/push/key", ""},
		{http.MethodGet, "/api/v1/push/subscriptions", ""},
		{http.MethodPut, "/api/v1/push/subscriptions", pushBody(t, "https://fcm.googleapis.com/fcm/send/x")},
		{http.MethodDelete, "/api/v1/push/subscriptions/0199e7f2-7b6a-7000-8000-000000000000", ""},
		{http.MethodPost, "/api/v1/push/test", ""},
	} {
		if rec := in.do(t, r.method, r.path, r.body, nil, bearer(tok.Token)); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with a token: %d, want 403", r.method, r.path, rec.Code)
		}
	}
}

// An instance without a sender says so.
func TestPushOff(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	base, _ := url.Parse("http://localhost")
	h, err := api.NewHandler(api.Config{Info: testInfo, DB: in.db, BaseURL: base, Logger: slog.New(slog.DiscardHandler), Now: in.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	off := &instance{h: h, db: in.db, clock: in.clock, user: in.user, logs: in.logs}
	cookies := []*http.Cookie{session}
	if rec := off.do(t, http.MethodGet, "/api/v1/push/key", "", cookies, nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("the key: %d, want 503", rec.Code)
	}
	if rec := off.do(t, http.MethodPost, "/api/v1/push/test", "", cookies, nil); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("the test: %d, want 503", rec.Code)
	}
}
