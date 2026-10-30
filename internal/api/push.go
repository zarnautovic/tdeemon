package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/notify"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// testPushTTL is how long a push service keeps a test for a device that
// is off: a test that comes an hour late tests nothing.
const testPushTTL = 10 * time.Minute

type pushKeyOutput struct {
	Body struct {
		PublicKey string `json:"public_key" doc:"The instance's VAPID public key, uncompressed and base64url: the applicationServerKey a browser subscribes with."`
	}
}

// pushSubscriptionBody is a device's subscription as the list shows it:
// what it is, not where its messages go.
type pushSubscriptionBody struct {
	ID         string     `json:"id" format:"uuid"`
	Device     string     `json:"device" example:"iPhone · Safari" doc:"What the device is, from its browser's user agent."`
	CreatedAt  time.Time  `json:"created_at"`
	LastSentAt *time.Time `json:"last_sent_at,omitempty" doc:"When the last notification went to it; none yet when absent."`
}

func newPushSubscriptionBody(s store.PushSubscription) pushSubscriptionBody {
	b := pushSubscriptionBody{ID: s.ID.String(), Device: s.Device, CreatedAt: s.CreatedAt}
	if !s.LastSentAt.IsZero() {
		b.LastSentAt = &s.LastSentAt
	}
	return b
}

type savePushInput struct {
	UserAgent string `header:"User-Agent"`
	Body      struct {
		Endpoint string `json:"endpoint" maxLength:"2048" doc:"The push service's URL of the subscription."`
		// The browser's own field, which a push service's answer replaces.
		ExpirationTime *float64 `json:"expirationTime,omitempty" nullable:"true" doc:"When the subscription expires, in ms since 1970; ignored."`
		Keys           struct {
			P256dh string `json:"p256dh" maxLength:"100" doc:"The browser's P-256 public key, base64url."`
			Auth   string `json:"auth" maxLength:"40" doc:"The browser's authentication secret, base64url."`
		} `json:"keys"`
	} `doc:"The subscription as the browser's PushSubscription.toJSON() gives it."`
}

type pushSubscriptionOutput struct {
	Body struct {
		Subscription pushSubscriptionBody `json:"subscription"`
	}
}

type pushSubscriptionsOutput struct {
	Body struct {
		Subscriptions []pushSubscriptionBody `json:"subscriptions" nullable:"false" doc:"The newest first."`
	}
}

type pushSubscriptionInput struct {
	ID string `path:"id" format:"uuid"`
}

type testPushOutput struct {
	Body struct {
		Sent   int `json:"sent" doc:"The devices the push services took the test for."`
		Gone   int `json:"gone" doc:"The devices whose subscriptions are no more, now deleted."`
		Failed int `json:"failed" doc:"The devices whose push services refused it or could not be reached."`
	}
}

// requirePush returns the sender, or 503 on an instance without one.
func (s *sessions) requirePush() (*push.Sender, error) {
	if s.push == nil {
		return nil, huma.Error503ServiceUnavailable("this instance does not send push notifications")
	}
	return s.push, nil
}

func (s *sessions) getPushKey(ctx context.Context, _ *struct{}) (*pushKeyOutput, error) {
	if _, err := requireSession(ctx); err != nil {
		return nil, err
	}
	sender, err := s.requirePush()
	if err != nil {
		return nil, err
	}
	out := &pushKeyOutput{}
	out.Body.PublicKey = sender.PublicKey()
	return out, nil
}

// savePushSubscription stores this device's subscription, by its
// endpoint: the same device again is the same subscription, and one
// another user subscribed on becomes the user's.
func (s *sessions) savePushSubscription(ctx context.Context, in *savePushInput) (*pushSubscriptionOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requirePush(); err != nil {
		return nil, err
	}
	b := in.Body
	sub := push.Subscription{Endpoint: b.Endpoint, P256dh: b.Keys.P256dh, Auth: b.Keys.Auth}
	if err := sub.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity(strings.TrimPrefix(err.Error(), push.ErrInvalidSubscription.Error()+": "))
	}
	saved, err := s.db.SavePushSubscription(ctx, p.User.ID, store.PushSubscription{
		ID: uuid.NewV7(), Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth, Device: deviceName(in.UserAgent),
	}, s.now())
	if err != nil {
		return nil, internalError("saving the push subscription failed", err)
	}
	out := &pushSubscriptionOutput{}
	out.Body.Subscription = newPushSubscriptionBody(saved)
	return out, nil
}

func (s *sessions) listPushSubscriptions(ctx context.Context, _ *struct{}) (*pushSubscriptionsOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	subs, err := s.db.PushSubscriptions(ctx, p.User.ID)
	if err != nil {
		return nil, internalError("reading the push subscriptions failed", err)
	}
	out := &pushSubscriptionsOutput{}
	out.Body.Subscriptions = make([]pushSubscriptionBody, 0, len(subs))
	for i := range subs {
		out.Body.Subscriptions = append(out.Body.Subscriptions, newPushSubscriptionBody(subs[i]))
	}
	return out, nil
}

