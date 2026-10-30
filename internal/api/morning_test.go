package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/zarnautovic/tdeemon/internal/api"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/metric"
	"github.com/zarnautovic/tdeemon/internal/domain/proposal"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/push"
	"github.com/zarnautovic/tdeemon/internal/store"
	"github.com/zarnautovic/tdeemon/internal/store/storetest"
	"github.com/zarnautovic/tdeemon/internal/telegram"
)

// The morning summary job is due at 10:00 on the user's clock, and sends
// a user with a Telegram chat the summary of last night and yesterday;
// a user without one gets nothing.
func TestMorningSummaryJob(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var sent []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		sent = append(sent, body)
		mu.Unlock()
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	db := storetest.Open(t)
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 10, 23, 10, 5, 0, 0, zagreb)
	newU := func(email string) user.User {
		u := user.User{
			ID: uuid.NewV7(), Email: email, Name: "Ana", Role: user.RoleUser, Locale: "hr",
			TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
		}
		if err := db.CreateUser(t.Context(), u, ""); err != nil {
			t.Fatal(err)
		}
		return u
	}
	ana, ivo := newU("ana@example.com"), newU("ivo@example.com")
	if err := db.SetTelegramChat(t.Context(), ana.ID, 853, now); err != nil {
		t.Fatal(err)
	}
	today := day.Date{Year: 2026, Month: 10, Day: 23}
	if _, err := db.MergeMetrics(t.Context(), ana.ID, isolationActor, today, []metric.Value{
		{Kind: metric.Sleep, Value: 7.5, Source: metric.Import}, {Kind: metric.Weight, Value: 81.2, Source: metric.Import},
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MergeMetrics(t.Context(), ana.ID, isolationActor, today.AddDays(-1), []metric.Value{
		{Kind: metric.Sleep, Value: 4, Source: metric.Import}, {Kind: metric.Steps, Value: 9100, Source: metric.Import},
	}, now); err != nil {
		t.Fatal(err)
	}

	waitingProposal(t, db, ana.ID, today, now)

	job := api.MorningSummaryJob(api.Config{DB: db, Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return now }},
		&telegram.Bot{Token: "123456:made-up-token-for-tests-only-0123", BaseURL: srv.URL})
	if _, due := job.Due(ana, now.Add(-10*time.Minute)); due {
		t.Error("due before 10:00")
	}
	occurrence, due := job.Due(ana, now)
	if !due || occurrence != "2026-10-23" {
		t.Fatalf("Due at 10:05 = %q, %v", occurrence, due)
	}
	for _, u := range []user.User{ana, ivo} {
		if err := job.Run(t.Context(), u, occurrence, now); err != nil {
			t.Fatalf("Run for %s: %v", u.Email, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || sent[0]["chat_id"] != float64(853) {
		t.Fatalf("sent = %v, want one message to Ana's chat", sent)
	}
	text, _ := sent[0]["text"].(string)
	for _, want := range []string{"Jutarnji sažetak, petak 23. 10.", "San: 7 h 30 min", "Težina: 81,2 kg", "Jučer: ništa upisano", "koraci 9100", "Kalibracija: nema plana", "Plan: prijedlog promjene čeka tvoj odgovor, do 30. 10."} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	if strings.Contains(text, "4 h") {
		t.Errorf("yesterday's sleep in\n%s", text)
	}
}

// waitingProposal gives userID a proposal that waits for their answer,
// until a week after now.
func waitingProposal(t *testing.T, db *store.DB, userID uuid.UUID, today day.Date, now time.Time) {
	t.Helper()
	if _, err := db.CreateProposal(t.Context(), userID, isolationActor, proposal.Draft{
		Rationale: "Train on Sundays too.", Changes: proposal.Changes{TrainingWeekdays: &proposal.WeekdaysChange{After: 1}},
	}, today, now, nil); err != nil {
		t.Fatal(err)
	}
}

// For a day that starts after 10:00, last night is still of the calendar
// date: the summary shows tonight's sleep and today's date, not those of
// the logical day; and a chat that blocked the bot is no error to retry.
func TestMorningSummaryJobLateDay(t *testing.T) {
	t.Parallel()

	status := http.StatusOK
	var mu sync.Mutex
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		text, _ := body["text"].(string)
		mu.Lock()
		defer mu.Unlock()
		texts = append(texts, text)
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`)
	}))
	t.Cleanup(srv.Close)

	db := storetest.Open(t)
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 10, 23, 10, 5, 0, 0, zagreb)
	ana := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser, Locale: "en",
		TimeZone: "Europe/Zagreb", DayStartHour: 12, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTelegramChat(t.Context(), ana.ID, 853, now); err != nil {
		t.Fatal(err)
	}
	today := day.Date{Year: 2026, Month: 10, Day: 23}
	for d, h := range map[day.Date]float64{today: 7.5, today.AddDays(-1): 4} {
		if _, err := db.MergeMetrics(t.Context(), ana.ID, isolationActor, d, []metric.Value{{Kind: metric.Sleep, Value: h, Source: metric.Import}}, now); err != nil {
			t.Fatal(err)
		}
	}
	job := api.MorningSummaryJob(api.Config{DB: db}, &telegram.Bot{Token: "123456:made-up-token-for-tests-only-0123", BaseURL: srv.URL})
	if err := job.Run(t.Context(), ana, "2026-10-22", now); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	text := texts[0]
	status = http.StatusForbidden
	mu.Unlock()
	if !strings.Contains(text, "Morning summary, Friday Oct 23") || !strings.Contains(text, "Sleep: 7 h 30 min") {
		t.Errorf("text:\n%s", text)
	}
	if err := job.Run(t.Context(), ana, "2026-10-22", now); err != nil {
		t.Errorf("a chat that blocked the bot: %v", err)
	}
}

// A user with a device and no chat gets the morning summary by push, with
// no bot at all.
func TestMorningSummaryJobByPush(t *testing.T) {
	t.Parallel()

	db := storetest.Open(t)
	zagreb, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 10, 23, 10, 5, 0, 0, zagreb)
	ana := user.User{
		ID: uuid.NewV7(), Email: "ana@example.com", Name: "Ana", Role: user.RoleUser, Locale: "hr",
		TimeZone: "Europe/Zagreb", DayStartHour: 4, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateUser(t.Context(), ana, ""); err != nil {
		t.Fatal(err)
	}
	p256dh, secret := browserKeys(t)
	if _, err := db.SavePushSubscription(t.Context(), ana.ID, store.PushSubscription{
		ID: uuid.NewV7(), Endpoint: "https://web.push.apple.com/phone", P256dh: p256dh, Auth: secret, Device: "iPhone · Safari",
	}, now); err != nil {
		t.Fatal(err)
	}
	sender, err := push.NewSender([]byte("a test secret of thirty-two bytes!"), "https://tdeemon.example.com")
	if err != nil {
		t.Fatal(err)
	}
	services := &pushServices{}
	sender.Client = &http.Client{Transport: services}
	job := api.MorningSummaryJob(api.Config{DB: db, Logger: slog.New(slog.DiscardHandler), Push: sender}, nil)
	if err := job.Run(t.Context(), ana, "2026-10-23", now); err != nil {
		t.Fatal(err)
	}
	if got := services.endpoints(); len(got) != 1 || got[0] != "https://web.push.apple.com/phone" {
		t.Errorf("pushed to %v, want Ana's phone", got)
	}
}

type morningAnswer struct {
	Date, Title, Text, Short string
}

func getMorning(t *testing.T, in *instance, session *http.Cookie, query string) morningAnswer {
	t.Helper()
	var got morningAnswer
	rec := in.do(t, http.MethodGet, "/api/v1/morning"+query, "", []*http.Cookie{session}, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("the morning%s: %d %s", query, rec.Code, rec.Body.String())
	}
	return got
}

// The morning summary of today reads now; one of before, that morning;
// one to come is refused.
func TestGetMorning(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	now := in.clock.Now()
	today := day.DateOf(now)
	if _, err := in.db.MergeMetrics(t.Context(), in.user.ID, isolationActor, today, []metric.Value{
		{Kind: metric.Weight, Value: 81.2, Source: metric.Import},
	}, now); err != nil {
		t.Fatal(err)
	}
	got := getMorning(t, in, session, "")
	if got.Date != today.String() || got.Title != "Jutarnji sažetak" || !strings.Contains(got.Text, "Težina: 81,2 kg") ||
		got.Short != "Jučer: ništa upisano · 81,2 kg" {
		t.Errorf("today: %+v", got)
	}
	before := today.AddDays(-3)
	if got = getMorning(t, in, session, "?date="+before.String()); got.Date != before.String() || strings.Contains(got.Text, "81,2") {
		t.Errorf("3 days before: %+v", got)
	}
	for _, q := range []string{today.AddDays(1).String(), "2026-13-01", "yesterday"} {
		if rec := in.do(t, http.MethodGet, "/api/v1/morning?date="+q, "", cookies, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, want 422", q, rec.Code)
		}
	}
	tok, created := in.createToken(t, session, `{"name":"phone","scopes":["read"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	if rec := in.do(t, http.MethodGet, "/api/v1/morning", "", nil, bearer(tok.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with a token: %d, want 403", rec.Code)
	}
}

// A morning of before is its own date, across a change of the clocks and
// with a day that starts after 10:00; it says nothing of a proposal,
// whose state then is not kept, while today's does.
func TestGetMorningOfBefore(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	// Zagreb's clocks went back on Oct 25.
	in.clock.Set(time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC))
	now := in.clock.Now()
	if rec := in.do(t, http.MethodPatch, "/api/v1/me", `{"day_start_hour": 12}`, []*http.Cookie{session}, nil); rec.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body.String())
	}
	waitingProposal(t, in.db, in.user.ID, day.DateOf(now), now)
	if got := getMorning(t, in, session, ""); !strings.Contains(got.Short, "promjena plana čeka") {
		t.Errorf("today's says no proposal: %+v", got)
	}
	for _, d := range []string{"2026-10-24", "2026-10-25", "2026-10-26"} {
		got := getMorning(t, in, session, "?date="+d)
		if got.Date != d || strings.Contains(got.Short, "promjena plana") || strings.Contains(got.Text, "Plan:") {
			t.Errorf("%s: %+v", d, got)
		}
	}
}
