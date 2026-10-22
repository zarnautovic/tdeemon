package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/day"
	"github.com/zarnautovic/tdeemon/internal/domain/event"
	"github.com/zarnautovic/tdeemon/internal/ingest"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// maxIngestBytes bounds the body of an ingest: a sync, or a larger export
// split in the app.
const maxIngestBytes = 10 << 20

// ingestReadTimeout is how long HAE's body may take to arrive, on a
// phone's slow upload: longer than the server's own deadline. The simple
// form, a day's values, keeps huma's 5 s.
const ingestReadTimeout = 2 * time.Minute

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

type ingestStrongInput struct {
	RawBody []byte `contentType:"text/csv"`
	// contentType tells a form, as the predecessor's Shortcut sends, from
	// the file itself.
	contentType string
}

// Resolve keeps the request's content type, which the document need not
// name: the body is the export, or a form with it.
func (in *ingestStrongInput) Resolve(ctx huma.Context) []error {
	in.contentType = ctx.Header("Content-Type")
	return nil
}

// strongCSV returns the export in body: the part named file of a form,
// or else body itself.
func strongCSV(contentType string, body []byte) ([]byte, error) {
	// A content type that does not read is no form either.
	media, params, _ := mime.ParseMediaType(contentType)
	if media != "multipart/form-data" {
		return body, nil
	}
	r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("a form without a part named file: send the export as the part file, or as the body")
		}
		if err != nil {
			return nil, fmt.Errorf("the form does not read: %w", err)
		}
		if part.FormName() == "file" {
			b, err := io.ReadAll(part)
			if err != nil {
				return nil, fmt.Errorf("the form does not read: %w", err)
			}
			return b, nil
		}
	}
}

type strongBody struct {
	Workouts int      `json:"workouts" doc:"The workouts in the export."`
	Days     int      `json:"days" doc:"The days with a workout, all of them now."`
	Added    []string `json:"added" nullable:"false" doc:"The days that became days with a workout."`
	Removed  []string `json:"removed" nullable:"false" doc:"The days that no longer are: their workouts are gone from Strong."`
}

type strongOutput struct {
	Body strongBody
}

// haePayloadSchema describes the body of ingest-hae for the document: huma
// gives a raw body a string's schema, and ParseHAE, not huma, reads it.
var haePayloadSchema = &huma.Schema{
	Type:        "object",
	Description: "Health Auto Export's REST API payload, as the app sends it.",
	Required:    []string{"data"},
	Properties: map[string]*huma.Schema{
		"data": {
			Type:     "object",
			Required: []string{"metrics"},
			Properties: map[string]*huma.Schema{
				"metrics": {Type: "array", Items: &huma.Schema{
					Type: "object",
					Properties: map[string]*huma.Schema{
						"name":  {Type: "string", Examples: []any{"step_count"}},
						"units": {Type: "string", Examples: []any{"count"}},
						"data": {Type: "array", Items: &huma.Schema{
							Type: "object", AdditionalProperties: true, Description: "A point: date, and qty or, for sleep, its totals.",
						}},
					},
				}},
			},
		},
	},
}

func (s *sessions) registerIngest(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "ingest-hae",
		Method:      http.MethodPost,
		Path:        Prefix + "/ingest/hae",
		Summary:     "Import a Health Auto Export payload",
		Description: "For the REST API automation of Health Auto Export: an API token with the ingest scope, not a web session. Each value belongs to the calendar date of its point; " +
			"the merge keeps a manual value, the day's first weighing and the larger of a running total (DESIGN.md §4). The body is archived as it came; a user's days older than 30 go with their next body.",
		Tags:            []string{"ingest"},
		MaxBodyBytes:    maxIngestBytes,
		BodyReadTimeout: ingestReadTimeout,
		// HAE's JSON, which ParseHAE reads: huma would take a raw body for
		// a string.
		SkipValidateBody: true,
		Errors:           []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity},
		Metadata:         openToTokens(auth.ScopeIngest),
	}, s.ingestHAE)
	a.OpenAPI().Paths[Prefix+"/ingest/hae"].Post.RequestBody.Content["application/json"].Schema = haePayloadSchema
	huma.Register(a, huma.Operation{
		OperationID:  "ingest-metrics",
		Method:       http.MethodPost,
		Path:         Prefix + "/ingest/metrics",
		Summary:      "Import a day's metrics",
		Description:  "The simple form, for a script or another app: an API token with the ingest scope, not a web session. The values merge as an import, as Health Auto Export's do.",
		Tags:         []string{"ingest"},
		MaxBodyBytes: maxIngestBytes,
		Errors:       []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity},
		Metadata:     openToTokens(auth.ScopeIngest),
	}, s.ingestMetrics)
	huma.Register(a, huma.Operation{
		OperationID: "ingest-strong",
		Method:      http.MethodPost,
		Path:        Prefix + "/ingest/strong",
		Summary:     "Import the days of workouts from a Strong export",
		Description: "Strong's CSV export, the whole history, as the body or as the part file of a form: an API token with the ingest scope, not a web session. " +
			"The day each workout starts in, by the user's day start hour, becomes a day with a workout, which makes it a training day unless the check-in's toggle says otherwise; " +
			"the days of an earlier export that this one lacks are no longer. An export without a workout is refused. " +
			"The body is archived as it came, and the latest export that read is kept for the training module.",
		Tags:            []string{"ingest"},
		MaxBodyBytes:    maxIngestBytes,
		BodyReadTimeout: ingestReadTimeout,
		Errors:          []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity},
		Metadata:        openToTokens(auth.ScopeIngest),
	}, s.ingestStrong)
}

