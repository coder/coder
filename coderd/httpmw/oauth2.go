package httpmw

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
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
//
// pkceMethods should be a list like ['S256', 'plain'] indicating
// which PKCE methods are supported by the OAuth2 provider. If empty,
// PKCE will not be used.
//
// redirectAllowedHosts, when non-empty, enables dynamic redirect_uri
// construction from the request Host header. The request Host must match
// (case-insensitive, ignoring port) one of the listed hostnames. The
// dynamic redirect_uri is cached in a cookie so the same value is reused
// for the token exchange, as required by RFC 6749 section 4.1.3. Pass nil
// to preserve the legacy behavior of using the redirect_uri baked into
// config at startup.
//
// redirectDefaultScheme is the scheme used when constructing the dynamic
// redirect_uri. It is populated from the configured AccessURL and takes
// precedence over r.TLS / X-Forwarded-Proto because some reverse proxies
// report the inner-hop scheme (e.g. "http") rather than the original
// client-facing scheme, which would produce a redirect_uri the IdP
// rejects. Callers must always supply this when redirectAllowedHosts is
// non-empty; an empty value would yield an invalid redirect_uri without
// a scheme.
func ExtractOAuth2(config promoauth.OAuth2Config, client *http.Client, cookieCfg codersdk.HTTPCookieConfig, authURLOpts map[string]string, pkceMethods []promoauth.Oauth2PKCEChallengeMethod, redirectAllowedHosts []string, redirectDefaultScheme string) func(http.Handler) http.Handler {
	opts := make([]oauth2.AuthCodeOption, 0, len(authURLOpts)+1)
	opts = append(opts, oauth2.AccessTypeOffline)
	for k, v := range authURLOpts {
		opts = append(opts, oauth2.SetAuthURLParam(k, v))
	}

	// Pre-normalize the allowlist once so the per-request check is a plain
	// case-insensitive compare and we do not re-allocate on every login.
	normalizedAllowedHosts := make([]string, 0, len(redirectAllowedHosts))
	for _, h := range redirectAllowedHosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		normalizedAllowedHosts = append(normalizedAllowedHosts, strings.ToLower(h))
	}
	dynamicRedirectEnabled := len(normalizedAllowedHosts) > 0

	// Only S256 PKCE is currently supported.
	sha256PKCESupported := slices.Contains(pkceMethods, promoauth.PKCEChallengeMethodSha256)
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
				redirect = httpapi.SafeRedirectPath(redirect)
			}

			// When dynamic redirect URIs are enabled, validate the request Host
			// against the allowlist regardless of whether we are initiating the
			// flow or handling the callback. Doing this upfront avoids burning
			// state and lets us reject obviously-bad requests with a clear error.
			var dynamicRedirectURI string
			if dynamicRedirectEnabled {
				hostname := r.Host
				if h, _, splitErr := net.SplitHostPort(r.Host); splitErr == nil {
					hostname = h
				}
				if !slices.Contains(normalizedAllowedHosts, strings.ToLower(hostname)) {
					if rlogger := loggermw.RequestLoggerFromContext(ctx); rlogger != nil {
						rlogger.WithFields(
							slog.F("oidc_rejected_reason", "host_not_in_allowlist"),
							slog.F("oidc_rejected_host", hostname),
						)
					}
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "OIDC login is not permitted from this host.",
						Detail:  fmt.Sprintf("Host %q is not in the OIDC redirect allowlist. Configure CODER_OIDC_REDIRECT_ALLOWED_HOSTS to include it.", hostname),
					})
					return
				}
				dynamicRedirectURI = buildDynamicRedirectURI(r, redirectDefaultScheme)
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

				authOpts := slices.Clone(opts)
				if sha256PKCESupported {
					verifier := oauth2.GenerateVerifier()
					authOpts = append(authOpts, oauth2.S256ChallengeOption(verifier))

					http.SetCookie(rw, cookieCfg.Apply(&http.Cookie{
						Name:     codersdk.OAuth2PKCEVerifier,
						Value:    verifier,
						Path:     "/",
						HttpOnly: true,
					}))
				}

				// Persist and inject the dynamic redirect_uri so the IdP
				// sends the user back to the same domain they started on,
				// and so the token exchange below uses the matching value.
				if dynamicRedirectURI != "" {
					http.SetCookie(rw, cookieCfg.Apply(&http.Cookie{
						Name:     codersdk.OAuth2RedirectURICookie,
						Value:    dynamicRedirectURI,
						Path:     "/",
						HttpOnly: true,
					}))
					authOpts = append(authOpts, oauth2.SetAuthURLParam("redirect_uri", dynamicRedirectURI))
				}

				http.Redirect(rw, r, config.AuthCodeURL(state, authOpts...), http.StatusTemporaryRedirect)
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

			exchangeOpts := make([]oauth2.AuthCodeOption, 0)
			if sha256PKCESupported {
				pkceVerifier, err := r.Cookie(codersdk.OAuth2PKCEVerifier)
				if err != nil {
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "PKCE challenge must be provided.",
					})
					return
				}
				exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(pkceVerifier.Value))
			}

			// RFC 6749 section 4.1.3: the redirect_uri included in the token
			// exchange must match the one sent in the authorization request.
			// When the dynamic-redirect path is in use, the original value was
			// stashed in a cookie; replay it here.
			//
			// Defense in depth: we do not blindly forward the cookie value to
			// the IdP. We recompute the expected redirect_uri from the (already
			// allowlist-validated) request Host, then require the cookie to
			// match. This guards against:
			//   - The cookie going missing (e.g. third-party cookie blocking)
			//     and silently falling back to the static redirect_uri, which
			//     would mismatch the authorization request and produce a
			//     confusing IdP rejection. Fail loudly here instead.
			//   - A tampered cookie pointing at a host the user did not
			//     authenticate on. The IdP allowlist would normally catch this,
			//     but we should not depend on it.
			if dynamicRedirectEnabled {
				redirectCookie, err := r.Cookie(codersdk.OAuth2RedirectURICookie)
				if err != nil || redirectCookie.Value == "" {
					if rlogger := loggermw.RequestLoggerFromContext(ctx); rlogger != nil {
						rlogger.WithFields(slog.F("oidc_rejected_reason", "missing_redirect_uri_cookie"))
					}
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: fmt.Sprintf("Cookie %q must be provided for the OIDC callback when CODER_OIDC_REDIRECT_ALLOWED_HOSTS is configured.", codersdk.OAuth2RedirectURICookie),
					})
					return
				}
				expectedRedirectURI := buildDynamicRedirectURI(r, redirectDefaultScheme)
				if redirectCookie.Value != expectedRedirectURI {
					if rlogger := loggermw.RequestLoggerFromContext(ctx); rlogger != nil {
						rlogger.WithFields(
							slog.F("oidc_rejected_reason", "redirect_uri_cookie_mismatch"),
							slog.F("oidc_cookie_redirect_uri", redirectCookie.Value),
							slog.F("oidc_expected_redirect_uri", expectedRedirectURI),
						)
					}
					httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
						Message: "OIDC redirect_uri cookie does not match the current request host.",
					})
					return
				}
				exchangeOpts = append(exchangeOpts, oauth2.SetAuthURLParam("redirect_uri", redirectCookie.Value))
			}

			oauthToken, err := config.Exchange(ctx, code, exchangeOpts...)
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
	return extractOAuth2ProviderAppBase(db, &codersdkErrorWriter{})
}

