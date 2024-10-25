package wsproxy

import (
	"context"
	"net/http"
	"net/url"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
)

var _ workspaceapps.SignedTokenProvider = (*TokenProvider)(nil)

type TokenProvider struct {
	DashboardURL *url.URL
	AccessURL    *url.URL
	AppHostname  string

	Client                   *wsproxysdk.Client
	TokenSigningKeycache     cryptokeys.SigningKeycache
	APIKeyEncryptionKeycache cryptokeys.EncryptionKeycache
	Logger                   slog.Logger
}

func (p *TokenProvider) FromRequest(r *http.Request) (*workspaceapps.SignedToken, bool) {
	return workspaceapps.FromRequest(r, p.TokenSigningKeycache)
}

func (p *TokenProvider) Issue(ctx context.Context, rw http.ResponseWriter, r *http.Request, issueReq workspaceapps.IssueTokenRequest) (*workspaceapps.SignedToken, string, bool) {
	appReq := issueReq.AppRequest.Normalize()
	err := appReq.Check()
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
