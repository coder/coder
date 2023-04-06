package wsproxy

import (
	"context"
	"net/http"
	"net/url"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/enterprise/wsproxy/wsproxysdk"
)

var _ workspaceapps.SignedTokenProvider = (*ProxyTokenProvider)(nil)

type ProxyTokenProvider struct {
	DashboardURL *url.URL
	Client       *wsproxysdk.Client
	SecurityKey  workspaceapps.SecurityKey
	Logger       slog.Logger
}

func NewProxyTokenProvider() *ProxyTokenProvider {
	return &ProxyTokenProvider{}
}

func (p *ProxyTokenProvider) TokenFromRequest(r *http.Request) (*workspaceapps.SignedToken, bool) {
	return workspaceapps.TokenFromRequest(r, p.SecurityKey)
}

func (p *ProxyTokenProvider) CreateToken(ctx context.Context, rw http.ResponseWriter, r *http.Request, appReq workspaceapps.Request) (*workspaceapps.SignedToken, string, bool) {
	appReq = appReq.Normalize()
	err := appReq.Validate()
	if err != nil {
		workspaceapps.WriteWorkspaceApp500(p.Logger, p.DashboardURL, rw, r, &appReq, err, "invalid app request")
		return nil, "", false
	}

	userToken := UserSessionToken(r)
	resp, ok := p.Client.IssueSignedAppTokenHTML(ctx, rw, wsproxysdk.IssueSignedAppTokenRequest{
		AppRequest:   appReq,
		SessionToken: userToken,
	})
	if !ok {
		return nil, "", false
	}

	// TODO: @emyrk we should probably verify the appReq and the returned signed token match?
	return &resp.SignedToken, resp.SignedTokenStr, true
}
