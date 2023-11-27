package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/tz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

// setupTestSchedule creates 4 workspaces:
// 1. a-owner-ws1: owned by owner, has both autostart and autostop enabled.
// 2. b-owner-ws2: owned by owner, has only autostart enabled.
// 3. c-member-ws3: owned by member, has only autostop enabled.
// 4. d-member-ws4: owned by member, has neither autostart nor autostop enabled.
// It returns the owner and member clients, the database, and the workspaces.
// The workspaces are returned in the same order as they are created.
func setupTestSchedule(t *testing.T, sched *cron.Schedule) (ownerClient, memberClient *codersdk.Client, db database.Store, ws []codersdk.Workspace) {
	t.Helper()

	ownerClient, db = coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	memberClient, memberUser := coderdtest.CreateAnotherUserMutators(t, ownerClient, owner.OrganizationID, nil, func(r *codersdk.CreateUserRequest) {
		r.Username = "testuser2" // ensure deterministic ordering
	})
	_ = dbfake.Workspace(t, db).Seed(database.Workspace{
		Name:              "a-owner",
		OwnerID:           owner.UserID,
		OrganizationID:    owner.OrganizationID,
		AutostartSchedule: sql.NullString{String: sched.String(), Valid: true},
		Ttl:               sql.NullInt64{Int64: 8 * time.Hour.Nanoseconds(), Valid: true},
	}).WithAgent().Do()
	_ = dbfake.Workspace(t, db).Seed(database.Workspace{
		Name:              "b-owner",
		OwnerID:           owner.UserID,
		OrganizationID:    owner.OrganizationID,
		AutostartSchedule: sql.NullString{String: sched.String(), Valid: true},
	}).WithAgent().Do()
	_ = dbfake.Workspace(t, db).Seed(database.Workspace{
		Name:           "c-member",
		OwnerID:        memberUser.ID,
		OrganizationID: owner.OrganizationID,
		Ttl:            sql.NullInt64{Int64: 8 * time.Hour.Nanoseconds(), Valid: true},
	}).WithAgent().Do()
	_ = dbfake.Workspace(t, db).Seed(database.Workspace{
		Name:           "d-member",
		OwnerID:        memberUser.ID,
		OrganizationID: owner.OrganizationID,
	}).WithAgent().Do()

	// Need this for LatestBuild.Deadline
	resp, err := ownerClient.Workspaces(context.Background(), codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	require.Len(t, resp.Workspaces, 4)
	// Ensure same order as in CLI output
	ws = resp.Workspaces
	sort.Slice(ws, func(i, j int) bool {
		a := ws[i].OwnerName + "/" + ws[i].Name
		b := ws[j].OwnerName + "/" + ws[j].Name
		return a < b
	})

	return ownerClient, memberClient, db, ws
}

//nolint:paralleltest // t.Setenv
func TestScheduleShow(t *testing.T) {
	// Given
	// Set timezone to Asia/Kolkata to surface any timezone-related bugs.
	t.Setenv("TZ", "Asia/Kolkata")
	loc, err := tz.TimezoneIANA()
	require.NoError(t, err)
	require.Equal(t, "Asia/Kolkata", loc.String())
	sched, err := cron.Weekly("CRON_TZ=Europe/Dublin 30 7 * * Mon-Fri")
	require.NoError(t, err, "invalid schedule")
	ownerClient, memberClient, _, ws := setupTestSchedule(t, sched)
	now := time.Now()

	t.Run("OwnerNoArgs", func(t *testing.T) {
		// When: owner specifies no args
		inv, root := clitest.New(t, "schedule", "show")
		//nolint:gocritic // Testing that owner user sees all
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: they should see their own workspaces.
		// 1st workspace: a-owner-ws1 has both autostart and autostop enabled.
		pty.ExpectMatch(ws[0].OwnerName + "/" + ws[0].Name)
		pty.ExpectMatch(sched.Humanize())
		pty.ExpectMatch(sched.Next(now).In(loc).Format(time.RFC3339))
		pty.ExpectMatch("8h")
		pty.ExpectMatch(ws[0].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339))
		// 2nd workspace: b-owner-ws2 has only autostart enabled.
		pty.ExpectMatch(ws[1].OwnerName + "/" + ws[1].Name)
		pty.ExpectMatch(sched.Humanize())
		pty.ExpectMatch(sched.Next(now).In(loc).Format(time.RFC3339))
	})

	t.Run("OwnerAll", func(t *testing.T) {
		// When: owner lists all workspaces
		inv, root := clitest.New(t, "schedule", "show", "--all")
		//nolint:gocritic // Testing that owner user sees all
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: they should see all workspaces
		// 1st workspace: a-owner-ws1 has both autostart and autostop enabled.
		pty.ExpectMatch(ws[0].OwnerName + "/" + ws[0].Name)
		pty.ExpectMatch(sched.Humanize())
		pty.ExpectMatch(sched.Next(now).In(loc).Format(time.RFC3339))
		pty.ExpectMatch("8h")
		pty.ExpectMatch(ws[0].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339))
		// 2nd workspace: b-owner-ws2 has only autostart enabled.
		pty.ExpectMatch(ws[1].OwnerName + "/" + ws[1].Name)
		pty.ExpectMatch(sched.Humanize())
		pty.ExpectMatch(sched.Next(now).In(loc).Format(time.RFC3339))
		// 3rd workspace: c-member-ws3 has only autostop enabled.
		pty.ExpectMatch(ws[2].OwnerName + "/" + ws[2].Name)
		pty.ExpectMatch("8h")
		pty.ExpectMatch(ws[2].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339))
		// 4th workspace: d-member-ws4 has neither autostart nor autostop enabled.
		pty.ExpectMatch(ws[3].OwnerName + "/" + ws[3].Name)
	})

	t.Run("OwnerSearchByName", func(t *testing.T) {
		// When: owner specifies a search query
		inv, root := clitest.New(t, "schedule", "show", "--search", "name:"+ws[1].Name)
		//nolint:gocritic // Testing that owner user sees all
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: they should see workspaces matching that query
		// 2nd workspace: b-owner-ws2 has only autostart enabled.
		pty.ExpectMatch(ws[1].OwnerName + "/" + ws[1].Name)
		pty.ExpectMatch(sched.Humanize())
		pty.ExpectMatch(sched.Next(now).In(loc).Format(time.RFC3339))
	})

	t.Run("OwnerOneArg", func(t *testing.T) {
		// When: owner asks for a specific workspace by name
		inv, root := clitest.New(t, "schedule", "show", ws[2].OwnerName+"/"+ws[2].Name)
		//nolint:gocritic // Testing that owner user sees all
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: they should see that workspace
		// 3rd workspace: c-member-ws3 has only autostop enabled.
		pty.ExpectMatch(ws[2].OwnerName + "/" + ws[2].Name)
		pty.ExpectMatch("8h")
		pty.ExpectMatch(ws[2].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339))
	})

	t.Run("MemberNoArgs", func(t *testing.T) {
		// When: a member specifies no args
		inv, root := clitest.New(t, "schedule", "show")
		clitest.SetupConfig(t, memberClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: they should see their own workspaces
		// 1st workspace: c-member-ws3 has only autostop enabled.
		pty.ExpectMatch(ws[2].OwnerName + "/" + ws[2].Name)
		pty.ExpectMatch("8h")
		pty.ExpectMatch(ws[2].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339))
		// 2nd workspace: d-member-ws4 has neither autostart nor autostop enabled.
		pty.ExpectMatch(ws[3].OwnerName + "/" + ws[3].Name)
	})

	t.Run("MemberAll", func(t *testing.T) {
		// When: a member lists all workspaces
		inv, root := clitest.New(t, "schedule", "show", "--all")
		clitest.SetupConfig(t, memberClient, root)
		pty := ptytest.New(t).Attach(inv)
		ctx := testutil.Context(t, testutil.WaitShort)
		errC := make(chan error)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()
		require.NoError(t, <-errC)

		// Then: they should only see their own
		// 1st workspace: c-member-ws3 has only autostop enabled.
		pty.ExpectMatch(ws[2].OwnerName + "/" + ws[2].Name)
		pty.ExpectMatch("8h")
		pty.ExpectMatch(ws[2].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339))
		// 2nd workspace: d-member-ws4 has neither autostart nor autostop enabled.
		pty.ExpectMatch(ws[3].OwnerName + "/" + ws[3].Name)
	})

	t.Run("JSON", func(t *testing.T) {
		// When: owner lists all workspaces in JSON format
		inv, root := clitest.New(t, "schedule", "show", "--all", "--output", "json")
		var buf bytes.Buffer
		inv.Stdout = &buf
		clitest.SetupConfig(t, ownerClient, root)
		ctx := testutil.Context(t, testutil.WaitShort)
		errC := make(chan error)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()
		assert.NoError(t, <-errC)

		// Then: they should see all workspace schedules in JSON format
		var parsed []map[string]string
		require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
		require.Len(t, parsed, 4)
		// Ensure same order as in CLI output
		sort.Slice(parsed, func(i, j int) bool {
			a := parsed[i]["workspace"]
			b := parsed[j]["workspace"]
			return a < b
		})
		// 1st workspace: a-owner-ws1 has both autostart and autostop enabled.
		assert.Equal(t, ws[0].OwnerName+"/"+ws[0].Name, parsed[0]["workspace"])
		assert.Equal(t, sched.Humanize(), parsed[0]["starts_at"])
		assert.Equal(t, sched.Next(now).In(loc).Format(time.RFC3339), parsed[0]["starts_next"])
		assert.Equal(t, "8h", parsed[0]["stops_after"])
		assert.Equal(t, ws[0].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339), parsed[0]["stops_next"])
		// 2nd workspace: b-owner-ws2 has only autostart enabled.
		assert.Equal(t, ws[1].OwnerName+"/"+ws[1].Name, parsed[1]["workspace"])
		assert.Equal(t, sched.Humanize(), parsed[1]["starts_at"])
		assert.Equal(t, sched.Next(now).In(loc).Format(time.RFC3339), parsed[1]["starts_next"])
		assert.Empty(t, parsed[1]["stops_after"])
		assert.Empty(t, parsed[1]["stops_next"])
		// 3rd workspace: c-member-ws3 has only autostop enabled.
		assert.Equal(t, ws[2].OwnerName+"/"+ws[2].Name, parsed[2]["workspace"])
		assert.Empty(t, parsed[2]["starts_at"])
		assert.Empty(t, parsed[2]["starts_next"])
		assert.Equal(t, "8h", parsed[2]["stops_after"])
		assert.Equal(t, ws[2].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339), parsed[2]["stops_next"])
		// 4th workspace: d-member-ws4 has neither autostart nor autostop enabled.
		assert.Equal(t, ws[3].OwnerName+"/"+ws[3].Name, parsed[3]["workspace"])
		assert.Empty(t, parsed[3]["starts_at"])
		assert.Empty(t, parsed[3]["starts_next"])
		assert.Empty(t, parsed[3]["stops_after"])
	})
}

