package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// set-settings changes the settings given, and only those; what the
// session says after is the user as changed.
func TestSetSettings(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	rec := in.do(t, http.MethodPatch, "/api/v1/me", `{"name": "Ana \u200dM.", "timezone": "Europe/Zagreb"}`, cookies, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a joiner by a space: %d %s, want 422", rec.Code, rec.Body.String())
	}
	// A name with an emoji of a family, its joiners between its people.
	rec = in.do(t, http.MethodPatch, "/api/v1/me", "{\"name\": \"Ana \U0001f468\u200d\U0001f469\", \"timezone\": \"Europe/Zagreb\", \"day_start_hour\": 3}", cookies, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		User struct {
			Name         string `json:"name"`
			Locale       string `json:"locale"`
			TimeZone     string `json:"timezone"`
			DayStartHour int    `json:"day_start_hour"`
		} `json:"user"`
	}
	session2 := in.do(t, http.MethodGet, "/api/v1/auth/session", "", cookies, nil)
	if err := json.Unmarshal(session2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.User.Name != "Ana \U0001f468\u200d\U0001f469" || got.User.TimeZone != "Europe/Zagreb" || got.User.DayStartHour != 3 || got.User.Locale != in.user.Locale {
		t.Errorf("the session after = %+v, want the name, zone and hour changed, the language as it was", got.User)
	}
	for name, body := range map[string]string{
		"a zone no one has": `{"timezone": "Mars/Olympus"}`,
		"the host's zone":   `{"timezone": "Local"}`,
		"no name":           `{"name": " "}`,
		"another language":  `{"locale": "de"}`,
		"an hour of 24":     `{"day_start_hour": 24}`,
	} {
		if rec := in.do(t, http.MethodPatch, "/api/v1/me", body, cookies, nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %s, want 422", name, rec.Code, rec.Body.String())
		}
	}
	tok, created := in.createToken(t, session, `{"name":"phone","scopes":["read","tools"]}`)
	if created.Code != http.StatusCreated {
		t.Fatal(created.Body.String())
	}
	if rec := in.do(t, http.MethodPatch, "/api/v1/me", `{"name": "Bot"}`, nil, bearer(tok.Token)); rec.Code != http.StatusForbidden {
		t.Errorf("with a token: %d, want 403", rec.Code)
	}
}

// A refusal of settings gives each reason, in one line, without the
// store's words around them.
func TestSetSettingsRefusal(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	rec := in.do(t, http.MethodPatch, "/api/v1/me", `{"name": "\u202e", "timezone": "Mars/Olympus"}`, cookies, nil)
	var problem struct{ Detail string }
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(problem.Detail, `name "`) || !strings.Contains(problem.Detail, "; time zone ") || strings.Contains(problem.Detail, "\n") {
		t.Errorf("two reasons: %d %q, want both in one line", rec.Code, problem.Detail)
	}
}

// set-notes stores the notes for a coach, which get_me gives, and undo
// takes them back.
func TestSetNotes(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	if rec := in.do(t, http.MethodPut, "/api/v1/me/notes", `{"notes": "Knees: no running.\nLactose."}`, cookies, nil); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if me := in.do(t, http.MethodGet, "/api/v1/me", "", cookies, nil); !strings.Contains(me.Body.String(), `"notes":"Knees: no running.\nLactose."`) {
		t.Errorf("get_me after: %s", me.Body.String())
	}
	if rec := in.do(t, http.MethodPost, "/api/v1/undo", "{}", cookies, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "notes.set") {
		t.Errorf("the undo: %d %s", rec.Code, rec.Body.String())
	}
	if me := in.do(t, http.MethodGet, "/api/v1/me", "", cookies, nil); !strings.Contains(me.Body.String(), `"notes":""`) {
		t.Errorf("get_me after the undo: %s", me.Body.String())
	}
	if rec := in.do(t, http.MethodPut, "/api/v1/me/notes", `{"notes": "`+strings.Repeat("a", 4001)+`"}`, cookies, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("notes too long: %d, want 422", rec.Code)
	}
}

// revoke-tokens revokes every token of the user's at once.
func TestRevokeTokens(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	var tokens []string
	for _, name := range []string{"phone", "laptop"} {
		tok, rec := in.createToken(t, session, `{"name":"`+name+`","scopes":["read"]}`)
		if rec.Code != http.StatusCreated {
			t.Fatal(rec.Body.String())
		}
		tokens = append(tokens, tok.Token)
	}
	rec := in.do(t, http.MethodDelete, "/api/v1/tokens", "", []*http.Cookie{session}, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"revoked":2`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for _, tok := range tokens {
		if rec := in.do(t, http.MethodGet, "/api/v1/auth/whoami", "", nil, bearer(tok)); rec.Code != http.StatusUnauthorized {
			t.Errorf("a revoked token: %d, want 401", rec.Code)
		}
	}
	if rec := in.do(t, http.MethodDelete, "/api/v1/tokens", "", nil, bearer(tokens[0])); rec.Code != http.StatusUnauthorized {
		t.Errorf("with a revoked token: %d, want 401", rec.Code)
	}
}
