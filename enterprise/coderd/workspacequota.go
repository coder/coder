package coderd

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/google/uuid"

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

func (c *Committer) CommitQuota(
	ctx context.Context, request *proto.CommitQuotaRequest,
) (*proto.CommitQuotaResponse, error) {
	jobID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, err
	}

	build, err := c.Database.GetWorkspaceBuildByJobID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	workspace, err := c.Database.GetWorkspaceByID(ctx, build.WorkspaceID)
	if err != nil {
		return nil, err
	}

	var (
		consumed  int64
		allowance int64
		permit    bool
	)
	err = c.Database.InTxOpts(func(s database.Store) error {
		var err error
		consumed, err = s.GetQuotaConsumedForUser(ctx, workspace.OwnerID)
		if err != nil {
			return err
		}

		allowance, err = s.GetQuotaAllowanceForUser(ctx, workspace.OwnerID)
		if err != nil {
			return err
		}

		newConsumed := int64(request.Cost) + consumed
		if newConsumed > allowance {
			return nil
		}

		_, err = s.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
			ID:   build.ID,
			Cost: request.Cost,
		})
		if err != nil {
			return err
		}
		permit = true
		consumed = newConsumed
		return nil
	}, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return nil, err
	}

	return &proto.CommitQuotaResponse{
		Ok:              permit,
		CreditsConsumed: int32(consumed),
		TotalAllowance:  int32(allowance),
	}, nil
}

func (api *API) workspaceQuota(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.AGPL.Authorize(r, rbac.ActionRead, rbac.ResourceUser) {
		httpapi.ResourceNotFound(rw)
		return
	}

	quotaAllowance, err := api.Database.GetQuotaAllowanceForUser(r.Context(), user.ID)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get allowance",
			Detail:  err.Error(),
		})
		return
	}

	quotaConsumed, err := api.Database.GetQuotaConsumedForUser(r.Context(), user.ID)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get consumed",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.WorkspaceQuota{
		CreditsConsumed: int(quotaConsumed),
		TotalAllowance:  int(quotaAllowance),
	})
}
