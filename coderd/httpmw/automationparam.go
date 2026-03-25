package httpmw

import (
	"context"
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type automationParamContextKey struct{}

// AutomationParam returns the automation from the
// ExtractAutomationParam handler.
func AutomationParam(r *http.Request) database.Automation {
	automation, ok := r.Context().Value(automationParamContextKey{}).(database.Automation)
	if !ok {
		panic("developer error: automation param middleware not provided")
	}
	return automation
}

// ExtractAutomationParam grabs an automation from the "automation" URL
// parameter.
func ExtractAutomationParam(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			automationID, parsed := ParseUUIDParam(rw, r, "automation")
			if !parsed {
				return
			}

			automation, err := db.GetAutomationByID(ctx, automationID)
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			if err != nil {
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error fetching automation.",
					Detail:  err.Error(),
				})
				return
			}

			ctx = context.WithValue(ctx, automationParamContextKey{}, automation)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
