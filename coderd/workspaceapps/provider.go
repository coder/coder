package workspaceapps

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// TODO(@deansheather): configurable expiry
	DefaultTokenExpiry = time.Minute

	// RedirectURIQueryParam is the query param for the app URL to be passed
	// back to the API auth endpoint on the main access URL.
	RedirectURIQueryParam = "redirect_uri"
)

type ResolveRequestOptions struct {
	Logger              slog.Logger
	SignedTokenProvider SignedTokenProvider
	SecureCookie        bool

	DashboardURL   *url.URL
	PathAppBaseURL *url.URL
	AppHostname    string

	AppRequest Request
	// TODO: Replace these 2 fields with a "BrowserURL" field which is used for
	// redirecting the user back to their initial request after authenticating.
	// AppPath is the path under the app that was hit.
	AppPath string
	// AppQuery is the raw query of the request.
	AppQuery string
}

func ResolveRequest(rw http.ResponseWriter, r *http.Request, opts ResolveRequestOptions) (*SignedToken, bool) {
	appReq := opts.AppRequest.Normalize()
	err := appReq.Check()
	if err != nil {
		// This is a 500 since it's a coder server or proxy that's making this
		// request struct based on details from the request. The values should
		// already be validated before they are put into the struct.
		WriteWorkspaceApp500(opts.Logger, opts.DashboardURL, rw, r, &appReq, err, "invalid app request")
		return nil, false
	}

	token, ok := opts.SignedTokenProvider.FromRequest(r)
	if ok && token.MatchesRequest(appReq) {
		// The request has a valid signed app token and it matches the request.
		return token, true
	}

	issueReq := IssueTokenRequest{
		AppRequest:     appReq,
		PathAppBaseURL: opts.PathAppBaseURL.String(),
		AppHostname:    opts.AppHostname,
		SessionToken:   AppConnectSessionTokenFromRequest(r, appReq.AccessMethod),
		AppPath:        opts.AppPath,
		AppQuery:       opts.AppQuery,
	}

	token, tokenStr, ok := opts.SignedTokenProvider.Issue(r.Context(), rw, r, issueReq)
	if !ok {
		return nil, false
	}

	// Write the signed app token cookie.
	//
	// For path apps, this applies to only the path app base URL on the current
	// domain, e.g.
	//   /@user/workspace[.agent]/apps/path-app/
	//
	// For subdomain apps, this applies to the entire subdomain, e.g.
	//   app--agent--workspace--user.apps.example.com
	http.SetCookie(rw, &http.Cookie{
		Name:    codersdk.SignedAppTokenCookie,
		Value:   tokenStr,
		Path:    appReq.BasePath,
		Expires: token.Expiry.Time(),
		Secure:  opts.SecureCookie,
	})

	return token, true
}

// SignedTokenProvider provides signed workspace app tokens (aka. app tickets).
type SignedTokenProvider interface {
	// FromRequest returns a parsed token from the request. If the request does
	// not contain a signed app token or is is invalid (expired, invalid
	// signature, etc.), it returns false.
	FromRequest(r *http.Request) (*SignedToken, bool)
	// Issue mints a new token for the given app request. It uses the long-lived
	// session token in the HTTP request to authenticate and authorize the
	// client for the given workspace app. The token is returned in struct and
	// string form. The string form should be written as a cookie.
	//
	// If the request is invalid or the user is not authorized to access the
	// app, false is returned. An error page is written to the response writer
	// in this case.
	Issue(ctx context.Context, rw http.ResponseWriter, r *http.Request, appReq IssueTokenRequest) (*SignedToken, string, bool)
}
