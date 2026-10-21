package store_test

import "github.com/zarnautovic/tdeemon/internal/domain/event"

// testActor is the user writing through the web app, for the tests that
// do not look at who wrote.
var testActor = event.Actor{Kind: event.User, Client: "web"}
