package coderd

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd/proto"
)

type committer struct {
	Database database.Store
}

func (c *committer) CommitQuota(
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
		consumed int64
		budget   int64
		permit   bool
	)
	err = c.Database.InTx(func(s database.Store) error {
		var err error
		consumed, err = s.GetQuotaConsumedForUser(ctx, workspace.OwnerID)
		if err != nil {
			return err
		}

		budget, err = s.GetQuotaAllowanceForUser(ctx, workspace.OwnerID)
		if err != nil {
			return err
		}

		// If the new build will reduce overall quota consumption, then we
		// allow it even if the user is over quota.
		netIncrease := true
		previousBuild, err := s.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
			WorkspaceID: workspace.ID,
			BuildNumber: build.BuildNumber - 1,
		})
		if err == nil {
			if build.DailyCost < previousBuild.DailyCost {
				netIncrease = false
			}
		} else if !xerrors.Is(err, sql.ErrNoRows) {
			return err
		}

		newConsumed := int64(request.DailyCost) + consumed
		if newConsumed > budget && netIncrease {
			return nil
		}

		_, err = s.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
			ID:        build.ID,
			DailyCost: request.DailyCost,
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
		Budget:          int32(budget),
	}, nil
}

// @Summary Get workspace quota by user
// @ID get-workspace-quota-by-user
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.WorkspaceQuota
// @Router /workspace-quota/{user} [get]
func (api *API) workspaceQuota(rw http.ResponseWriter, r *http.Request) {
	user := httpmw.UserParam(r)

	if !api.AGPL.Authorize(r, rbac.ActionRead, user) {
		httpapi.ResourceNotFound(rw)
		return
	}

	api.entitlementsMu.RLock()
	licensed := api.entitlements.Features[codersdk.FeatureTemplateRBAC].Enabled
	api.entitlementsMu.RUnlock()

	// There are no groups and thus no allowance if RBAC isn't licensed.
	var quotaAllowance int64 = -1
	if licensed {
		var err error
		quotaAllowance, err = api.Database.GetQuotaAllowanceForUser(r.Context(), user.ID)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get allowance",
				Detail:  err.Error(),
			})
			return
		}
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
		Budget:          int(quotaAllowance),
	})
}
