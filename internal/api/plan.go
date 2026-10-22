package api

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/domain/limits"
	"github.com/zarnautovic/tdeemon/internal/domain/profile"
)

// webActor is who writes through these operations: the user, in a web
// session.
var webActor = event.Actor{Kind: event.User, Client: "web"}

// ProfileFields are the profile's fields, as a request sets them and an
// answer shows them. It is exported so profileBody can embed it: huma
// describes no field of an unexported embedded struct.
type ProfileFields struct {
	Sex       *string  `json:"sex,omitempty" enum:"male,female" doc:"The sex the energy equations take."`
	BirthDate *string  `json:"birth_date,omitempty" format:"date" example:"1990-06-15"`
	HeightCm  *float64 `json:"height_cm,omitempty" minimum:"50" maximum:"275" doc:"The height in centimetres."`
}

// profileBody is ProfileFields with what a plan still needs.
type profileBody struct {
	ProfileFields
	Missing []string `json:"missing" nullable:"false" enum:"sex,birth_date,height_cm" doc:"What a plan still needs: sex, birth_date, height_cm. A plan needs the whole profile; logging needs none of it."`
}

type profileOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         profileBody
}

type setProfileInput struct {
	Body ProfileFields
}

type limitsBody struct {
	Limits    map[string]float64 `json:"limits" doc:"The user's limits, by name (docs/limits.md, \"The names of the limits\")."`
	Instance  map[string]float64 `json:"instance" doc:"The instance's limits: no user's goes past them."`
	Effective map[string]float64 `json:"effective" doc:"The limits that hold: for each, the stricter of the user's and the instance's."`
	Defaults  map[string]float64 `json:"defaults" doc:"A user's limits at the start."`
	// LooserBelow lets a client tell a loosening, which takes a
	// confirmation, from a tightening.
	LooserBelow []string `json:"looser_below" nullable:"false" doc:"The limits a lower value loosens (floors, the fastest loss, the waits); a higher value loosens the others."`
}

type limitsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         limitsBody
}

type setLimitsInput struct {
	Body struct {
		Limits  map[string]float64 `json:"limits" doc:"The limits to change, by name; those not named stay as they are."`
		Confirm bool               `json:"confirm,omitempty" doc:"Loosening a limit takes this: the user saw that the change allows a plan the limit refused."`
	}
}

