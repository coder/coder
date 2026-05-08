package httpmw

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// RequireTemplateBuilderEnabled returns middleware that responds with
// 404 when the template builder feature is disabled. This hides the
// feature entirely rather than returning 403, so clients cannot
// discover the endpoint exists when the feature is off.
func RequireTemplateBuilderEnabled(enabled *serpent.Bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled.Value() {
				httpapi.Write(r.Context(), w, http.StatusNotFound, codersdk.Response{
					Message: "Template builder is not enabled.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
