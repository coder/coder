package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
)

func agentHoursLicense(allocation int64) *coderdenttest.LicenseOptions {
	return (&coderdenttest.LicenseOptions{FeatureSet: codersdk.FeatureSetPremium}).
		AgentRuntimeHours(allocation, nil, nil)
}

func requireAgentHoursStatus(t *testing.T, err error, status int) *codersdk.Error {
	t.Helper()
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, status, sdkErr.StatusCode(), sdkErr.Error())
	return sdkErr
}

func allotmentReq(bps int32) codersdk.UpsertAgentHoursAllotmentRequest {
	return codersdk.UpsertAgentHoursAllotmentRequest{AllotmentBps: bps}
}

func TestAgentHoursAllotmentsFeatureGate(t *testing.T) {
	t.Parallel()

	// A Premium license without Agent Hours claims leaves the feature
	// disabled.
	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{FeatureSet: codersdk.FeatureSetPremium},
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:gocritic // The feature gate must reject even the owner.
	group, err := client.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "gated"})
	require.NoError(t, err)

	_, err = client.AgentHoursOrganizationAllotments(ctx)
	requireAgentHoursStatus(t, err, http.StatusForbidden)
	_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(100))
	requireAgentHoursStatus(t, err, http.StatusForbidden)
	err = client.DeleteAgentHoursOrganizationAllotment(ctx, owner.OrganizationID)
	requireAgentHoursStatus(t, err, http.StatusForbidden)
	_, err = client.AgentHoursGroupAllotments(ctx, owner.OrganizationID)
	requireAgentHoursStatus(t, err, http.StatusForbidden)
	_, err = client.UpsertAgentHoursGroupAllotment(ctx, group.ID, allotmentReq(100))
	requireAgentHoursStatus(t, err, http.StatusForbidden)
	err = client.DeleteAgentHoursGroupAllotment(ctx, group.ID)
	requireAgentHoursStatus(t, err, http.StatusForbidden)
}

func TestAgentHoursAllotmentNonIntegerRejected(t *testing.T) {
	t.Parallel()

	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: agentHoursLicense(1000),
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:gocritic // Organization allotments are owner-only.
	group, err := client.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "non-integer"})
	require.NoError(t, err)

	for _, path := range []string{
		fmt.Sprintf("/api/v2/organizations/%s/agent-hours/allotment", owner.OrganizationID),
		fmt.Sprintf("/api/v2/groups/%s/agent-hours/allotment", group.ID),
	} {
		for _, body := range []string{`{"allotment_bps":12.5}`, `{"allotment_bps":"5000"}`, `[5000]`, `5000`} {
			//nolint:gocritic // Organization allotments are owner-only.
			res, err := client.Request(ctx, http.MethodPut, path, json.RawMessage(body))
			require.NoError(t, err)
			sdkErr := requireAgentHoursStatus(t, codersdk.ReadBodyAsError(res), http.StatusBadRequest)
			_ = res.Body.Close()
			require.Equal(t, []codersdk.ValidationError{{
				Field:  "allotment_bps",
				Detail: "Must be an integer between 1 and 10000.",
			}}, sdkErr.Validations, "%s %s", path, body)
		}
	}
}

