package httpmw

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

type oauth2StateKey struct{}

type OAuth2State struct {
	Token    *oauth2.Token
	Redirect string
}

// OAuth2Config exposes a subset of *oauth2.Config functions for easier testing.
// *oauth2.Config should be used instead of implementing this in production.
type OAuth2Config interface {
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
	TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource
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
func ExtractOAuth2(config OAuth2Config, client *http.Client) func(http.Handler) http.Handler {
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

			code := r.URL.Query().Get("code")
			state := r.URL.Query().Get("state")

			if code == "" {
				// If the code isn't provided, we'll redirect!
				state, err := cryptorand.String(32)
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
						Message: "Internal error generating state string.",
						Detail:  err.Error(),
					})
					return
				}

				http.SetCookie(rw, &http.Cookie{
					Name:     codersdk.OAuth2StateCookie,
					Value:    state,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				// Redirect must always be specified, otherwise
				// an old redirect could apply!
				http.SetCookie(rw, &http.Cookie{
					Name:     codersdk.OAuth2RedirectCookie,
					Value:    r.URL.Query().Get("redirect"),
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})

				http.Redirect(rw, r, config.AuthCodeURL(state, oauth2.AccessTypeOffline), http.StatusTemporaryRedirect)
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

			var redirect string
			stateRedirect, err := r.Cookie(codersdk.OAuth2RedirectCookie)
			if err == nil {
				redirect = stateRedirect.Value
			}

			oauthToken, err := config.Exchange(ctx, code)
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error exchanging Oauth code.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, oauth2StateKey{}, OAuth2State{
				Token:    oauthToken,
				Redirect: redirect,
			})
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
