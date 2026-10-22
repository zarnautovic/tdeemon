package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/ingest"
)

// maxIngestBytes bounds the body of an ingest: a sync, or a larger export
// split in the app.
const maxIngestBytes = 10 << 20

type ingestHAEInput struct {
	RawBody []byte `contentType:"application/json"`
}

type ingestMetricsInput struct {
	Body struct {
		Date    string             `json:"date" format:"date" example:"2026-10-21" doc:"The day the values belong to."`
		Metrics map[string]float64 `json:"metrics" doc:"Values by metric name (docs: DESIGN.md §4): weight (kg), body_fat (%), lean_mass (kg), steps, active_kcal, basal_kcal, sleep_h (hours), rhr (bpm), hrv (ms), vo2max, exercise_min."`
	}
}

type ingestChange struct {
	Metric string   `json:"metric"`
	Value  float64  `json:"value"`
	Before *float64 `json:"before,omitempty" doc:"The day's value before, of an import; absent when the day had none."`
}

type ingestDay struct {
	Date    string         `json:"date" format:"date"`
	Changed []ingestChange `json:"changed" nullable:"false" doc:"The values the day keeps now; a value the day held already, or one a manual value or the day's first weighing keeps out, is not here."`
}

type ingestRejected struct {
	Date    string  `json:"date,omitempty" format:"date" doc:"Absent when the date itself did not read, or the whole metric was left out."`
	RawDate string  `json:"raw_date,omitempty" doc:"The date as the source wrote it, when it did not read."`
	Metric  string  `json:"metric"`
	Value   float64 `json:"value"`
	Reason  string  `json:"reason"`
}

type ingestBody struct {
	Days     []ingestDay      `json:"days" nullable:"false"`
	Rejected []ingestRejected `json:"rejected" nullable:"false" doc:"Values out of their bounds, left out; the others are stored."`
	Ignored  []string         `json:"ignored" nullable:"false" doc:"Metric names tdeemon does not read."`
}

type ingestOutput struct {
	Body ingestBody
}

func (s *sessions) registerIngest(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "ingest-hae",
		Method:      http.MethodPost,
		Path:        Prefix + "/ingest/hae",
		Summary:     "Import a Health Auto Export payload",
		Description: "For the REST API automation of Health Auto Export: an API token with the ingest scope. Each value belongs to the calendar date of its point; " +
			"the merge keeps a manual value, the day's first weighing and the larger of a running total (DESIGN.md §4). The raw body is archived for 30 days.",
		Tags:         []string{"ingest"},
		MaxBodyBytes: maxIngestBytes,
		// HAE's JSON, which ParseHAE reads: huma would take a raw body for
		// a string.
		SkipValidateBody: true,
		Errors:           []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity},
		Metadata:         openToTokens(auth.ScopeIngest),
	}, s.ingestHAE)
	huma.Register(a, huma.Operation{
		OperationID:  "ingest-metrics",
		Method:       http.MethodPost,
		Path:         Prefix + "/ingest/metrics",
		Summary:      "Import a day's metrics",
		Description:  "The simple form, for a script or another app: an API token with the ingest scope. The values merge as an import, as Health Auto Export's do.",
		Tags:         []string{"ingest"},
		MaxBodyBytes: maxIngestBytes,
		Errors:       []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity},
		Metadata:     openToTokens(auth.ScopeIngest),
	}, s.ingestMetrics)
}

func (s *sessions) ingestHAE(ctx context.Context, in *ingestHAEInput) (*ingestOutput, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("not signed in")
	}
	s.archive(ctx, p, "hae", in.RawBody)
	r, err := ingest.ParseHAE(in.RawBody)
	if errors.Is(err, ingest.ErrNotHAE) {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	} else if err != nil {
		return nil, huma.Error422UnprocessableEntity("the body does not read as a Health Auto Export payload")
	}
	return s.store(ctx, p, "hae", r)
}

func (s *sessions) ingestMetrics(ctx context.Context, in *ingestMetricsInput) (*ingestOutput, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil, huma.Error401Unauthorized("not signed in")
	}
	// A date and a map of numbers always marshal.
	raw, _ := json.Marshal(in.Body)
	s.archive(ctx, p, "metrics", raw)
	r, err := ingest.FromDays(map[string]map[string]float64{in.Body.Date: in.Body.Metrics})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	return s.store(ctx, p, "ingest", r)
}

// archive keeps body in the user's archive, when the instance has one.
// A failure is logged and does not stop the import: the values matter
// more than the copy.
func (s *sessions) archive(ctx context.Context, p auth.Principal, source string, body []byte) {
	if s.ingestArchive == nil {
		return
	}
	if err := s.ingestArchive.Save(p.User.ID, source, body); err != nil {
		s.logger.ErrorContext(ctx, "archiving an ingest failed", "error", err)
	}
}

// store merges r's days into the user's, as an import of client's, and
// says what changed. A day stored before a later day fails stays: a retry
// of the whole body changes nothing more for it.
func (s *sessions) store(ctx context.Context, p auth.Principal, client string, r ingest.Result) (*ingestOutput, error) {
	a := event.Actor{Kind: event.Import, Client: client}
	out := &ingestOutput{Body: ingestBody{Days: []ingestDay{}, Rejected: []ingestRejected{}, Ignored: []string{}}}
	for _, d := range r.Days {
		changes, err := s.db.MergeMetrics(ctx, p.User.ID, a, d.Date, d.Values, s.now())
		if err != nil {
			s.logger.ErrorContext(ctx, "importing metrics failed", "error", err, "day", d.Date.String())
			return nil, huma.Error500InternalServerError("importing the metrics failed")
		}
		day := ingestDay{Date: d.Date.String(), Changed: []ingestChange{}}
		for _, c := range changes {
			ch := ingestChange{Metric: string(c.After.Kind), Value: c.After.Value}
			if c.Before != nil {
				ch.Before = &c.Before.Value
			}
			day.Changed = append(day.Changed, ch)
		}
		out.Body.Days = append(out.Body.Days, day)
	}
	for _, x := range r.Rejected {
		rej := ingestRejected{RawDate: x.RawDate, Metric: string(x.Kind), Value: x.Value, Reason: x.Reason}
		if !x.Date.IsZero() {
			rej.Date = x.Date.String()
		}
		out.Body.Rejected = append(out.Body.Rejected, rej)
	}
	out.Body.Ignored = append(out.Body.Ignored, r.Ignored...)
	s.logger.InfoContext(ctx, "imported metrics", "user_id", p.User.ID.String(), "client", client,
		"days", len(r.Days), "rejected", len(r.Rejected), "ignored", len(r.Ignored))
	return out, nil
}
