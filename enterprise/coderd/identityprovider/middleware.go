package identityprovider

import (
	"net/http"
	"net/url"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
)

// authorizeMW serves to remove some code from the primary authorize handler.
// It decides when to show the html allow page, and when to just continue.
func authorizeMW(accessURL *url.URL) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get(httpmw.OriginHeader)
			originU, err := url.Parse(origin)
			if err != nil {
				httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid origin header.",
					Detail:  err.Error(),
				})
				return
			}

			referer := r.Referer()
			refererU, err := url.Parse(referer)
			if err != nil {
				httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid referer header.",
					Detail:  err.Error(),
				})
				return
			}

			app := httpmw.OAuth2ProviderApp(r)
			ua := httpmw.UserAuthorization(r)

			// url.Parse() allows empty URLs, which is fine because the origin is not
			// always set by browsers (or other tools like cURL).  If the origin does
			// exist, we will make sure it matches.  We require `referer` to be set at
			// a minimum in order to detect whether "allow" has been pressed, however.
			cameFromSelf := (origin == "" || originU.Hostname() == accessURL.Hostname()) &&
				refererU.Hostname() == accessURL.Hostname() &&
				refererU.Path == "/login/oauth2/authorize"

			// If we were redirected here from this same page it means the user
			// pressed the allow button so defer to the authorize handler which
			// generates the code, otherwise show the HTML allow page.
			// TODO: Skip this step if the user has already clicked allow before, and
			//       we can just reuse the token.
			if cameFromSelf {
				next.ServeHTTP(rw, r)
				return
			}

			// TODO: For now only browser-based auth flow is officially supported but
			// in a future PR we should support a cURL-based flow where we output text
			// instead of HTML.
			if r.URL.Query().Get("redirected") != "" {
				// When the user first comes into the page, referer might be blank which
				// is OK.  But if they click "allow" and their browser has *still* not
				// sent the referer header, we have no way of telling whether they
				// actually clicked the button.  "Redirected" means they *might* have
				// pressed it, but it could also mean an app added it for them as part
				// of their redirect, so we cannot use it as a replacement for referer
				// and the best we can do is error.
				if referer == "" {
					site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
						Status:       http.StatusInternalServerError,
						HideStatus:   false,
						Title:        "Referer header missing",
						Description:  "We cannot continue authorization because your client has not sent the referer header.",
						RetryEnabled: false,
						DashboardURL: accessURL.String(),
						Warnings:     nil,
					})
					return
				}
				site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
					Status:       http.StatusInternalServerError,
					HideStatus:   false,
					Title:        "Oauth Redirect Loop",
					Description:  "Oauth redirect loop detected.",
					RetryEnabled: false,
					DashboardURL: accessURL.String(),
					Warnings:     nil,
				})
				return
			}

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

			redirect := r.URL
			vals := redirect.Query()
			vals.Add("redirected", "true") // For loop detection.
			r.URL.RawQuery = vals.Encode()
			site.RenderOAuthAllowPage(rw, r, site.RenderOAuthAllowData{
				AppIcon:     app.Icon,
				AppName:     app.Name,
				CancelURI:   cancel.String(),
				RedirectURI: r.URL.String(),
				Username:    ua.ActorName,
			})
		})
	}
}
