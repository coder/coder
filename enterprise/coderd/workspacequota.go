package coderd

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/quartz"
)

// quotaLockTimeout bounds how long a quota commit waits for an admission
// slot, pooled connections, and earlier commits for the same owner and
// organization. It doesn't bound BEGIN, COMMIT, or a running transaction.
const quotaLockTimeout = 30 * time.Second

// A commit that finds the lock held retries after a full-jitter delay from
// 25ms, doubling to 1s. A holder normally keeps the lock for milliseconds,
// and the cap keeps each waiter to about two attempts a second.
const (
	quotaBackoffBase = 25 * time.Millisecond
	quotaBackoffMax  = time.Second
)

var errQuotaLockTimeout = xerrors.New("workspace quota lock timeout")

// quotaAdmissionLimit returns how many quota transactions a replica runs at
// once: a quarter of its connection pool, from 1 to 8. database/sql treats
// zero as unlimited.
func quotaAdmissionLimit(maxOpenConns int) int64 {
	if maxOpenConns <= 0 {
		return 8
	}
	return int64(min(max(maxOpenConns/4, 1), 8))
}

type committer struct {
	Log      slog.Logger
	Database database.Store
	// admission must be shared by every committer on the replica.
	admission *semaphore.Weighted
	clock     quartz.Clock
	// lockTimeout overrides quotaLockTimeout when set, for tests.
	lockTimeout time.Duration
}

// CommitQuota commits a build's cost if its owner has the budget for it.
//
// Commits for the same owner and organization take turns through an
// advisory lock. An attempt that finds it held commits its empty
// transaction, returns its connection and admission slot, and retries after
// a backoff. Waiters poll rather than queue, so a newer commit can win over
// an older one, and the lock can sit idle for up to one backoff after a
// release. Slots are shared across owners, so slow commits delay others on
// the replica.
//
// CommitQuota must not run within a transaction.
func (c *committer) CommitQuota(
	ctx context.Context, request *proto.CommitQuotaRequest,
) (*proto.CommitQuotaResponse, error) {
	jobID, err := uuid.Parse(request.JobId)
	if err != nil {
		return nil, err
	}

	// admitCtx ends waits for a slot, a connection, or a retry. Statements
	// use ctx, so the timeout never interrupts a transaction.
	lockTimeout := cmp.Or(c.lockTimeout, quotaLockTimeout)
	admitCtx, cancelAdmit := context.WithCancelCause(ctx)
	defer cancelAdmit(nil)
	admitTimer := c.clock.AfterFunc(lockTimeout, func() { cancelAdmit(errQuotaLockTimeout) }, "commitQuota", "admission")
	defer admitTimer.Stop()

	var (
		nextBuild database.WorkspaceBuild
		workspace database.Workspace
	)
	err = c.inQuotaTx(admitCtx, "commit_quota_lookup", func(s database.Store) error {
		var err error
		nextBuild, err = s.GetWorkspaceBuildByJobID(ctx, jobID)
		if err != nil {
			return err
		}
		workspace, err = s.GetWorkspaceByID(ctx, nextBuild.WorkspaceID)
		return err
	})
	if err != nil {
		return nil, c.timeoutError(ctx, admitCtx, err, lockTimeout, slog.F("job_id", jobID))
	}

	lockID := database.WorkspaceQuotaLockID(workspace.OwnerID, workspace.OrganizationID)
	fields := []slog.Field{
		slog.F("job_id", jobID),
		slog.F("workspace_id", workspace.ID),
		slog.F("owner_id", workspace.OwnerID),
		slog.F("organization_id", workspace.OrganizationID),
		slog.F("lock_id", lockID),
	}
	for miss := 0; ; miss++ {
		resp, busy, err := c.commitQuotaAttempt(ctx, admitCtx, request, nextBuild, workspace, lockID)
		if err != nil {
			return nil, c.timeoutError(ctx, admitCtx, err, lockTimeout, fields...)
		}
		if !busy {
			return resp, nil
		}
		// Only a miss is retried, since it read and wrote nothing. The delay
		// is never zero, so every retry goes through a clock timer.
		//nolint:gosec // Jitter does not need cryptographic randomness.
		delay := time.Duration(rand.Int64N(int64(min(quotaBackoffMax, quotaBackoffBase<<min(miss, 6))))) + 1
		timer := c.clock.NewTimer(delay, "commitQuota", "backoff")
		select {
		case <-timer.C:
		case <-admitCtx.Done():
			timer.Stop()
			return nil, c.timeoutError(ctx, admitCtx, admitCtx.Err(), lockTimeout, fields...)
		}
	}
}

