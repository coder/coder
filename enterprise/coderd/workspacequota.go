package coderd

import (
	"context"
	"net/http"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd/proto"
)

type Committer struct {
	Database database.Store
}

func (_ *Committer) CommitQuota(
	ctx context.Context, request *proto.CommitQuotaRequest,
) (*proto.CommitQuotaResponse, error) {
	return &proto.CommitQuotaResponse{
		Ok:               false,
		TotalCredits:     10,
		CreditsAvailable: 0,
	}, nil
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

	// e := *api.AGPL.WorkspaceQuotaEnforcer.Load()
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.WorkspaceQuota{
		CreditsConsumed: len(workspaces),
		TotalCredits:    1,
	})
}
