package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPGatewayEscalations(t *testing.T) {
	t.Parallel()

	t.Run("ListSponsorScopeAndStatus", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		now := time.Now().UTC()

		pending := seedMCPGatewayEscalation(ctx, t, db, owner.UserID, codersdk.MCPGatewayEscalationStatusPending, now.Add(time.Minute))
		approved := seedMCPGatewayEscalation(ctx, t, db, owner.UserID, codersdk.MCPGatewayEscalationStatusApproved, now.Add(time.Minute))

		all, err := client.MCPGatewayEscalations(ctx, "")
		require.NoError(t, err)
		require.Len(t, all, 2)
		require.Equal(t, approved.ID, all[0].ID)
		require.Equal(t, pending.ID, all[1].ID)
		require.Equal(t, pending.ServerSlug, all[1].ServerSlug)
		require.Equal(t, pending.Tool, all[1].Tool)
		require.Equal(t, string(pending.Input), all[1].Input)
		require.Equal(t, pending.WorkspaceName, all[1].WorkspaceName)

		filtered, err := client.MCPGatewayEscalations(ctx, codersdk.MCPGatewayEscalationStatusPending)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		require.Equal(t, pending.ID, filtered[0].ID)
		require.Equal(t, codersdk.MCPGatewayEscalationStatusPending, filtered[0].Status)

		other, err := otherClient.MCPGatewayEscalations(ctx, "")
		require.NoError(t, err)
		require.Empty(t, other)

		_, err = client.MCPGatewayEscalations(ctx, "garbage")
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("Approve", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		otherClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)
		escalation := seedMCPGatewayEscalation(ctx, t, db, owner.UserID, codersdk.MCPGatewayEscalationStatusPending, time.Now().UTC().Add(time.Minute))

		err := otherClient.ApproveMCPGatewayEscalation(ctx, escalation.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		err = client.ApproveMCPGatewayEscalation(ctx, escalation.ID)
		require.NoError(t, err)

		//nolint:gocritic // The test verifies the system-guarded storage result directly.
		stored, err := db.GetMCPGatewayEscalationByID(dbauthz.AsSystemRestricted(ctx), escalation.ID)
		require.NoError(t, err)
		require.Equal(t, string(codersdk.MCPGatewayEscalationStatusApproved), stored.Status)
		require.True(t, stored.ResolvedAt.Valid)
		require.True(t, stored.ResolvedBy.Valid)
		require.Equal(t, owner.UserID, stored.ResolvedBy.UUID)

		err = client.ApproveMCPGatewayEscalation(ctx, escalation.ID)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	})

	t.Run("Deny", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		escalation := seedMCPGatewayEscalation(ctx, t, db, owner.UserID, codersdk.MCPGatewayEscalationStatusPending, time.Now().UTC().Add(time.Minute))

		err := client.DenyMCPGatewayEscalation(ctx, escalation.ID)
		require.NoError(t, err)

		//nolint:gocritic // The test verifies the system-guarded storage result directly.
		stored, err := db.GetMCPGatewayEscalationByID(dbauthz.AsSystemRestricted(ctx), escalation.ID)
		require.NoError(t, err)
		require.Equal(t, string(codersdk.MCPGatewayEscalationStatusDenied), stored.Status)
		require.True(t, stored.ResolvedAt.Valid)
		require.True(t, stored.ResolvedBy.Valid)
		require.Equal(t, owner.UserID, stored.ResolvedBy.UUID)
	})

	t.Run("LazyExpiry", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		escalation := seedMCPGatewayEscalation(ctx, t, db, owner.UserID, codersdk.MCPGatewayEscalationStatusPending, time.Now().UTC().Add(-time.Minute))

		escalations, err := client.MCPGatewayEscalations(ctx, "")
		require.NoError(t, err)
		require.Len(t, escalations, 1)
		require.Equal(t, escalation.ID, escalations[0].ID)
		require.Equal(t, codersdk.MCPGatewayEscalationStatusExpired, escalations[0].Status)

		//nolint:gocritic // The test verifies the system-guarded storage result directly.
		stored, err := db.GetMCPGatewayEscalationByID(dbauthz.AsSystemRestricted(ctx), escalation.ID)
		require.NoError(t, err)
		require.Equal(t, string(codersdk.MCPGatewayEscalationStatusExpired), stored.Status)
		require.True(t, stored.ResolvedAt.Valid)
	})
}

func seedMCPGatewayEscalation(
	ctx context.Context,
	t testing.TB,
	db database.Store,
	sponsorID uuid.UUID,
	status codersdk.MCPGatewayEscalationStatus,
	expiresAt time.Time,
) database.MCPGatewayEscalation {
	t.Helper()

	createdAt := time.Now().UTC()
	resolvedAt := sql.NullTime{}
	resolvedBy := uuid.NullUUID{}
	if status != codersdk.MCPGatewayEscalationStatusPending {
		resolvedAt = sql.NullTime{Time: createdAt, Valid: true}
		resolvedBy = uuid.NullUUID{UUID: sponsorID, Valid: true}
	}

	//nolint:gocritic // Tests seed sponsor-scoped escalations through the system-guarded store method.
	escalation, err := db.InsertMCPGatewayEscalation(dbauthz.AsSystemRestricted(ctx), database.InsertMCPGatewayEscalationParams{
		ID:                uuid.New(),
		MCPServerConfigID: uuid.New(),
		ServerSlug:        "filesystem",
		ServerUrl:         "https://mcp.example.com",
		Tool:              "read_file",
		Input:             json.RawMessage(`{"path":"README.md"}`),
		AIAgentID:         uuid.New(),
		SponsorUserID:     sponsorID,
		WorkspaceName:     "example-workspace",
		Status:            string(status),
		CreatedAt:         createdAt,
		ExpiresAt:         expiresAt,
		ResolvedAt:        resolvedAt,
		ResolvedBy:        resolvedBy,
	})
	require.NoError(t, err)
	return escalation
}