func TestAgentHoursOrganizationAllotments(t *testing.T) {
	t.Parallel()

	t.Run("UnlimitedAllocation", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: agentHoursLicense(-1),
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Organization allotments are owner-only.
		_, err := client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(10000))
		require.NoError(t, err)
		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(10001))
		requireAgentHoursStatus(t, err, http.StatusBadRequest)
	})

	t.Run("CRUDAndCap", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: agentHoursLicense(1000),
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		other := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		//nolint:gocritic // Organization allotments are owner-only.
		created, err := client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(5950))
		require.NoError(t, err)
		require.Equal(t, owner.OrganizationID, created.OrganizationID)
		require.EqualValues(t, 5950, created.AllotmentBps)
		require.NotEmpty(t, created.OrganizationName)

		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, other.ID, allotmentReq(0))
		requireAgentHoursStatus(t, err, http.StatusBadRequest)

		sdkErr := requireAgentHoursStatus(t,
			func() error {
				_, err := client.UpsertAgentHoursOrganizationAllotment(ctx, other.ID, allotmentReq(5000))
				return err
			}(), http.StatusConflict)
		require.Equal(t, []codersdk.ValidationError{{Field: "allotment_bps", Detail: "Must not exceed 4050."}}, sdkErr.Validations)

		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, other.ID, allotmentReq(4000))
		require.NoError(t, err)

		// 10000 only fits once the organization's own 5950 is excluded.
		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, other.ID, allotmentReq(10000))
		sdkErr = requireAgentHoursStatus(t, err, http.StatusConflict)
		require.Equal(t, "Only 0.5% is unallotted, so this allotment can be at most 40.5%.", sdkErr.Detail)
		require.NoError(t, client.DeleteAgentHoursOrganizationAllotment(ctx, owner.OrganizationID))
		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, other.ID, allotmentReq(10000))
		require.NoError(t, err)

		list, err := client.AgentHoursOrganizationAllotments(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, other.ID, list[0].OrganizationID)
		require.EqualValues(t, 10000, list[0].AllotmentBps)

		err = client.DeleteAgentHoursOrganizationAllotment(ctx, owner.OrganizationID)
		requireAgentHoursStatus(t, err, http.StatusNotFound)
	})

	t.Run("OrgAdminCannotWrite", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: agentHoursLicense(1000),
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

		_, err := orgAdmin.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(100))
		requireAgentHoursStatus(t, err, http.StatusForbidden)
		err = orgAdmin.DeleteAgentHoursOrganizationAllotment(ctx, owner.OrganizationID)
		requireAgentHoursStatus(t, err, http.StatusForbidden)
		_, err = orgAdmin.AgentHoursOrganizationAllotments(ctx)
		requireAgentHoursStatus(t, err, http.StatusForbidden)

		// The organization's own share is readable as the base of its group
		// allotments.
		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(2500))
		require.NoError(t, err)
		groups, err := orgAdmin.AgentHoursGroupAllotments(ctx, owner.OrganizationID)
		require.NoError(t, err)
		require.NotNil(t, groups.OrganizationAllotmentBps)
		require.EqualValues(t, 2500, *groups.OrganizationAllotmentBps)
	})

	t.Run("DeletedOrganizationExcluded", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: agentHoursLicense(1000),
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		other := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		//nolint:gocritic // Organization allotments are owner-only.
		_, err := client.UpsertAgentHoursOrganizationAllotment(ctx, other.ID, allotmentReq(6000))
		require.NoError(t, err)
		require.NoError(t, client.DeleteOrganization(ctx, other.ID.String()))

		list, err := client.AgentHoursOrganizationAllotments(ctx)
		require.NoError(t, err)
		require.Empty(t, list)
		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(10000))
		require.NoError(t, err)
		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, other.ID, allotmentReq(100))
		requireAgentHoursStatus(t, err, http.StatusNotFound)

		// The Everyone group outlives its soft-deleted organization.
		_, err = client.UpsertAgentHoursGroupAllotment(ctx, other.ID, allotmentReq(100))
		requireAgentHoursStatus(t, err, http.StatusNotFound)
		err = client.DeleteAgentHoursGroupAllotment(ctx, other.ID)
		requireAgentHoursStatus(t, err, http.StatusNotFound)
	})

	// The advisory lock serializes writers: a writer queued behind an
	// in-flight transaction must see that transaction's row once it commits.
	t.Run("ConcurrentWriterSeesCommittedTotal", func(t *testing.T) {
		t.Parallel()
		db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options:        &coderdtest.Options{Database: db, Pubsub: ps},
			LicenseOptions: agentHoursLicense(1000),
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		other := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		holding := make(chan struct{})
		release := make(chan struct{})
		releaseLock := sync.OnceFunc(func() { close(release) })
		t.Cleanup(releaseLock)
		holderErr := make(chan error, 1)
		go func() {
			holderErr <- db.InTx(func(tx database.Store) error {
				if err := tx.AcquireLock(ctx, database.LockIDAgentHoursOrganizationAllotments); err != nil {
					return err
				}
				if _, err := tx.UpsertAgentHoursOrganizationAllotment(ctx, database.UpsertAgentHoursOrganizationAllotmentParams{
					OrganizationID: other.ID,
					AllotmentBps:   6000,
				}); err != nil {
					return err
				}
				close(holding)
				<-release
				return nil
			}, nil)
		}()
		testutil.TryReceive(ctx, t, holding)

		upsertErr := make(chan error, 1)
		go func() {
			_, err := client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(5000))
			upsertErr <- err
		}()
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			return advisoryLockWaiters(ctx, sqlDB, database.LockIDAgentHoursOrganizationAllotments) == 1
		}, testutil.IntervalFast, "API writer waits for the allotment lock")
		releaseLock()
		require.NoError(t, testutil.TryReceive(ctx, t, holderErr))

		requireAgentHoursStatus(t, testutil.TryReceive(ctx, t, upsertErr), http.StatusConflict)
	})
}

