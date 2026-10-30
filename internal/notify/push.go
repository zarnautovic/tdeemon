package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// Pushed is what Push did with each of a user's subscriptions.
type Pushed struct {
	// Sent took the message.
	Sent int
	// Gone are no more, and were deleted.
	Gone int
	// Failed refused it, or could not be reached; Err says why.
	Failed int
	Err    error
}

// Push sends n to every push subscription of u's through sender, with
// m's TTL, urgency and topic: a subscription the push service no longer
// has is deleted, one that took it is marked sent at now. Err joins the
// failures of the others, and of the database.
func Push(ctx context.Context, db *store.DB, sender *push.Sender, u user.User, n push.Notification, m push.Message, now time.Time) Pushed {
	subs, err := db.PushSubscriptions(ctx, u.ID)
	if err != nil {
		return Pushed{Err: err}
	}
	if m.Payload, err = json.Marshal(n); err != nil {
		return Pushed{Err: fmt.Errorf("push payload: %w", err)}
	}
	var out Pushed
	var errs []error
	for i := range subs {
		s := &subs[i]
		err := sender.Send(ctx, push.Subscription{Endpoint: s.Endpoint, P256dh: s.P256dh, Auth: s.Auth}, m)
		switch {
		case err == nil:
			out.Sent++
			if err := db.MarkPushSent(ctx, u.ID, s.ID, now); err != nil {
				errs = append(errs, err)
			}
		case errors.Is(err, push.ErrGone), errors.Is(err, push.ErrInvalidSubscription):
			out.Gone++
			if _, err := db.DeletePushSubscription(ctx, u.ID, s.ID); err != nil {
				errs = append(errs, err)
			}
		default:
			out.Failed++
			errs = append(errs, fmt.Errorf("push to %s: %w", s.Device, err))
		}
	}
	out.Err = errors.Join(errs...)
	return out
}
