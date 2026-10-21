package api

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"

	"github.com/zarnautovic/tdeemon/internal/auth"
	"github.com/zarnautovic/tdeemon/internal/domain/user"
	"github.com/zarnautovic/tdeemon/internal/store"
)

// requireSession returns the principal of a request that came with a web
// session: a person in a browser. Settings, and the tokens themselves,
// change only that way, never with an API token (DESIGN.md §8). gateTokens
// refuses tokens before such an operation runs; this keeps the operation
// safe if it is ever opened to them by mistake.
func requireSession(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	switch {
	case !ok:
		return auth.Principal{}, huma.Error401Unauthorized("not signed in")
	case p.Via != auth.ViaSession:
		return auth.Principal{}, huma.Error403Forbidden("this needs a web session, not an API token")
	}
	return p, nil
}

// tokenScopeKey, in an operation's Metadata (see openToTokens), lets API
// tokens call the operation. An operation without it refuses every token
// with 403, so a new endpoint is closed to tokens until it names the
// scope they need (DESIGN.md §8: settings only through a web session).
const tokenScopeKey = "tokenScope"

// anyScope, in tokenScopeKey, opens an operation to a token with any
// scope. It is no scope's name, and not the zero value: an empty scope
// opens nothing.
const anyScope auth.Scope = "*"

// openToTokens is the Metadata of an operation API tokens with scope may
// call.
func openToTokens(scope auth.Scope) map[string]any { return map[string]any{tokenScopeKey: scope} }

// openToAnyToken is the Metadata of an operation every API token may
// call.
func openToAnyToken() map[string]any { return openToTokens(anyScope) }

// gateTokens is the API's middleware that keeps tokens to the operations
// open to them (tokenScopeKey). Requests with a session, or with no one,
// pass; the operation decides about those.
func gateTokens(a huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		p, ok := auth.PrincipalFrom(ctx.Context())
		if !ok || p.Via != auth.ViaToken {
			next(ctx)
			return
		}
		scope, open := ctx.Operation().Metadata[tokenScopeKey].(auth.Scope)
		switch {
		case !open:
			_ = huma.WriteErr(a, ctx, http.StatusForbidden, "this operation is closed to API tokens")
		case scope != anyScope && !p.Can(scope):
			_ = huma.WriteErr(a, ctx, http.StatusForbidden, "the API token does not have the "+string(scope)+" scope")
		default:
			next(ctx)
		}
	}
}

// bearerToken returns the API token of an Authorization header: the
// Bearer scheme, whose name is case-insensitive (RFC 7235), with a
// credential that starts with auth.TokenPrefix, or none at all (which
// fails as a token). Anything else is not for tdeemon, so the request is
// left to its cookie (RFC 6750 §3.1 treats it as not authenticated): a
// reverse proxy's Basic credentials, or the ID token an authenticating
// proxy (oauth2-proxy) passes on as a Bearer, which the browser's
// requests then all carry.
func bearerToken(header string) (string, bool) {
	scheme, token, _ := strings.Cut(header, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimLeft(token, " ")
	if token != "" && !strings.HasPrefix(token, auth.TokenPrefix) {
		return "", false
	}
	return token, true
}

// writeBadToken answers a Bearer token that is no live API token, with
// the challenge RFC 6750 asks for.
func writeBadToken(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	writeProblem(w, http.StatusUnauthorized, "the API token is not valid")
}

// authenticateToken handles a request with a Bearer token. An address
// that sent too many bad tokens lately gets 429 for every Bearer request,
// a good token included, until its window passes (limit.go).
func (s *sessions) authenticateToken(w http.ResponseWriter, r *http.Request, token string, next http.Handler) {
	addr := addrKey(clientAddrFrom(r.Context()))
	if wait, ok := s.badTokens.take(addr); !ok {
		maps.Copy(w.Header(), retryAfter(wait))
		writeProblem(w, http.StatusTooManyRequests, "too many invalid API tokens from this address; try again later")
		return
	}
	// The attempt take reserved stays a failure, unless the token is good.
	if !strings.HasPrefix(token, auth.TokenPrefix) {
		writeBadToken(w)
		return
	}
	t, u, err := s.db.APITokenUser(r.Context(), auth.SessionID(token))
	if errors.Is(err, store.ErrNotFound) {
		writeBadToken(w)
		return
	}
	s.badTokens.release(addr)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "looking up an API token failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "the API token could not be checked")
		return
	}
	scopes, err := auth.ParseScopes(t.Scopes)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "a stored API token has bad scopes", "token_id", t.ID.String(), "error", err)
		writeBadToken(w)
		return
	}
	now := s.now()
	if now.Sub(t.LastUsedAt) >= touchEvery {
		if err := s.db.TouchAPIToken(r.Context(), u.ID, t.ID, now); err != nil {
			s.logger.WarnContext(r.Context(), "recording token use failed", "error", err)
		}
	}
	ctx := auth.WithPrincipal(r.Context(), auth.Principal{User: u, Via: auth.ViaToken, TokenID: t.ID.String(), Scopes: scopes})
	next.ServeHTTP(w, r.WithContext(ctx))
}

