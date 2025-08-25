package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// makeAITask creates an AI-task workspace.
func makeAITask(t *testing.T, db database.Store, orgID, adminID, ownerID uuid.UUID, transition database.WorkspaceTransition, prompt string) (workspace database.WorkspaceTable) {
	t.Helper()

	tv := dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			OrganizationID: orgID,
			CreatedBy:      adminID,
			HasAITask: sql.NullBool{
				Bool:  true,
				Valid: true,
			},
		}).Do()

	ws := database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        ownerID,
		TemplateID:     tv.Template.ID,
	}
	build := dbfake.WorkspaceBuild(t, db, ws).
		Seed(database.WorkspaceBuild{
			TemplateVersionID: tv.TemplateVersion.ID,
			Transition:        transition,
		}).WithAgent().Do()
	dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{
		{
			WorkspaceBuildID: build.Build.ID,
			Name:             codersdk.AITaskPromptParameterName,
			Value:            prompt,
		},
	})
	agents, err := db.GetWorkspaceAgentsByWorkspaceAndBuildNumber(
		dbauthz.AsSystemRestricted(context.Background()),
		database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{
			WorkspaceID: build.Workspace.ID,
			BuildNumber: build.Build.BuildNumber,
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, agents)
	agentID := agents[0].ID

	// Create a workspace app and set it as the sidebar app.
	app := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{
		AgentID:     agentID,
		Slug:        "task-sidebar",
		DisplayName: "Task Sidebar",
		External:    false,
	})

	// Update build flags to reference the sidebar app and HasAITask=true.
	err = db.UpdateWorkspaceBuildFlagsByID(
		dbauthz.AsSystemRestricted(context.Background()),
		database.UpdateWorkspaceBuildFlagsByIDParams{
			ID:               build.Build.ID,
			HasAITask:        sql.NullBool{Bool: true, Valid: true},
			HasExternalAgent: sql.NullBool{Bool: false, Valid: false},
			SidebarAppID:     uuid.NullUUID{UUID: app.ID, Valid: true},
			UpdatedAt:        build.Build.UpdatedAt,
		},
	)
	require.NoError(t, err)

	return build.Workspace
}

func TestExpTaskList(t *testing.T) {
	t.Parallel()

	t.Run("NoTasks_Table", func(t *testing.T) {
		t.Parallel()

		// Quiet logger to reduce noise.
		quiet := slog.Make(sloghuman.Sink(io.Discard))
		client, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{Logger: &quiet})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		inv, root := clitest.New(t, "exp", "task", "list")
		clitest.SetupConfig(t, memberClient, root)

		pty := ptytest.New(t).Attach(inv)
		ctx := testutil.Context(t, testutil.WaitShort)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		pty.ExpectMatch("No tasks found.")
	})

	t.Run("Single_Table", func(t *testing.T) {
		t.Parallel()

		// Quiet logger to reduce noise.
		quiet := slog.Make(sloghuman.Sink(io.Discard))
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{Logger: &quiet})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		wantPrompt := "build me a web app"
		ws := makeAITask(t, db, owner.OrganizationID, owner.UserID, memberUser.ID, database.WorkspaceTransitionStart, wantPrompt)

		inv, root := clitest.New(t, "exp", "task", "list", "--column", "id,name,status,initial prompt")
		clitest.SetupConfig(t, memberClient, root)

		pty := ptytest.New(t).Attach(inv)
		ctx := testutil.Context(t, testutil.WaitShort)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Validate the table includes the task and status.
		pty.ExpectMatch(ws.Name)
		pty.ExpectMatch("running")
		pty.ExpectMatch(wantPrompt)
	})

	t.Run("StatusFilter_JSON", func(t *testing.T) {
		t.Parallel()

		// Quiet logger to reduce noise.
		quiet := slog.Make(sloghuman.Sink(io.Discard))
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{Logger: &quiet})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Create two AI tasks: one running, one stopped.
		running := makeAITask(t, db, owner.OrganizationID, owner.UserID, memberUser.ID, database.WorkspaceTransitionStart, "keep me running")
		stopped := makeAITask(t, db, owner.OrganizationID, owner.UserID, memberUser.ID, database.WorkspaceTransitionStop, "stop me please")

		// Use JSON output to reliably validate filtering.
		inv, root := clitest.New(t, "exp", "task", "list", "--status=stopped", "--output=json")
		clitest.SetupConfig(t, memberClient, root)

		ctx := testutil.Context(t, testutil.WaitShort)
		var stdout bytes.Buffer
		inv.Stdout = &stdout
		inv.Stderr = &stdout

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var tasks []codersdk.Task
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &tasks))

		// Only the stopped task is returned.
		require.Len(t, tasks, 1, "expected one task after filtering")
		require.Equal(t, stopped.ID, tasks[0].ID)
		require.NotEqual(t, running.ID, tasks[0].ID)
	})

	t.Run("UserFlag_Me_Table", func(t *testing.T) {
		t.Parallel()

		quiet := slog.Make(sloghuman.Sink(io.Discard))
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{Logger: &quiet})
		owner := coderdtest.CreateFirstUser(t, client)
		_, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		_ = makeAITask(t, db, owner.OrganizationID, owner.UserID, memberUser.ID, database.WorkspaceTransitionStart, "other-task")
		ws := makeAITask(t, db, owner.OrganizationID, owner.UserID, owner.UserID, database.WorkspaceTransitionStart, "me-task")

		inv, root := clitest.New(t, "exp", "task", "list", "--user", "me")
		//nolint:gocritic // Owner client is intended here smoke test the member task not showing up.
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t).Attach(inv)
		ctx := testutil.Context(t, testutil.WaitShort)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		pty.ExpectMatch(ws.Name)
	})
}

