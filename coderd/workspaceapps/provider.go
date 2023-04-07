package workspaceapps

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/codersdk"
)

const (
	// TODO(@deansheather): configurable expiry
	DefaultTokenExpiry = time.Minute

	// RedirectURIQueryParam is the query param for the app URL to be passed
	// back to the API auth endpoint on the main access URL.
	RedirectURIQueryParam = "redirect_uri"
)

// ResolveRequest calls SignedTokenProvider to use an existing signed app token in the
// request or issue a new one. If it returns a newly minted token, it sets the
// cookie for you.
func ResolveRequest(log slog.Logger, dashboardURL *url.URL, p SignedTokenProvider, rw http.ResponseWriter, r *http.Request, appReq Request) (*SignedToken, bool) {
	appReq = appReq.Normalize()
	err := appReq.Validate()
	if err != nil {
		WriteWorkspaceApp500(log, dashboardURL, rw, r, &appReq, err, "invalid app request")
		return nil, false
	}

	token, ok := p.TokenFromRequest(r)
	if ok && token.MatchesRequest(appReq) {
		// The request has a valid signed app token and it matches the request.
		return token, true
	}

	token, tokenStr, ok := p.CreateToken(r.Context(), rw, r, appReq)
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
	// CreateToken mints a new token for the given app request. It uses the
	// long-lived session token in the HTTP request to authenticate and
	// authorize the client for the given workspace app. The token is returned
	// in struct and string form. The string form should be written as a cookie.
	//
	// If the request is invalid or the user is not authorized to access the
	// app, false is returned. An error page is written to the response writer
	// in this case.
	CreateToken(ctx context.Context, rw http.ResponseWriter, r *http.Request, appReq Request) (*SignedToken, string, bool)
}