type tokenBody struct {
	ID         string     `json:"id" format:"uuid" doc:"The token's handle; revoke it by this."`
	Name       string     `json:"name" doc:"What the user calls the token."`
	Scopes     []string   `json:"scopes" enum:"ingest,tools,read" doc:"What the token may do."`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" doc:"When it was last used, to the hour or so; absent until then."`
}

func newTokenBody(t store.APIToken) tokenBody {
	b := tokenBody{ID: t.ID.String(), Name: t.Name, Scopes: strings.Fields(t.Scopes), CreatedAt: t.CreatedAt.UTC()}
	if !t.LastUsedAt.IsZero() {
		last := t.LastUsedAt.UTC()
		b.LastUsedAt = &last
	}
	return b
}

type tokensOutput struct {
	CacheControl string `header:"Cache-Control" doc:"no-store"`
	Body         struct {
		Tokens []tokenBody `json:"tokens"`
	}
}

type createTokenInput struct {
	Body struct {
		Name   string   `json:"name" minLength:"1" maxLength:"100" example:"iPhone" doc:"What to call the token."`
		Scopes []string `json:"scopes" minItems:"1" maxItems:"3" uniqueItems:"true" enum:"ingest,tools,read" doc:"What it may do: ingest (the phone's health data), tools (log and change through MCP), read."`
	}
}

type createTokenOutput struct {
	CacheControl string `header:"Cache-Control" doc:"no-store"`
	Body         struct {
		tokenBody
		Token string `json:"token" example:"tdm_…" doc:"The token itself. It is shown only now; tdeemon keeps only its hash."`
	}
}

type revokeTokenInput struct {
	ID string `path:"id" format:"uuid"`
}

type whoamiOutput struct {
	CacheControl string `header:"Cache-Control" doc:"no-store"`
	Body         struct {
		User   userBody `json:"user"`
		Via    string   `json:"via" enum:"session,token" doc:"How the request authenticated."`
		Scopes []string `json:"scopes,omitempty" enum:"ingest,tools,read" doc:"What the token may do (via token)."`
	}
}

