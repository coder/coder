package workspaceapps

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

const (
	// TODO(@deansheather): configurable expiry
	DefaultTokenExpiry = time.Minute

	// RedirectURIQueryParam is the query param for the app URL to be passed
	// back to the API auth endpoint on the main access URL.
	RedirectURIQueryParam = "redirect_uri"
)

type ResolveRequestOpts struct {
	Logger              slog.Logger
	SignedTokenProvider SignedTokenProvider

	DashboardURL   *url.URL
	PathAppBaseURL *url.URL
	AppHostname    string

	AppRequest Request
	// AppPath is the path under the app that was hit.
	AppPath string
	// AppQuery is the raw query of the request.
	AppQuery string
}

func ResolveRequest(rw http.ResponseWriter, r *http.Request, opts ResolveRequestOpts) (*SignedToken, bool) {
	appReq := opts.AppRequest.Normalize()
	err := appReq.Validate()
	if err != nil {
		WriteWorkspaceApp500(opts.Logger, opts.DashboardURL, rw, r, &appReq, err, "invalid app request")
		return nil, false
	}

	token, ok := opts.SignedTokenProvider.TokenFromRequest(r)
	if ok && token.MatchesRequest(appReq) {
		// The request has a valid signed app token and it matches the request.
		return token, true
	}

	issueReq := IssueTokenRequest{
		AppRequest:     appReq,
		PathAppBaseURL: opts.PathAppBaseURL.String(),
		AppHostname:    opts.AppHostname,
		SessionToken:   httpmw.APITokenFromRequest(r),
		AppPath:        opts.AppPath,
		AppQuery:       opts.AppQuery,
	}

	token, tokenStr, ok := opts.SignedTokenProvider.IssueToken(r.Context(), rw, r, issueReq)
	if !ok {
		return nil, false
	}

	// Write the signed app token cookie. We always want this to apply to the
	// current hostname (even for subdomain apps, without any wildcard
	// shenanigans, because the token is only valid for a single app).
	http.SetCookie(rw, &http.Cookie{
		Name:    codersdk.DevURLSignedAppTokenCookie,
		Value:   tokenStr,
		Path:    appReq.BasePath,
		Expires: token.Expiry,
	})

	return token, true
}

// SignedTokenProvider provides signed workspace app tokens (aka. app tickets).
type SignedTokenProvider interface {
	// TokenFromRequest returns a parsed token from the request. If the request
	// does not contain a signed app token or is is invalid (expired, invalid
	// signature, etc.), it returns false.
	TokenFromRequest(r *http.Request) (*SignedToken, bool)
	// IssueToken mints a new token for the given app request. It uses the
	// long-lived session token in the HTTP request to authenticate and
	// authorize the client for the given workspace app. The token is returned
	// in struct and string form. The string form should be written as a cookie.
	//
	// If the request is invalid or the user is not authorized to access the
	// app, false is returned. An error page is written to the response writer
	// in this case.
	IssueToken(ctx context.Context, rw http.ResponseWriter, r *http.Request, appReq IssueTokenRequest) (*SignedToken, string, bool)
}
