package workspacestats

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/cryptorand"
)

func TestBatchStats(t *testing.T) {
	t.Parallel()

	// Given: a fresh batcher with no data
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store, ps := dbtestutil.NewDB(t)

	// Set up some test dependencies.
	deps1 := setupDeps(t, store, ps)
	deps2 := setupDeps(t, store, ps)
	tick := make(chan time.Time)
	flushed := make(chan int, 1)

	b, closer, err := NewBatcher(ctx,
		BatcherWithStore(store),
		BatcherWithLogger(log),
		func(b *DBBatcher) {
			b.tickCh = tick
			b.flushed = flushed
		},
	)
	require.NoError(t, err)
	t.Cleanup(closer)

	// Given: no data points are added for workspace
	// When: it becomes time to report stats
	t1 := dbtime.Now()
	// Signal a tick and wait for a flush to complete.
	tick <- t1
	f := <-flushed
	require.Equal(t, 0, f, "expected no data to be flushed")
	t.Log("flush 1 completed")

	// Then: it should report no stats.
	stats, err := store.GetWorkspaceAgentStats(ctx, t1)
	require.NoError(t, err, "should not error getting stats")
	require.Empty(t, stats, "should have no stats for workspace")

	// Given: a single data point is added for workspace
	t2 := t1.Add(time.Second)
	t.Log("inserting 1 stat")
	b.Add(t2.Add(time.Millisecond), deps1.Agent.ID, deps1.User.ID, deps1.Template.ID, deps1.Workspace.ID, randStats(t), false)

	// When: it becomes time to report stats
	// Signal a tick and wait for a flush to complete.
	tick <- t2
	f = <-flushed // Wait for a flush to complete.
	require.Equal(t, 1, f, "expected one stat to be flushed")
	t.Log("flush 2 completed")

	// Then: it should report a single stat.
	stats, err = store.GetWorkspaceAgentStats(ctx, t2)
	require.NoError(t, err, "should not error getting stats")
	require.Len(t, stats, 1, "should have stats for workspace")

	// Given: a lot of data points are added for both workspaces
	// (equal to batch size)
	t3 := t2.Add(time.Second)
	done := make(chan struct{})

	go func() {
		defer close(done)
		t.Logf("inserting %d stats", defaultBufferSize)
		for i := 0; i < defaultBufferSize; i++ {
			if i%2 == 0 {
				b.Add(t3.Add(time.Millisecond), deps1.Agent.ID, deps1.User.ID, deps1.Template.ID, deps1.Workspace.ID, randStats(t), false)
			} else {
				b.Add(t3.Add(time.Millisecond), deps2.Agent.ID, deps2.User.ID, deps2.Template.ID, deps2.Workspace.ID, randStats(t), false)
			}
		}
	}()

	// When: the buffer comes close to capacity
	// Then: The buffer will force-flush once.
	f = <-flushed
	t.Log("flush 3 completed")
	require.Greater(t, f, 819, "expected at least 819 stats to be flushed (>=80% of buffer)")
	// And we should finish inserting the stats
	<-done

	stats, err = store.GetWorkspaceAgentStats(ctx, t3)
	require.NoError(t, err, "should not error getting stats")
	require.Len(t, stats, 2, "should have stats for both workspaces")

	// Ensures that a subsequent flush pushes all the remaining data
	t4 := t3.Add(time.Second)
	tick <- t4
	f2 := <-flushed
	t.Log("flush 4 completed")
	expectedCount := defaultBufferSize - f
	require.Equal(t, expectedCount, f2, "did not flush expected remaining rows")

	// Ensure that a subsequent flush does not push stale data.
	t5 := t4.Add(time.Second)
	tick <- t5
	f = <-flushed
	require.Zero(t, f, "expected zero stats to have been flushed")
	t.Log("flush 5 completed")

	stats, err = store.GetWorkspaceAgentStats(ctx, t5)
	require.NoError(t, err, "should not error getting stats")
	require.Len(t, stats, 0, "should have no stats for workspace")

	// Ensure that buf never grew beyond what we expect
	require.Equal(t, defaultBufferSize, cap(b.buf.ID), "buffer grew beyond expected capacity")
}

// randStats returns a random agentproto.Stats
func randStats(t *testing.T, opts ...func(*agentproto.Stats)) *agentproto.Stats {
	t.Helper()
	s := &agentproto.Stats{
		ConnectionsByProto: map[string]int64{
			"ssh":              mustRandInt64n(t, 9) + 1,
			"vscode":           mustRandInt64n(t, 9) + 1,
			"jetbrains":        mustRandInt64n(t, 9) + 1,
			"reconnecting_pty": mustRandInt64n(t, 9) + 1,
		},
		ConnectionCount:             mustRandInt64n(t, 99) + 1,
		ConnectionMedianLatencyMs:   float64(mustRandInt64n(t, 99) + 1),
		RxPackets:                   mustRandInt64n(t, 99) + 1,
		RxBytes:                     mustRandInt64n(t, 99) + 1,
		TxPackets:                   mustRandInt64n(t, 99) + 1,
		TxBytes:                     mustRandInt64n(t, 99) + 1,
		SessionCountVscode:          mustRandInt64n(t, 9) + 1,
		SessionCountJetbrains:       mustRandInt64n(t, 9) + 1,
		SessionCountReconnectingPty: mustRandInt64n(t, 9) + 1,
		SessionCountSsh:             mustRandInt64n(t, 9) + 1,
		Metrics:                     []*agentproto.Stats_Metric{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// deps is a set of test dependencies.
type deps struct {
	Agent     database.WorkspaceAgent
	Template  database.Template
	User      database.User
	Workspace database.WorkspaceTable
}

// setupDeps sets up a set of test dependencies.
// It creates an organization, user, template, workspace, and agent
// along with all the other miscellaneous plumbing required to link
// them together.
func setupDeps(t *testing.T, store database.Store, ps pubsub.Pubsub) deps {
	t.Helper()

	org := dbgen.Organization(t, store, database.Organization{})
	user := dbgen.User(t, store, database.User{})
	_, err := store.InsertOrganizationMember(context.Background(), database.InsertOrganizationMemberParams{
		OrganizationID: org.ID,
		UserID:         user.ID,
		Roles:          []string{codersdk.RoleOrganizationMember},
	})
	require.NoError(t, err)
	tv := dbgen.TemplateVersion(t, store, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tpl := dbgen.Template(t, store, database.Template{
		CreatedBy:       user.ID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, store, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		LastUsedAt:     time.Now().Add(-time.Hour),
	})
	pj := dbgen.ProvisionerJob(t, store, ps, database.ProvisionerJob{
		InitiatorID:    user.ID,
		OrganizationID: org.ID,
	})
	_ = dbgen.WorkspaceBuild(t, store, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       ws.ID,
		JobID:             pj.ID,
	})
	res := dbgen.WorkspaceResource(t, store, database.WorkspaceResource{
		Transition: database.WorkspaceTransitionStart,
		JobID:      pj.ID,
	})
	agt := dbgen.WorkspaceAgent(t, store, database.WorkspaceAgent{
		ResourceID: res.ID,
	})
	return deps{
		Agent:     agt,
		Template:  tpl,
		User:      user,
		Workspace: ws,
	}
}

// mustRandInt64n returns a random int64 in the range [0, n).
func mustRandInt64n(t *testing.T, n int64) int64 {
	t.Helper()
	i, err := cryptorand.Intn(int(n))
	require.NoError(t, err)
	return int64(i)
}
