package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func (api *API) workspaceExecutionSession(rw http.ResponseWriter, r *http.Request, action policy.Action) (database.WorkspaceExecutionSession, bool) {
	id, ok := httpmw.ParseUUIDParam(rw, r, "session")
	if !ok {
		return database.WorkspaceExecutionSession{}, false
	}
	session, err := api.Database.GetWorkspaceExecutionSessionByID(r.Context(), id)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
		} else {
			httpapi.InternalServerError(rw, err)
		}
		return database.WorkspaceExecutionSession{}, false
	}
	if session.OrganizationID != httpmw.OrganizationParam(r).ID {
		httpapi.ResourceNotFound(rw)
		return database.WorkspaceExecutionSession{}, false
	}
	object := rbac.ResourceWorkspaceExecution.WithID(session.ID).InOrg(session.OrganizationID).WithOwner(session.OwnerID.String())
	if !api.Authorize(r, action, object) {
		httpapi.ResourceNotFound(rw)
		return database.WorkspaceExecutionSession{}, false
	}
	return session, true
}
