package agentapi_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/boundaryusage"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

const testMaxStalenessMs = 60000

func TestBoundaryLogsAPI_ReportBoundaryLogs_MultipleBatches(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	workspaceID := uuid.New()
	ownerID := uuid.New()
	replicaID := uuid.New()

	api := &agentapi.BoundaryLogsAPI{
		Log:                  slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		WorkspaceID:          workspaceID,
		OwnerID:              ownerID,
		TemplateID:           uuid.New(),
		TemplateVersionID:    uuid.New(),
		BoundaryUsageTracker: tracker,
	}

	// First batch: 3 allowed, 1 denied.
	req1 := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{Allowed: true, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://a.com"}}},
			{Allowed: true, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://b.com"}}},
			{Allowed: true, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://c.com"}}},
			{Allowed: false, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://blocked.com"}}},
		},
	}

	_, err := api.ReportBoundaryLogs(ctx, req1)
	require.NoError(t, err)

	// Second batch: 1 allowed, 2 denied.
	req2 := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{Allowed: true, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://a.com"}}},
			{Allowed: false, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://blocked1.com"}}},
			{Allowed: false, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://blocked2.com"}}},
		},
	}

	_, err = api.ReportBoundaryLogs(ctx, req2)
	require.NoError(t, err)

	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, testMaxStalenessMs)
	require.NoError(t, err)

	require.Equal(t, int64(1), summary.UniqueWorkspaces)
	require.Equal(t, int64(1), summary.UniqueUsers)
	require.Equal(t, int64(3+1), summary.AllowedRequests)
	require.Equal(t, int64(1+2), summary.DeniedRequests)
}

func TestBoundaryLogsAPI_ReportBoundaryLogs_EmptyRequest(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	api := &agentapi.BoundaryLogsAPI{
		Log:                  slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		WorkspaceID:          uuid.New(),
		OwnerID:              uuid.New(),
		TemplateID:           uuid.New(),
		TemplateVersionID:    uuid.New(),
		BoundaryUsageTracker: tracker,
	}

	// Send an empty request with no logs, then flush and verify no usage was
	// tracked.
	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{},
	}

	_, err := api.ReportBoundaryLogs(ctx, req)
	require.NoError(t, err)

	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, testMaxStalenessMs)
	require.NoError(t, err)

	require.Equal(t, int64(0), summary.UniqueWorkspaces)
	require.Equal(t, int64(0), summary.UniqueUsers)
	require.Equal(t, int64(0), summary.AllowedRequests)
	require.Equal(t, int64(0), summary.DeniedRequests)
}

func TestBoundaryLogsAPI_ReportBoundaryLogs_InvalidLogs(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	api := &agentapi.BoundaryLogsAPI{
		Log:                  slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		WorkspaceID:          uuid.New(),
		OwnerID:              uuid.New(),
		TemplateID:           uuid.New(),
		TemplateVersionID:    uuid.New(),
		BoundaryUsageTracker: tracker,
	}

	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: nil, // Invalid: nil HTTP request
				},
			},
			{
				Allowed: true,
				Time:    timestamppb.Now(),
				Resource: &agentproto.BoundaryLog_HttpRequest_{
					HttpRequest: &agentproto.BoundaryLog_HttpRequest{
						Method: "GET",
						Url:    "https://valid.com",
					},
				},
			},
		},
	}

	_, err := api.ReportBoundaryLogs(ctx, req)
	require.NoError(t, err)

	// Flush and verify only the valid log was tracked.
	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, testMaxStalenessMs)
	require.NoError(t, err)

	require.Equal(t, int64(1), summary.UniqueWorkspaces)
	require.Equal(t, int64(1), summary.UniqueUsers)
	require.Equal(t, int64(1), summary.AllowedRequests)
	require.Equal(t, int64(0), summary.DeniedRequests)
}

func TestBoundaryLogsAPI_ReportBoundaryLogs_MultipleWorkspaces(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	tracker := boundaryusage.NewTracker()
	replicaID := uuid.New()

	// Simulate multiple workspaces reporting through different API instances.
	workspace1, workspace2 := uuid.New(), uuid.New()
	owner1, owner2 := uuid.New(), uuid.New()

	api1 := &agentapi.BoundaryLogsAPI{
		Log:                  slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		WorkspaceID:          workspace1,
		OwnerID:              owner1,
		TemplateID:           uuid.New(),
		TemplateVersionID:    uuid.New(),
		BoundaryUsageTracker: tracker,
	}

	api2 := &agentapi.BoundaryLogsAPI{
		Log:                  slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		WorkspaceID:          workspace2,
		OwnerID:              owner2,
		TemplateID:           uuid.New(),
		TemplateVersionID:    uuid.New(),
		BoundaryUsageTracker: tracker,
	}

	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: []*agentproto.BoundaryLog{
			{Allowed: true, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://example.com"}}},
			{Allowed: false, Time: timestamppb.Now(), Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: &agentproto.BoundaryLog_HttpRequest{Method: "GET", Url: "https://blocked.com"}}},
		},
	}

	_, err := api1.ReportBoundaryLogs(ctx, req)
	require.NoError(t, err)

	_, err = api2.ReportBoundaryLogs(ctx, req)
	require.NoError(t, err)

	// Flush and verify both workspaces are tracked.
	err = tracker.FlushToDB(ctx, db, replicaID)
	require.NoError(t, err)

	boundaryCtx := dbauthz.AsBoundaryUsageTracker(ctx)
	summary, err := db.GetBoundaryUsageSummary(boundaryCtx, testMaxStalenessMs)
	require.NoError(t, err)

	require.Equal(t, int64(2), summary.UniqueWorkspaces)
	require.Equal(t, int64(2), summary.UniqueUsers)
	require.Equal(t, int64(1+1), summary.AllowedRequests)
	require.Equal(t, int64(1+1), summary.DeniedRequests)
}
