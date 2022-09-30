package coderd

import (
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/workspacequota"
	"github.com/coder/coder/codersdk"
)

type enforcer struct {
	userWorkspaceLimit int
}

func NewEnforcer(userWorkspaceLimit int) workspacequota.Enforcer {
	return &enforcer{
		userWorkspaceLimit: userWorkspaceLimit,
	}
}

func (e *enforcer) UserWorkspaceLimit() int {
	return e.userWorkspaceLimit
}

func (e *enforcer) CanCreateWorkspace(count int) bool {
	if e.userWorkspaceLimit == 0 {
		return true
	}

	return count < e.userWorkspaceLimit
}

func (api *API) workspaceQuota(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.AGPL.Authorize(r, rbac.ActionRead, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	workspaces, err := api.Database.GetWorkspaces(r.Context(), database.GetWorkspacesParams{
		OwnerID: user.ID,
	})
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error fetching workspaces.",
			Detail:  err.Error(),
		})
		return
	}

	e := *api.AGPL.WorkspaceQuotaEnforcer.Load()
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.WorkspaceQuota{
		UserWorkspaceCount: len(workspaces),
		UserWorkspaceLimit: e.UserWorkspaceLimit(),
	})
}