// commitQuotaAttempt runs one quota transaction. It reports busy, with a nil
// error, when another commit holds the lock.
func (c *committer) commitQuotaAttempt(
	ctx, admitCtx context.Context,
	request *proto.CommitQuotaRequest,
	nextBuild database.WorkspaceBuild,
	workspace database.Workspace,
	lockID int64,
) (*proto.CommitQuotaResponse, bool, error) {
	var (
		consumed int64
		budget   int64
		permit   bool
		busy     bool
	)
	err := c.inQuotaTx(admitCtx, "commit_quota", func(s database.Store) error {
		// InTx only retries SERIALIZABLE transactions, but reset anyway so the
		// response always comes from the attempt that committed.
		consumed, budget, permit, busy = 0, 0, false, false

		// Quota commits for the same owner and organization take turns. The
		// lock is held until commit, and READ COMMITTED gives each statement
		// below a fresh snapshot, so the reads include every cost committed
		// by the previous lock holder. Higher isolation levels can keep a
		// snapshot taken before the lock was granted, so they can miss that
		// cost; SERIALIZABLE also retains serialization conflicts and retries.
		locked, err := s.TryAcquireLock(ctx, lockID)
		if err != nil {
			return xerrors.Errorf("try workspace quota lock: %w", err)
		}
		if !locked {
			busy = true
			return nil
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
	})
	if err != nil || busy {
		return nil, busy, err
	}

	return &proto.CommitQuotaResponse{
		Ok: permit,
		// #nosec G115 - Safe conversion as quota credits consumed value is expected to be within int32 range
		CreditsConsumed: int32(consumed),
		// #nosec G115 - Safe conversion as quota budget value is expected to be within int32 range
		Budget: int32(budget),
	}, false, nil
}

// inQuotaTx runs fn in a top-level READ COMMITTED transaction while holding
// an admission slot until the connection is back in the pool. admitCtx ends
// the waits for the slot and the connection. Nested transactions are
// rejected, since they would wait on the outer connection and could read
// its snapshot.
func (c *committer) inQuotaTx(admitCtx context.Context, id string, fn func(database.Store) error) error {
	if err := c.admission.Acquire(admitCtx, 1); err != nil {
		return xerrors.Errorf("wait for quota admission: %w", err)
	}
	defer c.admission.Release(1)
	return c.Database.InTx(fn, &database.TxOptions{
		Isolation:       sql.LevelReadCommitted,
		TxIdentifier:    id,
		CheckoutContext: admitCtx,
		RequireTopLevel: true,
	})
}

// timeoutError reports a commit whose wait ended at the lock timeout as
// retryable, and returns any other error unchanged. Statements use ctx, so
// while ctx is live only a wait on admitCtx can return a cancellation.
func (c *committer) timeoutError(ctx, admitCtx context.Context, err error, timeout time.Duration, fields ...slog.Field) error {
	if ctx.Err() != nil || !errors.Is(err, context.Canceled) || !errors.Is(context.Cause(admitCtx), errQuotaLockTimeout) {
		return err
	}
	c.Log.Warn(ctx, "workspace quota lock wait timed out; retry the build", append(fields, slog.F("timeout", timeout))...)
	return xerrors.Errorf("timed out after %s waiting for workspace quota lock; quota checks are taking too long, please retry the build", timeout)
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
