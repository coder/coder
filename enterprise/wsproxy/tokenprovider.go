package wsproxy

import (
	"context"
	"net/http"
	"net/url"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/enterprise/wsproxy/wsproxysdk"
)

var _ workspaceapps.SignedTokenProvider = (*TokenProvider)(nil)

type TokenProvider struct {
	DashboardURL *url.URL
	AccessURL    *url.URL
	AppHostname  string

	Client      *wsproxysdk.Client
	SecurityKey workspaceapps.SecurityKey
	Logger      slog.Logger
}

func (p *TokenProvider) FromRequest(r *http.Request) (*workspaceapps.SignedToken, bool) {
	return workspaceapps.FromRequest(r, p.SecurityKey)
}

func (p *TokenProvider) Issue(ctx context.Context, rw http.ResponseWriter, r *http.Request, issueReq workspaceapps.IssueTokenRequest) (*workspaceapps.SignedToken, string, bool) {
	appReq := issueReq.AppRequest.Normalize()
	err := appReq.Validate()
	if err != nil {
		workspaceapps.WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "invalid app request")
		return nil, "", false
	}
	issueReq.AppRequest = appReq

	resp, ok := p.Client.IssueSignedAppTokenHTML(ctx, rw, issueReq)
	if !ok {
		return nil, "", false
	}

	// Check that it verifies properly and matches the string.
	token, err := p.SecurityKey.VerifySignedToken(resp.SignedTokenStr)
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
