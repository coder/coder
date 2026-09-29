package coderd

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
)

// quotaLockTimeout bounds how long a quota commit waits for an earlier commit
// for the same owner and organization. Holders keep the lock for a few quick
// queries, so a wait this long means a holder is stuck or the database is
// overloaded.
const quotaLockTimeout = 30 * time.Second

type committer struct {
	Log      slog.Logger
	Database database.Store
	// lockTimeout overrides quotaLockTimeout when set, for tests.
	lockTimeout time.Duration
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

	lockTimeout := quotaLockTimeout
	if c.lockTimeout > 0 {
		lockTimeout = c.lockTimeout
	}
	var (
		consumed     int64
		budget       int64
		permit       bool
		lockTimedOut bool
	)
	err = c.Database.InTx(func(s database.Store) error {
		// InTx only retries SERIALIZABLE transactions, but reset anyway so the
		// response always comes from the attempt that committed.
		consumed, budget, permit, lockTimedOut = 0, 0, false, false

		// Quota commits for the same owner and organization take turns. The
		// lock is held until commit, and READ COMMITTED gives each statement
		// below a fresh snapshot, so the reads include every cost committed
		// by the previous lock holder.
		lockCtx, cancel := context.WithTimeout(ctx, lockTimeout)
		err := s.AcquireLock(lockCtx, database.WorkspaceQuotaLockID(workspace.OwnerID, workspace.OrganizationID))
		cancel()
		// Check the deadline even when AcquireLock succeeded. The driver can
		// close the connection once the deadline passes, so a lock granted
		// at that moment may not be usable.
		if errors.Is(lockCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			lockTimedOut = true
			return xerrors.New("workspace quota lock wait timed out")
		}
		if err != nil {
			return xerrors.Errorf("acquire workspace quota lock: %w", err)
		}

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
		Isolation:    sql.LevelReadCommitted,
		TxIdentifier: "commit_quota",
	})
	if lockTimedOut {
		// InTx's error also reports the failed rollback on the closed
		// connection, which reads like a database problem.
		return nil, xerrors.Errorf("timed out after %s waiting for workspace quota lock", lockTimeout)
	}
	if err != nil {
		return nil, err
	}

	return &proto.CommitQuotaResponse{
		Ok: permit,
		// #nosec G115 - Safe conversion as quota credits consumed value is expected to be within int32 range
		CreditsConsumed: int32(consumed),
		// #nosec G115 - Safe conversion as quota budget value is expected to be within int32 range
		Budget: int32(budget),
	}, nil
}

// @Summary Get workspace quota by user deprecated
// @ID get-workspace-quota-by-user-deprecated
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user path string true "User ID, name, or me"
// @Success 200 {object} codersdk.WorkspaceQuota
// @Router /api/v2/workspace-quota/{user} [get]
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
// @Router /api/v2/organizations/{organization}/members/{user}/workspace-quota [get]
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