// ExtractOAuth2ProviderAppWithOAuth2Errors is the same as ExtractOAuth2ProviderApp but
// returns OAuth2-compliant errors instead of generic API errors. This should be used
// for OAuth2 endpoints like /oauth2/tokens.
func ExtractOAuth2ProviderAppWithOAuth2Errors(db database.Store) func(http.Handler) http.Handler {
	return extractOAuth2ProviderAppBase(db, &oauth2ErrorWriter{})
}

// errorWriter interface abstracts different error response formats.
// This uses the Strategy pattern to avoid a control flag (useOAuth2Errors bool)
// which was flagged by the linter as an anti-pattern. Instead of duplicating
// the entire function logic or using a boolean parameter, we inject the error
// handling behavior through this interface.
type errorWriter interface {
	writeMissingClientID(ctx context.Context, rw http.ResponseWriter)
	writeInvalidClientID(ctx context.Context, rw http.ResponseWriter, err error)
	writeClientNotFound(ctx context.Context, rw http.ResponseWriter)
}

// codersdkErrorWriter writes standard codersdk errors for general API endpoints
type codersdkErrorWriter struct{}

func (*codersdkErrorWriter) writeMissingClientID(ctx context.Context, rw http.ResponseWriter) {
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Missing OAuth2 client ID.",
	})
}

