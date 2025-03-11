package httpmw

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

type oauth2StateKey struct{}

type OAuth2State struct {
	Token       *oauth2.Token
	Redirect    string
	StateString string
}

// OAuth2 returns the state from an oauth request.
func OAuth2(r *http.Request) OAuth2State {
	oauth, ok := r.Context().Value(oauth2StateKey{}).(OAuth2State)
	if !ok {
		panic("developer error: oauth middleware not provided")
	}
	return oauth
}

// ExtractOAuth2 is a middleware for automatically redirecting to OAuth
// URLs, and handling the exchange inbound. Any route that does not have
// a "code" URL parameter will be redirected.
// AuthURLOpts are passed to the AuthCodeURL function. If this is nil,
// the default option oauth2.AccessTypeOffline will be used.
func ExtractOAuth2(config promoauth.OAuth2Config, client *http.Client, cookieCfg codersdk.HTTPCookieConfig, authURLOpts map[string]string) func(http.Handler) http.Handler {
	opts := make([]oauth2.AuthCodeOption, 0, len(authURLOpts)+1)
	opts = append(opts, oauth2.AccessTypeOffline)
	for k, v := range authURLOpts {
		opts = append(opts, oauth2.SetAuthURLParam(k, v))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if client != nil {
				ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
			}

			// Interfaces can hold a nil value
			if config == nil || reflect.ValueOf(config).IsNil() {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "The oauth2 method requested is not configured!",
				})
				return
			}

			// OIDC errors can be returned as query parameters. This can happen
			// if for example we are providing and invalid scope.
			// We should terminate the OIDC process if we encounter an error.
			errorMsg := r.URL.Query().Get("error")
			errorDescription := r.URL.Query().Get("error_description")
			errorURI := r.URL.Query().Get("error_uri")
			if errorMsg != "" {
				// Combine the errors into a single string if either is provided.
				if errorDescription == "" && errorURI != "" {
					errorDescription = fmt.Sprintf("error_uri: %s", errorURI)
				} else if errorDescription != "" && errorURI != "" {
					errorDescription = fmt.Sprintf("%s, error_uri: %s", errorDescription, errorURI)
				}
				errorMsg = fmt.Sprintf("Encountered error in oidc process: %s", errorMsg)
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: errorMsg,
					// This message might be blank. This is ok.
					Detail: errorDescription,
				})
				return
			}

			code := r.URL.Query().Get("code")
			state := r.URL.Query().Get("state")
			redirect := r.URL.Query().Get("redirect")
			if redirect != "" {
				// We want to ensure that we're only ever redirecting to the application.
				// We could be more strict here and check to see if the host matches
				// the host of the AccessURL but ultimately as long as our redirect
				// url omits a host we're ensuring that we're routing to a path
				// local to the application.
				redirect = uriFromURL(redirect)
			}

			if code == "" {
				// If the code isn't provided, we'll redirect!
				var state string
				// If this url param is provided, then a user is trying to merge
				// their account with an OIDC account. Their password would have
				// been required to get to this point, so we do not need to verify
				// their password again.
				oidcMergeState := r.URL.Query().Get("oidc_merge_state")
				if oidcMergeState != "" {
					state = oidcMergeState
				} else {
					var err error
					state, err = cryptorand.String(32)
					if err != nil {
						httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
							Message: "Internal error generating state string.",
							Detail:  err.Error(),
						})
						return
					}
				}

				http.SetCookie(rw, cookieCfg.Apply(&http.Cookie{
					Name:     codersdk.OAuth2StateCookie,
					Value:    state,
					Path:     "/",
					HttpOnly: true,
				}))
				// Redirect must always be specified, otherwise
				// an old redirect could apply!
				http.SetCookie(rw, cookieCfg.Apply(&http.Cookie{
					Name:     codersdk.OAuth2RedirectCookie,
					Value:    redirect,
					Path:     "/",
					HttpOnly: true,
				}))

				http.Redirect(rw, r, config.AuthCodeURL(state, opts...), http.StatusTemporaryRedirect)
				return
			}

			if state == "" {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "State must be provided.",
				})
				return
			}

			stateCookie, err := r.Cookie(codersdk.OAuth2StateCookie)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("Cookie %q must be provided.", codersdk.OAuth2StateCookie),
				})
				return
			}
			if stateCookie.Value != state {
				httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
					Message: "State mismatched.",
				})
				return
			}

			stateRedirect, err := r.Cookie(codersdk.OAuth2RedirectCookie)
			if err == nil {
				redirect = stateRedirect.Value
			}

			oauthToken, err := config.Exchange(ctx, code)
			if err != nil {
				errorCode := http.StatusInternalServerError
				detail := err.Error()
				if detail == "authorization_pending" {
					// In the device flow, the token may not be immediately
					// available. This is expected, and the client will retry.
					errorCode = http.StatusBadRequest
				}
				httpapi.Write(ctx, rw, errorCode, codersdk.Response{
					Message: "Failed exchanging Oauth code.",
					Detail:  detail,
				})
				return
			}

			ctx = context.WithValue(ctx, oauth2StateKey{}, OAuth2State{
				Token:       oauthToken,
				Redirect:    redirect,
				StateString: state,
			})
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