// advisoryLockWaiters counts sessions waiting for the given advisory lock
// in the current database.
func advisoryLockWaiters(ctx context.Context, sqlDB *sql.DB, lockID int64) int {
	var waiting int
	// #nosec G115: pg_locks reports the 64-bit key as two 32-bit halves.
	err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_locks
		WHERE locktype = 'advisory' AND NOT granted
		AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		AND classid::bigint = $1 AND objid::bigint = $2`,
		int64(uint32(uint64(lockID)>>32)), int64(uint32(lockID))).Scan(&waiting)
	if err != nil {
		return -1
	}
	return waiting
}

func TestAgentHoursGroupAllotments(t *testing.T) {
	t.Parallel()

	t.Run("OrgAdminScopedToOrganization", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: agentHoursLicense(1000),
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))
		member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		other := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

		first, err := orgAdmin.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "first"})
		require.NoError(t, err)
		second, err := orgAdmin.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "second"})
		require.NoError(t, err)
		//nolint:gocritic // Setup: the org admin cannot create groups in another organization.
		foreign, err := client.CreateGroup(ctx, other.ID, codersdk.CreateGroupRequest{Name: "foreign"})
		require.NoError(t, err)

		created, err := orgAdmin.UpsertAgentHoursGroupAllotment(ctx, first.ID, allotmentReq(7000))
		require.NoError(t, err)
		require.Equal(t, first.ID, created.GroupID)
		require.Equal(t, "first", created.GroupName)

		// Group shares are counted per organization, independent of the
		// organization tier and of other organizations.
		_, err = client.UpsertAgentHoursGroupAllotment(ctx, foreign.ID, allotmentReq(10000))
		require.NoError(t, err)
		_, err = client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(10000))
		require.NoError(t, err)

		sdkErr := requireAgentHoursStatus(t,
			func() error {
				_, err := orgAdmin.UpsertAgentHoursGroupAllotment(ctx, second.ID, allotmentReq(3001))
				return err
			}(), http.StatusConflict)
		require.Equal(t, []codersdk.ValidationError{{Field: "allotment_bps", Detail: "Must not exceed 3000."}}, sdkErr.Validations)
		_, err = orgAdmin.UpsertAgentHoursGroupAllotment(ctx, second.ID, allotmentReq(3000))
		require.NoError(t, err)

		list, err := orgAdmin.AgentHoursGroupAllotments(ctx, owner.OrganizationID)
		require.NoError(t, err)
		require.NotNil(t, list.OrganizationAllotmentBps)
		require.EqualValues(t, 10000, *list.OrganizationAllotmentBps)
		require.Len(t, list.Groups, 2)
		require.Equal(t, "first", list.Groups[0].GroupName)
		require.EqualValues(t, 7000, list.Groups[0].AllotmentBps)

		_, err = orgAdmin.UpsertAgentHoursGroupAllotment(ctx, foreign.ID, allotmentReq(100))
		requireAgentHoursStatus(t, err, http.StatusNotFound)
		err = orgAdmin.DeleteAgentHoursGroupAllotment(ctx, foreign.ID)
		requireAgentHoursStatus(t, err, http.StatusNotFound)
		_, err = orgAdmin.AgentHoursGroupAllotments(ctx, other.ID)
		requireAgentHoursStatus(t, err, http.StatusNotFound)

		_, err = member.UpsertAgentHoursGroupAllotment(ctx, first.ID, allotmentReq(100))
		requireAgentHoursStatus(t, err, http.StatusForbidden)
		err = member.DeleteAgentHoursGroupAllotment(ctx, first.ID)
		requireAgentHoursStatus(t, err, http.StatusForbidden)

		require.NoError(t, orgAdmin.DeleteAgentHoursGroupAllotment(ctx, first.ID))
		list, err = orgAdmin.AgentHoursGroupAllotments(ctx, owner.OrganizationID)
		require.NoError(t, err)
		require.Len(t, list.Groups, 1)
	})

	t.Run("EveryoneCannotBeAllotted", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: agentHoursLicense(1000),
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		// The Everyone group's ID is its organization's ID.
		//nolint:gocritic // The owner can update every group.
		_, err := client.UpsertAgentHoursGroupAllotment(ctx, owner.OrganizationID, allotmentReq(100))
		requireAgentHoursStatus(t, err, http.StatusBadRequest)
		//nolint:gocritic // The owner can read every group.
		list, err := client.AgentHoursGroupAllotments(ctx, owner.OrganizationID)
		require.NoError(t, err)
		require.Empty(t, list.Groups)
	})

	t.Run("DeletingGroupRemovesAllotment", func(t *testing.T) {
		t.Parallel()
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: agentHoursLicense(1000),
		})
		ctx := testutil.Context(t, testutil.WaitLong)

		//nolint:gocritic // Setup only.
		doomed, err := client.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "doomed"})
		require.NoError(t, err)
		next, err := client.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "next"})
		require.NoError(t, err)
		_, err = client.UpsertAgentHoursGroupAllotment(ctx, doomed.ID, allotmentReq(10000))
		require.NoError(t, err)

		require.NoError(t, client.DeleteGroup(ctx, doomed.ID))
		_, err = client.UpsertAgentHoursGroupAllotment(ctx, next.ID, allotmentReq(10000))
		require.NoError(t, err)
	})
}

func TestAgentHoursAllotmentsAudit(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		AuditLogging:   true,
		Options:        &coderdtest.Options{Auditor: auditor},
		LicenseOptions: agentHoursLicense(1000),
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:gocritic // Setup only.
	group, err := client.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{Name: "audited"})
	require.NoError(t, err)

	for _, tc := range []struct {
		resourceType database.ResourceType
		resourceID   uuid.UUID
		write        func(bps int32) error
		remove       func() error
	}{
		{
			resourceType: database.ResourceTypeAgentHoursOrganizationAllotment,
			resourceID:   owner.OrganizationID,
			write: func(bps int32) error {
				_, err := client.UpsertAgentHoursOrganizationAllotment(ctx, owner.OrganizationID, allotmentReq(bps))
				return err
			},
			remove: func() error { return client.DeleteAgentHoursOrganizationAllotment(ctx, owner.OrganizationID) },
		},
		{
			resourceType: database.ResourceTypeAgentHoursGroupAllotment,
			resourceID:   group.ID,
			write: func(bps int32) error {
				_, err := client.UpsertAgentHoursGroupAllotment(ctx, group.ID, allotmentReq(bps))
				return err
			},
			remove: func() error { return client.DeleteAgentHoursGroupAllotment(ctx, group.ID) },
		},
	} {
		auditor.ResetLogs()
		require.NoError(t, tc.write(2550))
		require.NoError(t, tc.write(3000))
		// Re-setting the same value changes nothing, so it is not audited.
		require.NoError(t, tc.write(3000))
		require.NoError(t, tc.remove())

		var actions []database.AuditAction
		for _, entry := range auditor.AuditLogs() {
			require.Equal(t, tc.resourceType, entry.ResourceType)
			require.Equal(t, tc.resourceID, entry.ResourceID)
			require.Equal(t, owner.OrganizationID, entry.OrganizationID)
			actions = append(actions, entry.Action)
		}
		require.Equal(t, []database.AuditAction{
			database.AuditActionCreate,
			database.AuditActionWrite,
			database.AuditActionDelete,
		}, actions, tc.resourceType)
	}
}
