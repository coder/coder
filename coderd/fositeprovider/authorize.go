package fositeprovider

import (
	"fmt"
	"net/http"
	"net/url"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/site"
)

func (p *Provider) ShowAuthorizationPage(accessURL *url.URL) http.HandlerFunc {
	// TODO: Unsure how correct ths is.
	return func(rw http.ResponseWriter, r *http.Request) {
		logger := p.logger.With(slog.F("handler", "get_auth_endpoint"))

		ctx := r.Context()

		// TODO: Do we do coderd auth here?
		ua := httpmw.UserAuthorization(r.Context())

		// Let's create an AuthorizeRequest object!
		// It will analyze the request and extract important information like scopes, response type and others.
		ar, err := p.provider.NewAuthorizeRequest(ctx, r)
		if err != nil {
			logger.Error(ctx, "error occurred in ShowAuthorizationPage", slog.Error(err))
			p.provider.WriteAuthorizeError(ctx, rw, ar, err)
			return
		}

		app := ar.GetClient()
		// primary redirect URI is always the first one
		appRedirects := app.GetRedirectURIs()
		if len(appRedirects) == 0 {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{Status: http.StatusInternalServerError, HideStatus: false, Title: "Internal Server Error",
				Description:  fmt.Sprintf("No redirect URIs configured for app %s", app.GetID()),
				RetryEnabled: false, DashboardURL: accessURL.String(), Warnings: nil})
			return
		}

		// TODO: Probably only needed if there is no redirect URI in the request
		callbackURL, err := url.Parse(appRedirects[0])
		if err != nil {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{Status: http.StatusInternalServerError, HideStatus: false, Title: "Internal Server Error", Description: err.Error(), RetryEnabled: false, DashboardURL: accessURL.String(), Warnings: nil})
			return
		}

		redirectURL := ar.GetRedirectURI()
		if redirectURL == nil {
			redirectURL = callbackURL
		}

		cancel := redirectURL
		cancelQuery := redirectURL.Query()
		cancelQuery.Add("error", "access_denied")
		cancel.RawQuery = cancelQuery.Encode()

		site.RenderOAuthAllowPage(rw, r, site.RenderOAuthAllowData{
			// TODO: Extend fosite.DefaultClient to have our information
			AppIcon:     "",          //app.Icon,
			AppName:     app.GetID(), // app.Name,
			CancelURI:   cancel.String(),
			RedirectURI: r.URL.String(),
			Username:    ua.FriendlyName,
		})
	}
}

// https://github.com/ory/fosite-example/blob/master/authorizationserver/oauth2_auth.go#L9
func (p *Provider) AuthEndpoint(rw http.ResponseWriter, r *http.Request) {
	// This context will be passed to all methods.
	ctx := r.Context()
	logger := p.logger.With(slog.F("handler", "post_auth_endpoint"))

	// Let's create an AuthorizeRequest object!
	// It will analyze the request and extract important information like scopes, response type and others.
	ar, err := p.provider.NewAuthorizeRequest(ctx, r)
	if err != nil {
		logger.Error(ctx, "error occurred in NewAuthorizeRequest", slog.Error(err))
		p.provider.WriteAuthorizeError(ctx, rw, ar, err)
		return
	}
	// You have now access to authorizeRequest, Code ResponseTypes, Scopes ...

	var requestedScopes string
	for _, this := range ar.GetRequestedScopes() {
		requestedScopes += fmt.Sprintf(`<li><input type="checkbox" name="scopes" value="%s">%s</li>`, this, this)
	}

	// This verifies the user is authenticated
	ua := httpmw.APIKey(r)

	// TODO: When we support scopes, this is how we can handle them.
	// let's see what scopes the user gave consent to
	//for _, scope := range r.PostForm["scopes"] {
	//	ar.GrantScope(scope)
	//}

	// Now that the user is authorized, we set up a session:
	mySessionData := p.newSession(ua)

	// When using the HMACSHA strategy you must use something that implements the HMACSessionContainer.
	// It brings you the power of overriding the default values.
	//
	// mySessionData.HMACSession = &strategy.HMACSession{
	//	AccessTokenExpiry: time.Now().Add(time.Day),
	//	AuthorizeCodeExpiry: time.Now().Add(time.Day),
	// }
	//

	// If you're using the JWT strategy, there's currently no distinction between access token and authorize code claims.
	// Therefore, you both access token and authorize code will have the same "exp" claim. If this is something you
	// need let us know on github.
	//
	// mySessionData.JWTClaims.ExpiresAt = time.Now().Add(time.Day)

	// It's also wise to check the requested scopes, e.g.:
	// if ar.GetRequestedScopes().Has("admin") {
	//     http.Error(rw, "you're not allowed to do that", http.StatusForbidden)
	//     return
	// }

	// Now we need to get a response. This is the place where the AuthorizeEndpointHandlers kick in and start processing the request.
	// NewAuthorizeResponse is capable of running multiple response type handlers which in turn enables this library
	// to support open id connect.
	response, err := p.provider.NewAuthorizeResponse(ctx, ar, mySessionData)

	// Catch any errors, e.g.:
	// * unknown client
	// * invalid redirect
	// * ...
	if err != nil {
		logger.Error(ctx, "error occurred in NewAuthorizeResponse", slog.Error(err))
		p.provider.WriteAuthorizeError(ctx, rw, ar, err)
		return
	}

	// Last but not least, send the response!
	p.provider.WriteAuthorizeResponse(ctx, rw, ar, response)
}