func (s *sessions) ingestStrong(ctx context.Context, in *ingestStrongInput) (*strongOutput, error) {
	p, err := requireToken(ctx)
	if err != nil {
		return nil, err
	}
	s.archive(ctx, p, "strong.csv", in.RawBody)
	export, err := strongCSV(in.contentType, in.RawBody)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	cal, err := day.NewCalendar(p.User.TimeZone, p.User.DayStartHour)
	if err != nil {
		return nil, internalError("importing the workouts failed", fmt.Errorf("the user's calendar: %w", err))
	}
	w, err := ingest.ParseStrong(bytes.NewReader(export), cal)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	// An export holds the whole history: one without a workout would
	// wipe the days, and is more likely a wrong file than a fresh start.
	if w.Count == 0 {
		return nil, huma.Error422UnprocessableEntity("the export has no workout")
	}
	added, removed, err := s.db.SetWorkoutDays(ctx, p.User.ID, event.Actor{Kind: event.Import, Client: "strong"}, store.Strong, w.Days, s.now())
	if err != nil {
		return nil, internalError("importing the workouts failed", err)
	}
	if s.ingestArchive != nil {
		if err := s.ingestArchive.SaveLatest(p.User.ID, "strong.csv", export); err != nil {
			s.logger.ErrorContext(ctx, "keeping the latest export failed", "user_id", p.User.ID.String(), "error", err)
		}
	}
	s.logger.InfoContext(ctx, "imported workouts", "user_id", p.User.ID.String(), "workouts", w.Count, "days", len(w.Days),
		"added", len(added), "removed", len(removed))
	return &strongOutput{Body: strongBody{Workouts: w.Count, Days: len(w.Days), Added: dateStrings(added), Removed: dateStrings(removed)}}, nil
}

func dateStrings(ds []day.Date) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.String()
	}
	return out
}

func (s *sessions) ingestHAE(ctx context.Context, in *ingestHAEInput) (*ingestOutput, error) {
	p, err := requireToken(ctx)
	if err != nil {
		return nil, err
	}
	s.archive(ctx, p, "hae.json", in.RawBody)
	r, err := ingest.ParseHAE(in.RawBody)
	if errors.Is(err, ingest.ErrNotHAE) {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	} else if err != nil {
		return nil, huma.Error422UnprocessableEntity("the body does not read as a Health Auto Export payload")
	}
	return s.store(ctx, p, "hae", r)
}

func (s *sessions) ingestMetrics(ctx context.Context, in *ingestMetricsInput) (*ingestOutput, error) {
	p, err := requireToken(ctx)
	if err != nil {
		return nil, err
	}
	// The body as huma read it: one it refused is not archived. A date and
	// a map of numbers always marshal.
	raw, _ := json.Marshal(in.Body)
	s.archive(ctx, p, "metrics.json", raw)
	r, err := ingest.FromDays(map[string]map[string]float64{in.Body.Date: in.Body.Metrics})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	return s.store(ctx, p, "ingest", r)
}

// archive keeps body in the user's archive under name, its source and
// format (ingest.Archive.Save), when the instance has one.
// A failure is logged and does not stop the import: the values matter
// more than the copy.
func (s *sessions) archive(ctx context.Context, p auth.Principal, name string, body []byte) {
	if s.ingestArchive == nil {
		return
	}
	if err := s.ingestArchive.Save(p.User.ID, name, body); err != nil {
		s.logger.ErrorContext(ctx, "archiving an ingest failed", "user_id", p.User.ID.String(), "error", err)
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
			return nil, internalError("importing the metrics failed", fmt.Errorf("the day %s: %w", d.Date, err))
		}
		got := ingestDay{Date: d.Date.String(), Changed: []ingestChange{}}
		for _, c := range changes {
			ch := ingestChange{Metric: string(c.After.Kind), Value: c.After.Value}
			if c.Before != nil {
				ch.Before = &c.Before.Value
			}
			got.Changed = append(got.Changed, ch)
		}
		out.Body.Days = append(out.Body.Days, got)
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
