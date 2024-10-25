package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
)

type committer struct {
	Log      slog.Logger
	Database database.Store
}

func (c *committer) CommitQuota(
	ctx context.Context, request *proto.CommitQuotaRequest,
) (*proto.CommitQuotaResponse, error) {
	jobID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, err
	}

	nextBuild, err := c.Database.GetWorkspaceBuildByJobID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	workspace, err := c.Database.GetWorkspaceByID(ctx, nextBuild.WorkspaceID)
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
		consumed, err = s.GetQuotaConsumedForUser(ctx, database.GetQuotaConsumedForUserParams{
			OwnerID:        workspace.OwnerID,
			OrganizationID: workspace.OrganizationID,
		})
		if err != nil {
			return err
		}

		budget, err = s.GetQuotaAllowanceForUser(ctx, database.GetQuotaAllowanceForUserParams{
			UserID:         workspace.OwnerID,
			OrganizationID: workspace.OrganizationID,
		})
		if err != nil {
			return err
		}

		// If the new build will reduce overall quota consumption, then we
		// allow it even if the user is over quota.
		netIncrease := true
		prevBuild, err := s.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
			WorkspaceID: workspace.ID,
			BuildNumber: nextBuild.BuildNumber - 1,
		})
		if err == nil {
			netIncrease = request.DailyCost >= prevBuild.DailyCost
			c.Log.Debug(
				ctx, "previous build cost",
				slog.F("prev_cost", prevBuild.DailyCost),
				slog.F("next_cost", request.DailyCost),
				slog.F("net_increase", netIncrease),
			)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		newConsumed := int64(request.DailyCost) + consumed
		if newConsumed > budget && netIncrease {
			c.Log.Debug(
				ctx, "over quota, rejecting",
				slog.F("prev_consumed", consumed),
				slog.F("next_consumed", newConsumed),
				slog.F("budget", budget),
			)
			return nil
		}

		err = s.UpdateWorkspaceBuildCostByID(ctx, database.UpdateWorkspaceBuildCostByIDParams{
			ID:        nextBuild.ID,
			DailyCost: request.DailyCost,
		})
		if err != nil {
			return err
		}
		permit = true
		consumed = newConsumed
		return nil
	}, &database.TxOptions{
		Isolation:    sql.LevelSerializable,
		TxIdentifier: "commit_quota",
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

// @Summary Get workspace quota by user deprecated
// @ID get-workspace-quota-by-user-deprecated
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.WorkspaceQuota
// @Router /workspace-quota/{user} [get]
// @Deprecated this endpoint will be removed, use /organizations/{organization}/members/{user}/workspace-quota instead
func (api *API) workspaceQuotaByUser(rw http.ResponseWriter, r *http.Request) {
	defaultOrg, err := api.Database.GetDefaultOrganization(r.Context())
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	// defer to the new endpoint using default org as the organization
	chi.RouteContext(r.Context()).URLParams.Add("organization", defaultOrg.ID.String())
	mw := httpmw.ExtractOrganizationParam(api.Database)
	mw(http.HandlerFunc(api.workspaceQuota)).ServeHTTP(rw, r)
}

// @Summary Get workspace quota by user
// @ID get-workspace-quota-by-user
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, name, or me"
// @Param organization path string true "Organization ID" format(uuid)
// @Success 200 {object} codersdk.WorkspaceQuota
// @Router /organizations/{organization}/members/{user}/workspace-quota [get]
func (api *API) workspaceQuota(rw http.ResponseWriter, r *http.Request) {
	var (
		organization = httpmw.OrganizationParam(r)
		user         = httpmw.UserParam(r)
	)

	licensed := api.Entitlements.Enabled(codersdk.FeatureTemplateRBAC)

	// There are no groups and thus no allowance if RBAC isn't licensed.
	var quotaAllowance int64 = -1
	if licensed {
		var err error
		quotaAllowance, err = api.Database.GetQuotaAllowanceForUser(r.Context(), database.GetQuotaAllowanceForUserParams{
			UserID:         user.ID,
			OrganizationID: organization.ID,
		})
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to get allowance",
				Detail:  err.Error(),
			})
			return
		}
	}

	quotaConsumed, err := api.Database.GetQuotaConsumedForUser(r.Context(), database.GetQuotaConsumedForUserParams{
		OwnerID:        user.ID,
		OrganizationID: organization.ID,
	})
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