//nolint:paralleltest // t.Setenv
func TestScheduleModify(t *testing.T) {
	// Given
	// Set timezone to Asia/Kolkata to surface any timezone-related bugs.
	t.Setenv("TZ", "Asia/Kolkata")
	loc, err := tz.TimezoneIANA()
	require.NoError(t, err)
	require.Equal(t, "Asia/Kolkata", loc.String())
	sched, err := cron.Weekly("CRON_TZ=Europe/Dublin 30 7 * * Mon-Fri")
	require.NoError(t, err, "invalid schedule")
	ownerClient, _, _, ws := setupTestSchedule(t, sched)
	now := time.Now()

	t.Run("SetStart", func(t *testing.T) {
		// When: we set the start schedule
		inv, root := clitest.New(t,
			"schedule", "start", ws[3].OwnerName+"/"+ws[3].Name, "7:30AM", "Mon-Fri", "Europe/Dublin",
		)
		//nolint:gocritic // this workspace is not owned by the same user
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: the updated schedule should be shown
		pty.ExpectMatch(ws[3].OwnerName + "/" + ws[3].Name)
		pty.ExpectMatch(sched.Humanize())
		pty.ExpectMatch(sched.Next(now).In(loc).Format(time.RFC3339))
	})

	t.Run("SetStop", func(t *testing.T) {
		// When: we set the stop schedule
		inv, root := clitest.New(t,
			"schedule", "stop", ws[2].OwnerName+"/"+ws[2].Name, "8h30m",
		)
		//nolint:gocritic // this workspace is not owned by the same user
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: the updated schedule should be shown
		pty.ExpectMatch(ws[2].OwnerName + "/" + ws[2].Name)
		pty.ExpectMatch("8h30m")
		pty.ExpectMatch(ws[2].LatestBuild.Deadline.Time.In(loc).Format(time.RFC3339))
	})

	t.Run("UnsetStart", func(t *testing.T) {
		// When: we unset the start schedule
		inv, root := clitest.New(t,
			"schedule", "start", ws[1].OwnerName+"/"+ws[1].Name, "manual",
		)
		//nolint:gocritic // this workspace is owned by owner
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: the updated schedule should be shown
		pty.ExpectMatch(ws[1].OwnerName + "/" + ws[1].Name)
	})

	t.Run("UnsetStop", func(t *testing.T) {
		// When: we unset the stop schedule
		inv, root := clitest.New(t,
			"schedule", "stop", ws[0].OwnerName+"/"+ws[0].Name, "manual",
		)
		//nolint:gocritic // this workspace is owned by owner
		clitest.SetupConfig(t, ownerClient, root)
		pty := ptytest.New(t).Attach(inv)
		require.NoError(t, inv.Run())

		// Then: the updated schedule should be shown
		pty.ExpectMatch(ws[0].OwnerName + "/" + ws[0].Name)
	})
}