func (s *sessions) deletePushSubscription(ctx context.Context, in *pushSubscriptionInput) (*struct{}, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("the id is not a UUID")
	}
	ok, err := s.db.DeletePushSubscription(ctx, p.User.ID, id)
	if err != nil {
		return nil, internalError("deleting the push subscription failed", err)
	}
	if !ok {
		return nil, huma.Error404NotFound("no device of yours has that subscription")
	}
	return &struct{}{}, nil
}

// testPush sends a test notification to every device of the user's.
func (s *sessions) testPush(ctx context.Context, _ *struct{}) (*testPushOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	sender, err := s.requirePush()
	if err != nil {
		return nil, err
	}
	words := testPushWords[p.User.Locale]
	if words.Title == "" {
		words = testPushWords["en"]
	}
	words.URL, words.Tag = "/settings/notifications", "test"
	r := notify.Push(ctx, s.db, sender, p.User, words, push.Message{TTL: testPushTTL, Urgency: push.UrgencyHigh}, s.now())
	if r.Err != nil {
		if r.Sent+r.Gone+r.Failed == 0 {
			return nil, internalError("sending the test failed", r.Err)
		}
		s.logger.WarnContext(ctx, "a test push failed", "user_id", p.User.ID.String(), "error", r.Err)
	}
	out := &testPushOutput{}
	out.Body.Sent, out.Body.Gone, out.Body.Failed = r.Sent, r.Gone, r.Failed
	return out, nil
}

// testPushWords are the test's title and body in each language.
var testPushWords = map[string]push.Notification{
	"en": {Title: "tdeemon", Body: "Notifications work on this device."},
	"hr": {Title: "tdeemon", Body: "Obavijesti rade na ovom uređaju."},
}

// deviceName names a device by its browser's user agent, as the list of
// devices shows it: "iPhone · Safari", "Mac · Chrome"; what it cannot
// tell it leaves out.
func deviceName(ua string) string {
	var device, browser string
	for _, d := range []struct{ in, name string }{
		{"iPhone", "iPhone"},
		{"iPad", "iPad"},
		{"Android", "Android"},
		{"Macintosh", "Mac"},
		{"Windows", "Windows"},
		{"CrOS", "ChromeOS"},
		{"Linux", "Linux"},
	} {
		if strings.Contains(ua, d.in) {
			device = d.name
			break
		}
	}
	// The order matters: Edge's and Opera's agents name Chrome, Chrome's
	// names Safari, and every browser on iOS is Safari underneath.
	for _, b := range []struct{ in, name string }{
		{"Edg", "Edge"},
		{"OPR/", "Opera"},
		{"SamsungBrowser", "Samsung Internet"},
		{"Firefox/", "Firefox"},
		{"FxiOS", "Firefox"},
		{"CriOS", "Chrome"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
	} {
		if strings.Contains(ua, b.in) {
			browser = b.name
			break
		}
	}
	switch {
	case device != "" && browser != "":
		return device + " · " + browser
	case device != "" || browser != "":
		return device + browser
	}
	return "Unknown device"
}

func (s *sessions) registerPush(a huma.API) {
	errs := []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable}
	huma.Register(a, huma.Operation{
		OperationID: "get-push-key",
		Method:      http.MethodGet,
		Path:        Prefix + "/push/key",
		Summary:     "Get the push key",
		Description: "Needs a web session. The VAPID public key a browser subscribes to the instance's notifications with.",
		Tags:        []string{"push"},
		Errors:      errs,
	}, s.getPushKey)
	huma.Register(a, huma.Operation{
		OperationID: "save-push-subscription",
		Method:      http.MethodPut,
		Path:        Prefix + "/push/subscriptions",
		Summary:     "Subscribe this device to notifications",
		Description: "Needs a web session. Stores the browser's push subscription, by its endpoint: the same one again is the same " +
			"subscription, with its keys renewed. The device is named by the request's User-Agent.",
		Tags:   []string{"push"},
		Errors: append(errs, http.StatusUnprocessableEntity),
	}, s.savePushSubscription)
	huma.Register(a, huma.Operation{
		OperationID: "list-push-subscriptions",
		Method:      http.MethodGet,
		Path:        Prefix + "/push/subscriptions",
		Summary:     "List the devices notifications go to",
		Description: "Needs a web session.",
		Tags:        []string{"push"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.listPushSubscriptions)
	huma.Register(a, huma.Operation{
		OperationID:   "delete-push-subscription",
		Method:        http.MethodDelete,
		Path:          Prefix + "/push/subscriptions/{id}",
		Summary:       "Stop the notifications to a device",
		Description:   "Needs a web session.",
		Tags:          []string{"push"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity},
	}, s.deletePushSubscription)
	huma.Register(a, huma.Operation{
		OperationID: "test-push",
		Method:      http.MethodPost,
		Path:        Prefix + "/push/test",
		Summary:     "Send a test notification",
		Description: "Needs a web session. Sends a test notification to every device of yours, and says how many took it; " +
			"a device whose subscription is no more is deleted.",
		Tags:   []string{"push"},
		Errors: errs,
	}, s.testPush)
}
