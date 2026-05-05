package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateWorkspace_ExistingBuildQuotaFailureWithAuthz(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})
	modelCfg := seedModelConfig(t, db)
	orgResp := dbfake.Organization(t, db).
		EveryoneAllowance(40).
		Members(user).
		Do()
	org := orgResp.Org
	wsResp := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
	}).Seed(database.WorkspaceBuild{
		Transition: database.WorkspaceTransitionStart,
		DailyCost:  40,
	}).Starting().Do()
	ws := wsResp.Workspace

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		WorkspaceID:       uuid.NullUUID{UUID: ws.ID, Valid: true},
		BuildID:           uuid.NullUUID{UUID: wsResp.Build.ID, Valid: true},
		LastModelConfigID: modelCfg.ID,
		Title:             "test-existing-create-quota-authz",
	})

	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil),
		testAccessControlStorePointer(),
	)
	jobRead := make(chan struct{}, 1)
	wrappedDB := &jobInterceptStore{Store: authzDB, jobRead: jobRead}

	tool := chattool.CreateWorkspace(org.ID, wrappedDB, chattool.CreateWorkspaceOptions{
		OwnerID:     user.ID,
		ChatID:      chat.ID,
		WorkspaceMu: &sync.Mutex{},
		CreateFn: func(context.Context, uuid.UUID, codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
			t.Fatal("CreateFn should not be called when an existing build is in progress")
			return codersdk.Workspace{}, nil
		},
		Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})

	type toolResult struct {
		resp fantasy.ToolResponse
		err  error
	}
	done := make(chan toolResult, 1)
	go func() {
		resp, err := tool.Run(
			dbauthz.AsChatd(ctx),
			fantasy.ToolCall{
				ID:    "call-1",
				Name:  "create_workspace",
				Input: fmt.Sprintf(`{"template_id":%q}`, uuid.NewString()),
			},
		)
		done <- toolResult{resp, err}
	}()

	testutil.TryReceive(ctx, t, jobRead)

	now := time.Now().UTC()
	require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID:          wsResp.Build.JobID,
		UpdatedAt:   now,
		CompletedAt: sql.NullTime{Time: now, Valid: true},
		Error:       sql.NullString{String: "insufficient quota", Valid: true},
		ErrorCode: sql.NullString{
			String: string(codersdk.InsufficientQuota),
			Valid:  true,
		},
	}))

	res := testutil.TryReceive(ctx, t, done)
	require.NoError(t, res.err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.resp.Content), &result))
	require.Equal(t, string(codersdk.InsufficientQuota), result["error_code"])
	require.Equal(t, "Workspace quota reached", result["title"])
	require.Contains(t, result["error"], "existing workspace build failed")
	require.Contains(t, result["message"], "could not start this workspace")
	require.Equal(t, wsResp.Build.ID.String(), result["build_id"])
	quota, ok := result["quota"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(40), quota["credits_consumed"])
	require.Equal(t, float64(40), quota["budget"])
	require.False(t, res.resp.IsError)
}