func (s *sessions) registerPlan(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "get-profile",
		Method:      http.MethodGet,
		Path:        Prefix + "/profile",
		Summary:     "Get your profile",
		Description: "Needs a web session. The profile a plan needs: the sex of the energy equations, the birth date and the height.",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.getProfile)
	huma.Register(a, huma.Operation{
		OperationID: "set-profile",
		Method:      http.MethodPut,
		Path:        Prefix + "/profile",
		Summary:     "Set your profile",
		Description: "Needs a web session: the limits of a plan depend on the profile (docs/limits.md), so no API token changes it. A field left out is cleared.",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, s.setProfile)
	huma.Register(a, huma.Operation{
		OperationID: "get-limits",
		Method:      http.MethodGet,
		Path:        Prefix + "/limits",
		Summary:     "Get your limits",
		Description: "Needs a web session. Your limits of a plan, the instance's, the ones that hold and the defaults.",
		Tags:        []string{"plan"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.getLimits)
	huma.Register(a, huma.Operation{
		OperationID: "set-limits",
		Method:      http.MethodPut,
		Path:        Prefix + "/limits",
		Summary:     "Change your limits",
		Description: "Needs a web session, never an API token. A limit may be tightened freely; loosening one takes `confirm`, and goes no further than the instance's (422). " +
			"A limit the instance has tightened since is brought to the instance's.",
		Tags:   []string{"plan"},
		Errors: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity},
	}, s.setLimits)
}

func newProfileBody(p profile.Profile) profileBody {
	var b profileBody
	if p.Sex != "" {
		b.Sex = new(string(p.Sex))
	}
	if !p.BirthDate.IsZero() {
		b.BirthDate = new(p.BirthDate.String())
	}
	if p.HeightCm != 0 {
		b.HeightCm = new(p.HeightCm)
	}
	b.Missing = p.Missing()
	if b.Missing == nil {
		b.Missing = []string{}
	}
	return b
}

func (s *sessions) getProfile(ctx context.Context, _ *struct{}) (*profileOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	pr, err := s.db.Profile(ctx, p.User.ID)
	if err != nil {
		s.logger.ErrorContext(ctx, "reading the profile failed", "error", err)
		return nil, huma.Error500InternalServerError("reading the profile failed")
	}
	return &profileOutput{CacheControl: "no-store", Body: newProfileBody(pr)}, nil
}

// today returns the user's day now, by their calendar.
func (s *sessions) today(p auth.Principal) (day.Date, error) {
	c, err := day.NewCalendar(p.User.TimeZone, p.User.DayStartHour)
	if err != nil {
		return day.Date{}, err
	}
	return c.DayOf(s.now()), nil
}

func (s *sessions) setProfile(ctx context.Context, in *setProfileInput) (*profileOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	var pr profile.Profile
	if in.Body.Sex != nil {
		pr.Sex = profile.Sex(*in.Body.Sex)
	}
	if in.Body.BirthDate != nil {
		if pr.BirthDate, err = day.ParseDate(*in.Body.BirthDate); err != nil {
			return nil, huma.Error422UnprocessableEntity("the birth date: " + err.Error())
		}
	}
	if in.Body.HeightCm != nil {
		pr.HeightCm = *in.Body.HeightCm
	}
	today, err := s.today(p)
	if err != nil {
		s.logger.ErrorContext(ctx, "reading the user's calendar failed", "error", err)
		return nil, huma.Error500InternalServerError("setting the profile failed")
	}
	if err := pr.Validate(today); err != nil {
		return nil, huma.Error422UnprocessableEntity(strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	if err := s.db.SetProfile(ctx, p.User.ID, webActor, pr, today, s.now()); err != nil {
		s.logger.ErrorContext(ctx, "setting the profile failed", "error", err)
		return nil, huma.Error500InternalServerError("setting the profile failed")
	}
	s.logger.InfoContext(ctx, "set the profile", "user_id", p.User.ID.String())
	return &profileOutput{CacheControl: "no-store", Body: newProfileBody(pr)}, nil
}

// limitsMap returns l by the limits' names.
func limitsMap(l limits.Limits, userOnly bool) map[string]float64 {
	out := map[string]float64{}
	for _, f := range limits.Fields() {
		if f.UserOnly && !userOnly {
			continue
		}
		out[f.Name] = f.Get(l)
	}
	return out
}

func (s *sessions) newLimitsBody(user limits.Limits) limitsBody {
	b := limitsBody{
		Limits:    limitsMap(user, true),
		Instance:  limitsMap(s.instance, false),
		Effective: limitsMap(limits.Effective(user, s.instance), true),
		Defaults:  limitsMap(limits.UserDefaults(), true),
	}
	for _, f := range limits.Fields() {
		if f.LooserBelow {
			b.LooserBelow = append(b.LooserBelow, f.Name)
		}
	}
	return b
}

func (s *sessions) getLimits(ctx context.Context, _ *struct{}) (*limitsOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	l, err := s.db.UserLimits(ctx, p.User.ID)
	if err != nil {
		s.logger.ErrorContext(ctx, "reading the limits failed", "error", err)
		return nil, huma.Error500InternalServerError("reading the limits failed")
	}
	return &limitsOutput{CacheControl: "no-store", Body: s.newLimitsBody(l)}, nil
}

func (s *sessions) setLimits(ctx context.Context, in *setLimitsInput) (*limitsOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	// The check runs on the limits as they are when the change is stored:
	// a loosening another tab stored meanwhile needs no confirmation of
	// this one, and this one cannot loosen what the other tightened.
	var refusal error
	var loosened []string
	next, err := s.db.UpdateUserLimits(ctx, p.User.ID, webActor, s.instance, func(current limits.Limits) (limits.Limits, error) {
		next, err := s.changeLimits(current, in.Body.Limits, in.Body.Confirm)
		if err != nil {
			refusal = err
			return current, err
		}
		loosened = limits.Loosened(current, next)
		return next, nil
	}, s.now())
	switch {
	case refusal != nil:
		return nil, refusal
	case err != nil:
		s.logger.ErrorContext(ctx, "changing the limits failed", "error", err)
		return nil, huma.Error500InternalServerError("changing the limits failed")
	}
	s.logger.InfoContext(ctx, "changed the limits", "user_id", p.User.ID.String(), "loosened", loosened)
	return &limitsOutput{CacheControl: "no-store", Body: s.newLimitsBody(next)}, nil
}

// changeLimits returns current with the limits named in values set, or
// the refusal of the change (422 or 409). A value equal to the current one
// is not a change of it, so a client may send back what it read.
func (s *sessions) changeLimits(current limits.Limits, values map[string]float64, confirm bool) (limits.Limits, error) {
	fields := map[string]limits.Field{}
	for _, f := range limits.Fields() {
		fields[f.Name] = f
	}
	next := current
	var unknown, named []string
	for name, v := range values {
		f, ok := fields[name]
		switch {
		case !ok:
			unknown = append(unknown, name)
		case v != f.Get(current):
			f.Set(&next, v)
			named = append(named, name)
		}
	}
	if unknown != nil {
		sort.Strings(unknown)
		return current, huma.Error422UnprocessableEntity("no such limit: " + strings.Join(unknown, ", "))
	}
	if err := next.Validate(); err != nil {
		return current, huma.Error422UnprocessableEntity(strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	// A limit the request changes goes no further than the instance's;
	// one it leaves, which the instance may have tightened since, is
	// brought to the instance's.
	var beyond []string
	for _, name := range limits.Beyond(next, s.instance) {
		if slices.Contains(named, name) {
			beyond = append(beyond, name)
		}
	}
	if beyond != nil {
		sort.Strings(beyond)
		return current, huma.Error422UnprocessableEntity("past the instance's limit: " + strings.Join(beyond, ", "))
	}
	next = limits.Effective(next, s.instance)
	if err := next.Validate(); err != nil {
		return current, huma.Error422UnprocessableEntity("with the instance's limits, which hold over yours: " +
			strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	if loosened := limits.Loosened(current, next); loosened != nil && !confirm {
		return current, huma.Error409Conflict("loosening " + strings.Join(loosened, ", ") + " needs confirm: true")
	}
	return next, nil
}
