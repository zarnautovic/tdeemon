package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/domain/profile"
	"github.com/zarnautovic/tdeemon/internal/store"
)

type settingsInput struct {
	Body struct {
		Name         *string `json:"name,omitempty" maxLength:"100" doc:"The name the app shows."`
		Locale       *string `json:"locale,omitempty" enum:"en,hr" doc:"The language of the app."`
		TimeZone     *string `json:"timezone,omitempty" example:"Europe/Zagreb" doc:"An IANA time zone."`
		DayStartHour *int    `json:"day_start_hour,omitempty" minimum:"0" maximum:"23" doc:"The hour at which the day starts; a meal before it belongs to the day before."`
	}
}

type settingsOutput struct {
	Body struct {
		User userBody `json:"user" doc:"The user as they stand now."`
	}
}

// setSettings changes the settings the body gives, and only those, on
// the user as they are when the change is made.
func (s *sessions) setSettings(ctx context.Context, in *settingsInput) (*settingsOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	b := in.Body
	u, err := s.db.ChangeSettings(ctx, p.User.ID, store.SettingsChange{
		Name: b.Name, Locale: b.Locale, TimeZone: b.TimeZone, DayStartHour: b.DayStartHour,
	}, s.now())
	switch {
	case errors.Is(err, store.ErrInvalidSettings):
		return nil, huma.Error422UnprocessableEntity(
			strings.ReplaceAll(strings.TrimPrefix(err.Error(), store.ErrInvalidSettings.Error()+": "), "\n", "; "))
	case err != nil:
		return nil, internalError("saving the settings failed", err)
	}
	// A time zone or a day's start changed moves the days to come.
	s.logger.InfoContext(ctx, "changed the settings", "user_id", u.ID.String(), "timezone", u.TimeZone, "day_start_hour", u.DayStartHour)
	out := &settingsOutput{}
	out.Body.User = newUserBody(u)
	return out, nil
}

type notesInput struct {
	Body struct {
		Notes string `json:"notes" maxLength:"4000" doc:"What a coach should know of the user, in their words; empty deletes them."`
	}
}

type notesOutput struct {
	Body struct {
		Notes string `json:"notes"`
	}
}

// setNotes stores the user's notes for a coach, which get_me gives a
// model; a change of them is an event, which undo takes back.
func (s *sessions) setNotes(ctx context.Context, in *notesInput) (*notesOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	if err := profile.ValidateNotes(in.Body.Notes); err != nil {
		return nil, huma.Error422UnprocessableEntity(oneLine(err))
	}
	if err := s.db.SetProfileNotes(ctx, p.User.ID, webActor, in.Body.Notes, s.now()); err != nil {
		return nil, internalError("saving the notes failed", err)
	}
	out := &notesOutput{}
	out.Body.Notes = in.Body.Notes
	return out, nil
}

type revokeTokensOutput struct {
	Body struct {
		Revoked int `json:"revoked" doc:"How many tokens there were, which no longer work."`
	}
}

// revokeTokens revokes every token of the user's at once.
func (s *sessions) revokeTokens(ctx context.Context, _ *struct{}) (*revokeTokensOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.db.RevokeAPITokens(ctx, p.User.ID)
	if err != nil {
		return nil, internalError("revoking the tokens failed", err)
	}
	s.logger.InfoContext(ctx, "revoked every API token", "user_id", p.User.ID.String(), "revoked", n)
	out := &revokeTokensOutput{}
	out.Body.Revoked = n
	return out, nil
}

func (s *sessions) registerSettings(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "set-settings",
		Method:      http.MethodPatch,
		Path:        Prefix + "/me",
		Summary:     "Change your settings",
		Description: "Needs a web session. Changes the name, the language, the time zone and the hour the day starts, those given only. " +
			"What was logged before keeps the day it was logged on.",
		Tags:   []string{"settings"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, s.setSettings)
	huma.Register(a, huma.Operation{
		OperationID: "set-notes",
		Method:      http.MethodPut,
		Path:        Prefix + "/me/notes",
		Summary:     "Set your notes for a coach",
		Description: "Needs a web session. What a coach, a model, should know of you, which get_me gives it; empty deletes them. " +
			"Undo takes a change of them back.",
		Tags:   []string{"settings"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, s.setNotes)
	huma.Register(a, huma.Operation{
		OperationID: "revoke-tokens",
		Method:      http.MethodDelete,
		Path:        Prefix + "/tokens",
		Summary:     "Revoke all your API tokens",
		Description: "Needs a web session. Every token stops working at once: after a password reset, the devices and clients need new ones.",
		Tags:        []string{"tokens"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.revokeTokens)
}
