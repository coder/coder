package oauth2provider

import (
	"net/http"
	"net/url"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/site"
)

// authorizeMW serves to remove some code from the primary authorize handler.
// It decides when to show the html allow page, and when to just continue.
func authorizeMW(accessURL *url.URL) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			app := httpmw.OAuth2ProviderApp(r)
			ua := httpmw.UserAuthorization(r.Context())

			// If this is a POST request, it means the user clicked the "Allow" button
			// on the consent form. Process the authorization.
			if r.Method == http.MethodPost {
				next.ServeHTTP(rw, r)
				return
			}

			// For GET requests, show the authorization consent page
			// TODO: For now only browser-based auth flow is officially supported but
			// in a future PR we should support a cURL-based flow where we output text
			// instead of HTML.

			callbackURL, err := url.Parse(app.CallbackURL)
			if err != nil {
				site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
					Status:       http.StatusInternalServerError,
					HideStatus:   false,
					Title:        "Internal Server Error",
					Description:  err.Error(),
					RetryEnabled: false,
					DashboardURL: accessURL.String(),
					Warnings:     nil,
				})
				return
			}

			// Extract the form parameters for two reasons:
			// 1. We need the redirect URI to build the cancel URI.
			// 2. Since validation will run once the user clicks "allow", it is
			//    better to validate now to avoid wasting the user's time clicking a
			//    button that will just error anyway.
			params, validationErrs, err := extractAuthorizeParams(r, callbackURL)
			if err != nil {
				errStr := make([]string, len(validationErrs))
				for i, err := range validationErrs {
					errStr[i] = err.Detail
				}
				site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
					Status:       http.StatusBadRequest,
					HideStatus:   false,
					Title:        "Invalid Query Parameters",
					Description:  "One or more query parameters are missing or invalid.",
					RetryEnabled: false,
					DashboardURL: accessURL.String(),
					Warnings:     errStr,
				})
				return
			}

			cancel := params.redirectURL
			cancelQuery := params.redirectURL.Query()
			cancelQuery.Add("error", "access_denied")
			cancel.RawQuery = cancelQuery.Encode()

			// Render the consent page with the current URL (no need to add redirected parameter)
			site.RenderOAuthAllowPage(rw, r, site.RenderOAuthAllowData{
				AppIcon:     app.Icon,
				AppName:     app.Name,
				CancelURI:   cancel.String(),
				RedirectURI: r.URL.String(),
				Username:    ua.FriendlyName,
			})
		})
	}
}
