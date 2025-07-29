package httpmw

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// RequireExperiment returns middleware that checks if all required experiments are enabled.
// If any experiment is disabled, it returns a 403 Forbidden response with details about the missing experiments.
func RequireExperiment(experiments codersdk.Experiments, requiredExperiments ...codersdk.Experiment) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, experiment := range requiredExperiments {
				if !experiments.Enabled(experiment) {
					var experimentNames []string
					for _, exp := range requiredExperiments {
						experimentNames = append(experimentNames, string(exp))
					}

					// Print a message that includes the experiment names
					// even if some experiments are already enabled.
					var message string
					if len(requiredExperiments) == 1 {
						message = fmt.Sprintf("%s functionality requires enabling the '%s' experiment.",
							requiredExperiments[0].DisplayName(), requiredExperiments[0])
					} else {
						message = fmt.Sprintf("This functionality requires enabling the following experiments: %s",
							strings.Join(experimentNames, ", "))
					}

					httpapi.Write(r.Context(), w, http.StatusForbidden, codersdk.Response{
						Message: message,
					})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireExperimentWithDevBypass checks if ALL the given experiments are enabled,
// but bypasses the check in development mode (buildinfo.IsDev()).
func RequireExperimentWithDevBypass(experiments codersdk.Experiments, requiredExperiments ...codersdk.Experiment) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if buildinfo.IsDev() {
				next.ServeHTTP(w, r)
				return
			}

			RequireExperiment(experiments, requiredExperiments...)(next).ServeHTTP(w, r)
		})
	}
}
