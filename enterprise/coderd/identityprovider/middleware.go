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
				// TODO: Curl requests will not have this. One idea is to always show
				// html here??
				httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid or missing origin header.",
					Detail:  err.Error(),
				})
				return
			}

			referer := r.Referer()
			refererU, err := url.Parse(referer)
			if err != nil {
				httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid or missing referer header.",
					Detail:  err.Error(),
				})
				return
			}

			app := httpmw.OAuth2ProviderApp(r)
			ua := httpmw.UserAuthorization(r)

			// If the request comes from outside, then we show the html allow page.
			// TODO: Skip this step if the user has already clicked allow before, and
			// we can just reuse the token.
			if originU.Hostname() != accessURL.Hostname() && refererU.Path != "/login/oauth2/authorize" {
				if r.URL.Query().Get("redirected") != "" {
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

				// Extract the form parameters for two reasons:
				// 1. We need the redirect URI to build the cancel URI.
				// 2. Since validation will run once the user clicks "allow", it is
				//    better to validate now to avoid wasting the user's time clicking a
				//    button that will just error anyway.
				params, errs, err := extractAuthorizeParams(r, app.CallbackURL)
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
				if len(errs) > 0 {
					errStr := make([]string, len(errs))
					for i, err := range errs {
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
				vals.Add("redirected", "true")
				r.URL.RawQuery = vals.Encode()
				site.RenderOAuthAllowPage(rw, r, site.RenderOAuthAllowData{
					AppIcon:     app.Icon,
					AppName:     app.Name,
					CancelURI:   cancel.String(),
					RedirectURI: r.URL.String(),
					Username:    ua.ActorName,
				})
				return
			}

			next.ServeHTTP(rw, r)
		})
	}
}