func (*codersdkErrorWriter) writeInvalidClientID(ctx context.Context, rw http.ResponseWriter, err error) {
	httpapi.Write(ctx, rw, http.StatusUnauthorized, codersdk.Response{
		Message: "Invalid OAuth2 client ID.",
		Detail:  err.Error(),
	})
}

func (*codersdkErrorWriter) writeClientNotFound(ctx context.Context, rw http.ResponseWriter) {
	// Management API endpoints return 404 for missing OAuth2 apps (proper REST semantics).
	// This differs from OAuth2 protocol endpoints which return 401 "invalid_client" per RFC 6749.
	// Returning 401 here would trigger the frontend's automatic logout interceptor when React Query
	// refetches a deleted app, incorrectly logging out users who just deleted their own OAuth2 apps.
	httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
		Message: "OAuth2 application not found.",
	})
}

// oauth2ErrorWriter writes OAuth2-compliant errors for OAuth2 endpoints
type oauth2ErrorWriter struct{}

func (*oauth2ErrorWriter) writeMissingClientID(ctx context.Context, rw http.ResponseWriter) {
	httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, "Missing client_id parameter")
}

func (*oauth2ErrorWriter) writeInvalidClientID(ctx context.Context, rw http.ResponseWriter, _ error) {
	httpapi.WriteOAuth2Error(ctx, rw, http.StatusUnauthorized, codersdk.OAuth2ErrorCodeInvalidClient, "The client credentials are invalid")
}

func (*oauth2ErrorWriter) writeClientNotFound(ctx context.Context, rw http.ResponseWriter) {
	httpapi.WriteOAuth2Error(ctx, rw, http.StatusUnauthorized, codersdk.OAuth2ErrorCodeInvalidClient, "The client credentials are invalid")
}

// extractOAuth2ProviderAppBase is the internal implementation that uses the strategy pattern
// instead of a control flag to handle different error formats.
func extractOAuth2ProviderAppBase(db database.Store, errWriter errorWriter) func(http.Handler) http.Handler {
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
					// RFC 6749 §2.3.1: confidential clients may authenticate via
					// HTTP Basic where the username is the client_id.
					if user, _, ok := r.BasicAuth(); ok && user != "" {
						paramAppID = user
					}
				}
				if paramAppID == "" {
					errWriter.writeMissingClientID(ctx, rw)
					return
				}

				var err error
				appID, err = uuid.Parse(paramAppID)
				if err != nil {
					errWriter.writeInvalidClientID(ctx, rw, err)
					return
				}
			}

			app, err := db.GetOAuth2ProviderAppByID(ctx, appID)
			if httpapi.Is404Error(err) {
				errWriter.writeClientNotFound(ctx, rw)
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

// buildDynamicRedirectURI constructs the OIDC redirect_uri from the incoming
// request, used when CODER_OIDC_REDIRECT_ALLOWED_HOSTS is configured.
//
// The scheme is taken from the configured AccessURL (passed in as
// defaultScheme by the caller) rather than from the request itself. Real
// deployments that use this feature always sit behind a TLS-terminating
// proxy, and some such proxies set X-Forwarded-Proto to the inner-hop
// scheme (e.g. "http" between proxy and coderd) instead of the original
// client-facing scheme. Trusting the request for scheme would produce a
// redirect_uri the IdP rejects. AccessURL is the operator-defined source
// of truth and is the same value the static OIDC path uses, so reusing
// it keeps the dynamic and static paths byte-for-byte consistent.
//
// The callback path is whatever path the middleware is mounted at, which
// today is /api/v2/users/oidc/callback for OIDC.
func buildDynamicRedirectURI(r *http.Request, defaultScheme string) string {
	u := url.URL{
		Scheme: defaultScheme,
		Host:   r.Host,
		Path:   r.URL.Path,
	}
	return u.String()
}
