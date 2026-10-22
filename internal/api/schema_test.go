package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

// checkBody fails unless body, an answer of the operation opID with
// status, holds what the committed OpenAPI document says: every
// property it has is in the schema, and every required one is there. A
// response type the document misdescribes (a field huma skips) shows
// here, where the generated TypeScript types would not.
func checkBody(t *testing.T, opID string, status int, body []byte) {
	t.Helper()
	raw, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
			Responses   map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	var ref string
	for _, ops := range spec.Paths {
		for _, op := range ops {
			if op.OperationID == opID {
				ref = op.Responses[strconv.Itoa(status)].Content["application/json"].Schema.Ref
			}
		}
	}
	schema, ok := spec.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
	if !ok {
		t.Fatalf("no schema for %s %d in the document (%q)", opID, status, ref)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	for k := range got {
		if _, ok := schema.Properties[k]; !ok {
			t.Errorf("%s %d: the answer has %q, which the document does not describe", opID, status, k)
		}
	}
	for _, k := range schema.Required {
		if _, ok := got[k]; !ok {
			t.Errorf("%s %d: the answer lacks %q, which the document requires", opID, status, k)
		}
	}
}

// The answers of the operations hold what the document says they do.
func TestAnswersMatchTheDocument(t *testing.T) {
	t.Parallel()

	in, session := signedIn(t)
	cookies := []*http.Cookie{session}
	_, rec := in.createToken(t, session, `{"name":"phone","scopes":["read"]}`)
	checkBody(t, "create-token", http.StatusCreated, rec.Body.Bytes())
	for _, r := range []struct{ op, path string }{
		{"list-tokens", "/api/v1/tokens"},
		{"whoami", "/api/v1/auth/whoami"},
		{"get-session", "/api/v1/auth/session"},
		{"get-info", "/api/v1/info"},
	} {
		rec := in.do(t, http.MethodGet, r.path, "", cookies, nil)
		checkBody(t, r.op, rec.Code, rec.Body.Bytes())
	}
}
