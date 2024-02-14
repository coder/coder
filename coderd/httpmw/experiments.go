package httpmw

import (
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func RequireExperiment(experiments codersdk.Experiments, experiment codersdk.Experiment) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !experiments.Enabled(experiment) {
				httpapi.Write(r.Context(), w, http.StatusForbidden, codersdk.Response{
					Message: fmt.Sprintf("Experiment '%s' is required but not enabled", experiment),
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
