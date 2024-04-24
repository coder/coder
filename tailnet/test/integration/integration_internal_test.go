package integration

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/testutil"
)

var (
	isChild            = flag.Bool("child", false, "Run tests as a child")
	childTestID        = flag.Int("child-test-id", 0, "Which test is being run")
	childCoordinateURL = flag.String("child-coordinate-url", "", "The coordinate url to connect back to")
	childAgentID       = flag.String("child-agent-id", "", "The agent id of the child")
)

func TestMain(m *testing.M) {
	if run := os.Getenv("CODER_TAILNET_TESTS"); run == "" {
		_, _ = fmt.Println("skipping tests...")
		return
	}
	if os.Getuid() != 0 {
		_, _ = fmt.Println("networking integration tests must run as root")
		return
	}
	flag.Parse()
	os.Exit(m.Run())
}

var tests = []Test{{
	Name:               "Normal",
	DERPMap:            DERPMapTailscale,
	Coordinator:        CoordinatorInMemory,
	NetworkSetupParent: NetworkSetupDefault,
	NetworkSetupChild:  NetworkSetupDefault,
	TailnetSetupParent: TailnetSetupDRPC,
	TailnetSetupChild:  TailnetSetupDRPC,
	RunParent: func(ctx context.Context, t *testing.T, opts ParentOpts) {
		reach := opts.Conn.AwaitReachable(ctx, tailnet.IPFromUUID(opts.AgentID))
		assert.True(t, reach)
	},
	RunChild: func(ctx context.Context, t *testing.T, opts ChildOpts) {
		// wait until the parent kills us
		<-make(chan struct{})
	},
}}

//nolint:paralleltest
func TestIntegration(t *testing.T) {
	if *isChild {
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
		t.Cleanup(cancel)

		agentID, err := uuid.Parse(*childAgentID)
		require.NoError(t, err)

		test := tests[*childTestID]
		test.NetworkSetupChild(t)
		dm := test.DERPMap(ctx, t)
		conn := test.TailnetSetupChild(ctx, t, logger, agentID, uuid.Nil, *childCoordinateURL, dm)
		test.RunChild(ctx, t, ChildOpts{
			Logger:  logger,
			Conn:    conn,
			AgentID: agentID,
		})
		return
	}

	for id, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
			t.Cleanup(cancel)

			logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			parentID, childID := uuid.New(), uuid.New()
			dm := test.DERPMap(ctx, t)
			_, coordURL := test.Coordinator(t, logger, dm)

			child, waitChild := execChild(ctx, id, coordURL, childID)
			test.NetworkSetupParent(t)
			conn := test.TailnetSetupParent(ctx, t, logger, parentID, childID, coordURL, dm)
			test.RunParent(ctx, t, ParentOpts{
				Logger:   logger,
				Conn:     conn,
				ClientID: parentID,
				AgentID:  childID,
			})
			child.Process.Signal(syscall.SIGINT)
			<-waitChild
		})
	}
}

type Test struct {
	// Name is the name of the test.
	Name string

	// DERPMap returns the DERP map to use for both the parent and child. It is
	// called once at the beginning of the test.
	DERPMap func(ctx context.Context, t *testing.T) *tailcfg.DERPMap
	// Coordinator returns a running tailnet coordinator, and the url to reach
	// it on.
	Coordinator func(t *testing.T, logger slog.Logger, dm *tailcfg.DERPMap) (coord tailnet.Coordinator, url string)

	// NetworkSetup functions are run before all test code on both the parent
	// and the child. It can be used to setup a netns.
	NetworkSetupParent func(t *testing.T)
	NetworkSetupChild  func(t *testing.T)

	// TailnetSetup functions create tailnet networks.
	TailnetSetupParent func(
		ctx context.Context, t *testing.T, logger slog.Logger,
		id, agentID uuid.UUID, coordURL string, dm *tailcfg.DERPMap,
	) *tailnet.Conn
	TailnetSetupChild func(
		ctx context.Context, t *testing.T, logger slog.Logger,
		id, _ uuid.UUID, coordURL string, dm *tailcfg.DERPMap,
	) *tailnet.Conn

	// Run functions run the actual test. Parents and children run in separate
	// processes, so it's important to ensure no communication happens over
	// memory between these two functions.
	RunParent func(ctx context.Context, t *testing.T, opts ParentOpts)
	RunChild  func(ctx context.Context, t *testing.T, opts ChildOpts)
}

type ParentOpts struct {
	Logger   slog.Logger
	Conn     *tailnet.Conn
	ClientID uuid.UUID
	AgentID  uuid.UUID
}

type ChildOpts struct {
	Logger  slog.Logger
	Conn    *tailnet.Conn
	AgentID uuid.UUID
}

func execChild(ctx context.Context, testID int, coordURL string, agentID uuid.UUID) (*exec.Cmd, <-chan error) {
	ch := make(chan error)
	binary := os.Args[0]
	args := os.Args[1:]
	args = append(args,
		"--child=true",
		"--child-test-id="+strconv.Itoa(testID),
		"--child-coordinate-url="+coordURL,
		"--child-agent-id="+agentID.String(),
	)

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	go func() {
		ch <- cmd.Run()
	}()
	return cmd, ch
}
