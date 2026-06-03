package wsproxy

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
)

var _ workspaceapps.Provider = (*AppsProvider)(nil)

type AppsProvider struct {
	DashboardURL *url.URL

	Client               *wsproxysdk.Client
	TokenSigningKeycache cryptokeys.SigningKeycache
	Logger               slog.Logger
}

func (p *AppsProvider) FromRequest(r *http.Request) (*workspaceapps.SignedToken, bool) {
	return workspaceapps.FromRequest(r, p.TokenSigningKeycache)
}

func (p *AppsProvider) Issue(ctx context.Context, rw http.ResponseWriter, r *http.Request, issueReq workspaceapps.IssueTokenRequest) (*workspaceapps.SignedToken, string, bool) {
	appReq := issueReq.AppRequest.Normalize()
	err := appReq.Check()
	if err != nil {
		workspaceapps.WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "invalid app request")
		return nil, "", false
	}
	issueReq.AppRequest = appReq

	resp, ok := p.Client.IssueSignedAppTokenHTML(ctx, rw, issueReq, r.RemoteAddr)
	if !ok {
		return nil, "", false
	}

	// Check that it verifies properly and matches the string.
	var token workspaceapps.SignedToken
	err = jwtutils.Verify(ctx, p.TokenSigningKeycache, resp.SignedTokenStr, &token)
	if err != nil {
		workspaceapps.WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "failed to verify newly generated signed token")
		return nil, "", false
	}

	// Check that it matches the request.
	if !token.MatchesRequest(appReq) {
		workspaceapps.WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "newly generated signed token does not match request")
		return nil, "", false
	}

	return &token, resp.SignedTokenStr, true
}

// ResolveAppOwnerID fetches app owner via coderd. wsproxy can verify
// signed app tokens locally, but authoritative app ownership still
// comes from coderd.
func (p *AppsProvider) ResolveAppOwnerID(ctx context.Context, app appurl.ApplicationURL) (uuid.UUID, error) {
	req := (workspaceapps.Request{
		AccessMethod:      workspaceapps.AccessMethodSubdomain,
		BasePath:          "/",
		Prefix:            app.Prefix,
		UsernameOrID:      app.Username,
		WorkspaceNameOrID: app.WorkspaceName,
		AgentNameOrID:     app.AgentName,
		AppSlugOrPort:     app.AppSlugOrPort,
	}).Normalize()

	resp, err := p.Client.ResolveAppOwnerID(ctx, wsproxysdk.ResolveAppOwnerIDRequest{
		AppRequest: req,
	})
	if err != nil {
		return uuid.Nil, xerrors.Errorf("resolve app owner ID: %w", err)
	}
	return resp.OwnerID, nil
}
