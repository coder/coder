package workspaceapps

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/codersdk"
)

// SignedToken is the struct data contained inside a workspace app JWE. It
// contains the details of the workspace app that the token is valid for to
// avoid database queries.
type SignedToken struct {
	jwtutils.RegisteredClaims
	// Request details.
	Request `json:"request"`

	UserID      uuid.UUID `json:"user_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	AppURL      string    `json:"app_url"`
}

// MatchesRequest returns true if the token matches the request. Any token that
// does not match the request should be considered invalid.
func (t SignedToken) MatchesRequest(req Request) bool {
	tokenBasePath := t.Request.BasePath
	if !strings.HasSuffix(tokenBasePath, "/") {
		tokenBasePath += "/"
	}
	reqBasePath := req.BasePath
	if !strings.HasSuffix(reqBasePath, "/") {
		reqBasePath += "/"
	}

	return t.AccessMethod == req.AccessMethod &&
		tokenBasePath == reqBasePath &&
		t.Prefix == req.Prefix &&
		t.UsernameOrID == req.UsernameOrID &&
		t.WorkspaceNameOrID == req.WorkspaceNameOrID &&
		t.AgentNameOrID == req.AgentNameOrID &&
		t.AppSlugOrPort == req.AppSlugOrPort
}

type EncryptedAPIKeyPayload struct {
	jwtutils.RegisteredClaims
	APIKey string `json:"api_key"`
}

func (e *EncryptedAPIKeyPayload) Fill(now time.Time) {
	e.Issuer = "coderd"
	e.Audience = jwt.Audience{"wsproxy"}
	e.Expiry = jwt.NewNumericDate(now.Add(time.Minute))
	e.NotBefore = jwt.NewNumericDate(now.Add(-time.Minute))
}

func (e EncryptedAPIKeyPayload) Validate(ex jwt.Expected) error {
	if e.NotBefore == nil {
		return xerrors.Errorf("not before is required")
	}

	ex.Issuer = "coderd"
	ex.AnyAudience = jwt.Audience{"wsproxy"}

	return e.RegisteredClaims.Validate(ex)
}

// FromRequest returns the signed token from the request, if it exists and is
// valid. The caller must check that the token matches the request.
func FromRequest(r *http.Request, mgr cryptokeys.SigningKeycache) (*SignedToken, bool) {
	// Get all signed app tokens from the request. This includes the query
	// parameter and all matching cookies sent with the request. If there are
	// somehow multiple signed app token cookies, we want to try all of them
	// (up to 4). The first one that is valid is used.
	//
	// Browsers will send all cookies in the request, even if there are multiple
	// with the same name on different paths.
	//
	// If using a query parameter the request MUST be a terminal request. We use
	// this to support cross-domain terminal access for the web terminal.
	var (
		tokens        = []string{}
		hasQueryParam = false
	)
	if q := r.URL.Query().Get(codersdk.SignedAppTokenQueryParameter); q != "" {
		hasQueryParam = true
		tokens = append(tokens, q)
	}
	for _, cookie := range r.Cookies() {
		if cookie.Name == codersdk.SignedAppTokenCookie {
			tokens = append(tokens, cookie.Value)
		}
	}

	if len(tokens) > 4 {
		tokens = tokens[:4]
	}

	ctx := r.Context()
	for _, tokenStr := range tokens {
		var token SignedToken
		err := jwtutils.Verify(ctx, mgr, tokenStr, &token, jwtutils.WithVerifyExpected(jwt.Expected{
			Time: time.Now(),
		}))
		if err == nil {
			req := token.Request.Normalize()
			if hasQueryParam && req.AccessMethod != AccessMethodTerminal {
				// The request must be a terminal request if we're using a
				// query parameter.
				return nil, false
			}

			err := req.Check()
			if err == nil {
				// The request has a valid signed app token, which is a valid
				// token signed by us. The caller must check that it matches
				// the request.
				return &token, true
			}
		}
	}

	return nil, false
}