//nolint:paralleltest // t.Setenv
func TestScheduleOverride(t *testing.T) {
	// Given
	// Set timezone to Asia/Kolkata to surface any timezone-related bugs.
	t.Setenv("TZ", "Asia/Kolkata")
	loc, err := tz.TimezoneIANA()
	require.NoError(t, err)
	require.Equal(t, "Asia/Kolkata", loc.String())
	sched, err := cron.Weekly("CRON_TZ=Europe/Dublin 30 7 * * Mon-Fri")
	require.NoError(t, err, "invalid schedule")
	ownerClient, _, _, ws := setupTestSchedule(t, sched)
	now := time.Now()
	// To avoid the likelihood of time-related flakes, only matching up to the hour.
	expectedDeadline := time.Now().In(loc).Add(10 * time.Hour).Format("2006-01-02T15:")

	// When: we override the stop schedule
	inv, root := clitest.New(t,
		"schedule", "override-stop", ws[0].OwnerName+"/"+ws[0].Name, "10h",
	)

	clitest.SetupConfig(t, ownerClient, root)
	pty := ptytest.New(t).Attach(inv)
	require.NoError(t, inv.Run())

	// Then: the updated schedule should be shown
	pty.ExpectMatch(ws[0].OwnerName + "/" + ws[0].Name)
	pty.ExpectMatch(sched.Humanize())
	pty.ExpectMatch(sched.Next(now).In(loc).Format(time.RFC3339))
	pty.ExpectMatch("8h")
	pty.ExpectMatch(expectedDeadline)
}
