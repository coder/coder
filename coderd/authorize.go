package coderd

import (
	"net/http"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
)

func (api *api) Authorize(rw http.ResponseWriter, r *http.Request, action rbac.Action, object rbac.Object) bool {
	roles := httpmw.UserRoles(r)
	err := api.Authorizer.ByRoleName(r.Context(), roles.ID.String(), roles.Roles, action, object)
	if err != nil {
		httpapi.Write(rw, http.StatusForbidden, httpapi.Response{
			Message: err.Error(),
		})

		// Log the errors for debugging
		internalError := new(rbac.UnauthorizedError)
		logger := api.Logger
		if xerrors.As(err, internalError) {
			logger = api.Logger.With(slog.F("internal", internalError.Internal()))
		}
		// Log information for debugging. This will be very helpful
		// in the early days
		logger.Warn(r.Context(), "unauthorized",
			slog.F("roles", roles.Roles),
			slog.F("user_id", roles.ID),
			slog.F("username", roles.Username),
			slog.F("route", r.URL.Path),
			slog.F("action", action),
			slog.F("object", object),
		)

		return false
	}
	return true
}