type (
	oauth2ProviderAppParamContextKey       struct{}
	oauth2ProviderAppSecretParamContextKey struct{}
)

// OAuth2ProviderApp returns the OAuth2 app from the ExtractOAuth2ProviderAppParam handler.
func OAuth2ProviderApp(r *http.Request) database.OAuth2ProviderApp {
	app, ok := r.Context().Value(oauth2ProviderAppParamContextKey{}).(database.OAuth2ProviderApp)
	if !ok {
		panic("developer error: oauth2 app param middleware not provided")
	}
	return app
}

// ExtractOAuth2ProviderApp grabs an OAuth2 app from the "app" URL parameter.  This
// middleware requires the API key middleware higher in the call stack for
// authentication.
func ExtractOAuth2ProviderApp(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// App can come from a URL param, query param, or form value.
			paramID := "app"
			var appID uuid.UUID
			if chi.URLParam(r, paramID) != "" {
				var ok bool
				appID, ok = ParseUUIDParam(rw, r, "app")
				if !ok {
					return
				}
			} else {
				// If not provided by the url, then it is provided according to the
				// oauth 2 spec. This can occur with query params, or in the body as
				// form parameters.
				// This also depends on if you are doing a POST (tokens) or GET (authorize).
				paramAppID := r.URL.Query().Get("client_id")
				if paramAppID == "" {
					// Check the form params!
					if r.ParseForm() == nil {
						paramAppID = r.Form.Get("client_id")
					}
				}
				if paramAppID == "" {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "Missing OAuth2 client ID.",
					})
					return
				}

				var err error
				appID, err = uuid.Parse(paramAppID)
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "Invalid OAuth2 client ID.",
						Detail:  err.Error(),
					})
					return
				}
			}

			app, err := db.GetOAuth2ProviderAppByID(ctx, appID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching OAuth2 app.",
					Detail:  err.Error(),
				})
				return
			}
			ctx = context.WithValue(ctx, oauth2ProviderAppParamContextKey{}, app)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// OAuth2ProviderAppSecret returns the OAuth2 app secret from the
// ExtractOAuth2ProviderAppSecretParam handler.
func OAuth2ProviderAppSecret(r *http.Request) database.OAuth2ProviderAppSecret {
	app, ok := r.Context().Value(oauth2ProviderAppSecretParamContextKey{}).(database.OAuth2ProviderAppSecret)
	if !ok {
		panic("developer error: oauth2 app secret param middleware not provided")
	}
	return app
}

// ExtractOAuth2ProviderAppSecret grabs an OAuth2 app secret from the "app" and
// "secret" URL parameters.  This middleware requires the ExtractOAuth2ProviderApp
// middleware higher in the stack
func ExtractOAuth2ProviderAppSecret(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			secretID, ok := ParseUUIDParam(rw, r, "secretID")
			if !ok {
				return
			}
			app := OAuth2ProviderApp(r)
			secret, err := db.GetOAuth2ProviderAppSecretByID(ctx, secretID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching OAuth2 app secret.",
					Detail:  err.Error(),
				})
				return
			}
			// If the user can read the secret they can probably also read the app it
			// belongs to and they can read this app as well, so it seems safe to give
			// them a more helpful message than a 404 on mismatches.
			if app.ID != secret.AppID {
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "App ID does not match secret app ID.",
				})
				return
			}
			ctx = context.WithValue(ctx, oauth2ProviderAppSecretParamContextKey{}, secret)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

func uriFromURL(u string) string {
	uri, err := url.Parse(u)
	if err != nil {
		return "/"
	}

	return uri.RequestURI()
}