func TestExpTaskList_OwnerCanListOthers(t *testing.T) {
	t.Parallel()

	// Quiet logger to reduce noise.
	quiet := slog.Make(sloghuman.Sink(io.Discard))
	ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{Logger: &quiet})
	owner := coderdtest.CreateFirstUser(t, ownerClient)

	// Create two additional members in the owner's organization.
	_, memberAUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	_, memberBUser := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	// Seed an AI task for member A and B.
	_ = makeAITask(t, db, owner.OrganizationID, owner.UserID, memberAUser.ID, database.WorkspaceTransitionStart, "member-A-task")
	_ = makeAITask(t, db, owner.OrganizationID, owner.UserID, memberBUser.ID, database.WorkspaceTransitionStart, "member-B-task")

	t.Run("OwnerListsSpecificUserWithUserFlag_JSON", func(t *testing.T) {
		t.Parallel()

		// As the owner, list only member A tasks.
		inv, root := clitest.New(t, "exp", "task", "list", "--user", memberAUser.Username, "--output=json")
		//nolint:gocritic // Owner client is intended here to allow member tasks to be listed.
		clitest.SetupConfig(t, ownerClient, root)

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		ctx := testutil.Context(t, testutil.WaitShort)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var tasks []codersdk.Task
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &tasks))

		// At least one task to belong to member A.
		require.NotEmpty(t, tasks, "expected at least one task for member A")
		// All tasks should belong to member A.
		for _, task := range tasks {
			require.Equal(t, memberAUser.ID, task.OwnerID, "expected only member A tasks")
		}
	})

	t.Run("OwnerListsAllWithAllFlag_JSON", func(t *testing.T) {
		t.Parallel()

		// As the owner, list all tasks to verify both member tasks are present.
		// Use JSON output to reliably validate filtering.
		inv, root := clitest.New(t, "exp", "task", "list", "--all", "--output=json")
		//nolint:gocritic // Owner client is intended here to allow all tasks to be listed.
		clitest.SetupConfig(t, ownerClient, root)

		var stdout bytes.Buffer
		inv.Stdout = &stdout

		ctx := testutil.Context(t, testutil.WaitShort)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		var tasks []codersdk.Task
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &tasks))

		// Expect at least two tasks and ensure both owners (member A and member B) are represented.
		require.GreaterOrEqual(t, len(tasks), 2, "expected two or more tasks in --all listing")

		// Use slice.Find for concise existence checks.
		_, foundA := slice.Find(tasks, func(t codersdk.Task) bool { return t.OwnerID == memberAUser.ID })
		_, foundB := slice.Find(tasks, func(t codersdk.Task) bool { return t.OwnerID == memberBUser.ID })

		require.True(t, foundA, "expected at least one task for member A in --all listing")
		require.True(t, foundB, "expected at least one task for member B in --all listing")
	})
}