func (s *sessions) registerTokens(a huma.API) {
	huma.Register(a, huma.Operation{
		OperationID: "whoami",
		Method:      http.MethodGet,
		Path:        Prefix + "/auth/whoami",
		Summary:     "Get who the request acts for",
		Description: "The user, and whether the request came with a web session or an API token (with its scopes): a way for a client to check its token. Any token may call it.",
		Tags:        []string{"auth"},
		Errors:      []int{http.StatusUnauthorized, http.StatusTooManyRequests},
		Metadata:    openToAnyToken(),
	}, func(ctx context.Context, _ *struct{}) (*whoamiOutput, error) {
		p, ok := auth.PrincipalFrom(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("not signed in")
		}
		out := &whoamiOutput{CacheControl: "no-store"}
		out.Body.User = newUserBody(p.User)
		out.Body.Via = "session"
		if p.Via == auth.ViaToken {
			out.Body.Via = "token"
			for _, sc := range p.Scopes {
				out.Body.Scopes = append(out.Body.Scopes, string(sc))
			}
		}
		return out, nil
	})
	huma.Register(a, huma.Operation{
		OperationID: "list-tokens",
		Method:      http.MethodGet,
		Path:        Prefix + "/tokens",
		Summary:     "List your API tokens",
		Description: "Needs a web session. The tokens themselves are never shown again.",
		Tags:        []string{"tokens"},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.listTokens)
	huma.Register(a, huma.Operation{
		OperationID:   "create-token",
		Method:        http.MethodPost,
		Path:          Prefix + "/tokens",
		Summary:       "Create an API token",
		Description:   "Needs a web session. The response carries the token, once. A device or a client then sends it as `Authorization: Bearer <token>`.",
		Tags:          []string{"tokens"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden},
	}, s.createToken)
	huma.Register(a, huma.Operation{
		OperationID:   "revoke-token",
		Method:        http.MethodDelete,
		Path:          Prefix + "/tokens/{id}",
		Summary:       "Revoke an API token",
		Description:   "Needs a web session. The token stops working at once.",
		Tags:          []string{"tokens"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
	}, s.revokeToken)
}

func (s *sessions) listTokens(ctx context.Context, _ *struct{}) (*tokensOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	tokens, err := s.db.APITokens(ctx, p.User.ID)
	if err != nil {
		s.logger.ErrorContext(ctx, "listing API tokens failed", "error", err)
		return nil, huma.Error500InternalServerError("listing the tokens failed")
	}
	out := &tokensOutput{CacheControl: "no-store"}
	out.Body.Tokens = make([]tokenBody, 0, len(tokens))
	for _, t := range tokens {
		out.Body.Tokens = append(out.Body.Tokens, newTokenBody(t))
	}
	return out, nil
}

func (s *sessions) createToken(ctx context.Context, in *createTokenInput) (*createTokenOutput, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Body.Name)
	if err := user.ValidateName(name); err != nil {
		return nil, huma.Error422UnprocessableEntity("the token's name: " + strings.TrimPrefix(err.Error(), "name "))
	}
	scopes, err := auth.ParseScopes(strings.Join(in.Body.Scopes, " "))
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	token, hash := auth.NewAPIToken()
	t := store.APIToken{
		ID: uuid.NewV7(), UserID: p.User.ID, Name: name, Scopes: auth.FormatScopes(scopes),
		CreatedAt: s.now().Truncate(time.Second),
	}
	if err := s.db.CreateAPIToken(ctx, t, hash); err != nil {
		s.logger.ErrorContext(ctx, "creating an API token failed", "error", err)
		return nil, huma.Error500InternalServerError("creating the token failed")
	}
	s.logger.InfoContext(ctx, "created an API token", "user_id", p.User.ID.String(), "token_id", t.ID.String(), "scopes", t.Scopes)
	out := &createTokenOutput{CacheControl: "no-store"}
	out.Body.tokenBody = newTokenBody(t)
	out.Body.Token = token
	return out, nil
}

func (s *sessions) revokeToken(ctx context.Context, in *revokeTokenInput) (*struct{}, error) {
	p, err := requireSession(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, huma.Error404NotFound("no such token")
	}
	if err := s.db.RevokeAPIToken(ctx, p.User.ID, id); errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such token")
	} else if err != nil {
		s.logger.ErrorContext(ctx, "revoking an API token failed", "error", err)
		return nil, huma.Error500InternalServerError("revoking the token failed")
	}
	s.logger.InfoContext(ctx, "revoked an API token", "user_id", p.User.ID.String(), "token_id", id.String())
	return nil, nil //nolint:nilnil // huma answers a nil output with the DefaultStatus, 204.
}
