package coderd_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestWorkspaceAgentACPExperiment(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		experiments []string
	}{
		{name: "Disabled"},
		{name: "NotIncludedInWildcard", experiments: []string{"*"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dv := coderdtest.DeploymentValues(t)
			dv.Experiments = tc.experiments
			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
			user := coderdtest.CreateFirstUser(t, client)
			workspace := newACPWorkspace(t, db, user.OrganizationID, user.UserID, "linux")

			ctx := testutil.Context(t, testutil.WaitLong)
			conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, client, workspace.Agents[0].ID, nil)
			require.Error(t, err)
			require.Nil(t, conn)
			require.Equal(t, http.StatusNotFound, statusCode)
		})
	}
}

func TestWorkspaceAgentACPAuthorization(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, acpTestOptions(t))
	admin := coderdtest.CreateFirstUser(t, client)
	owner, ownerUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	other, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleTemplateAdmin())
	workspace := newACPWorkspace(t, db, admin.OrganizationID, ownerUser.ID, "linux")
	anonymous := codersdk.New(client.URL, codersdk.WithHTTPClient(client.HTTPClient))
	proxy, secret := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})

	// Reading the agent is deliberately insufficient to execute commands.
	_, err := templateAdmin.WorkspaceAgent(testutil.Context(t, testutil.WaitLong), workspace.Agents[0].ID)
	require.NoError(t, err)

	for _, tc := range []struct {
		name       string
		client     *codersdk.Client
		headers    http.Header
		statusCode int
	}{
		{name: "NoAuthentication", client: anonymous, statusCode: http.StatusUnauthorized},
		{name: "OtherUser", client: other, statusCode: http.StatusNotFound},
		{name: "ReadableWithoutSSH", client: templateAdmin, statusCode: http.StatusNotFound},
		{
			name:   "WorkspaceProxy",
			client: anonymous,
			headers: http.Header{
				httpmw.WorkspaceProxyAuthTokenHeader: []string{fmt.Sprintf("%s:%s", proxy.ID, secret)},
			},
			statusCode: http.StatusUnauthorized,
		},
		{
			name:   "WorkspaceAgentToken",
			client: anonymous,
			headers: http.Header{
				codersdk.SessionTokenHeader: []string{workspace.AgentToken},
			},
			statusCode: http.StatusUnauthorized,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, tc.client, workspace.Agents[0].ID, tc.headers)
			require.Error(t, err)
			require.Nil(t, conn)
			require.Equal(t, tc.statusCode, statusCode)
		})
	}

	for _, agentOS := range []string{"darwin", "windows", ""} {
		t.Run("UnsupportedOS/"+agentOS, func(t *testing.T) {
			t.Parallel()

			workspace := newACPWorkspace(t, db, admin.OrganizationID, ownerUser.ID, agentOS)
			ctx := testutil.Context(t, testutil.WaitLong)
			conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, owner, workspace.Agents[0].ID, nil)
			require.Error(t, err)
			require.Nil(t, conn)
			require.Equal(t, http.StatusBadRequest, statusCode)
		})
	}
}

func TestWorkspaceAgentACPBrowserOnly(t *testing.T) {
	t.Parallel()

	client, _, api := coderdtest.NewWithAPI(t, acpTestOptions(t))
	user := coderdtest.CreateFirstUser(t, client)
	workspace := newACPWorkspace(t, api.Database, user.OrganizationID, user.UserID, "linux")
	blocked := make(chan struct{}, 1)
	override := func(w http.ResponseWriter) bool {
		blocked <- struct{}{}
		http.Error(w, "Non-browser connections are disabled for your deployment.", http.StatusConflict)
		return true
	}
	api.WorkspaceClientCoordinateOverride.Store(&override)

	ctx := testutil.Context(t, testutil.WaitLong)
	conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, client, workspace.Agents[0].ID, nil)
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusConflict, statusCode)
	testutil.TryReceive(ctx, t, blocked)
}

func TestWorkspaceAgentACPRelay(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the ACP bridge requires a Linux agent")
	}

	client, db := coderdtest.NewWithDatabase(t, acpTestOptions(t))
	admin := coderdtest.CreateFirstUser(t, client)
	owner, ownerUser := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

	setup := func(t *testing.T) (uuid.UUID, *acpTestExecer) {
		t.Helper()
		workspace := newACPWorkspace(t, db, admin.OrganizationID, ownerUser.ID, "linux")
		execer := newACPTestExecer(t)
		agenttest.New(t, client.URL, workspace.AgentToken, func(options *agent.Options) {
			options.Execer = execer
		})
		coderdtest.NewWorkspaceAgentWaiter(t, owner, workspace.Workspace.ID).Wait()
		return workspace.Agents[0].ID, execer
	}

	t.Run("BinaryBidirectionalAndNormalExit", func(t *testing.T) {
		t.Parallel()
		agentID, execer := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		conn, _, err := dialWorkspaceAgentACP(ctx, t, owner, agentID, http.Header{"Origin": []string{owner.URL.String()}})
		require.NoError(t, err)
		process := execer.accept(ctx, t)

		// NDJSON is a byte stream, not one JSON object per WebSocket frame.
		// Include a message larger than the default WebSocket read limit.
		payload := []byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n" +
			"{\"jsonrpc\":\"2.0\",\"id\":2,\"params\":\"" + strings.Repeat("x", 64*1024) + "\"}\n")
		for _, chunk := range [][]byte{payload[:17], payload[17 : len(payload)-3], payload[len(payload)-3:]} {
			require.NoError(t, conn.Write(ctx, websocket.MessageBinary, chunk))
		}
		requireACPBytes(ctx, t, conn, payload)
		process.command(t, "exit")
		requireACPClose(ctx, t, conn, websocket.StatusNormalClosure)
	})

	t.Run("CrossOriginRejected", func(t *testing.T) {
		t.Parallel()
		agentID, _ := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		conn, statusCode, err := dialWorkspaceAgentACP(ctx, t, owner, agentID, http.Header{"Origin": []string{"https://other.example.com"}})
		require.Error(t, err)
		require.Nil(t, conn)
		require.Equal(t, http.StatusForbidden, statusCode)
	})

	t.Run("StderrIsDrainedAndNotForwarded", func(t *testing.T) {
		t.Parallel()
		agentID, execer := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		conn, _, err := dialWorkspaceAgentACP(ctx, t, owner, agentID, nil)
		require.NoError(t, err)
		process := execer.accept(ctx, t)

		// More than an SSH channel window must drain without the browser
		// reading it, otherwise a verbose bridge can deadlock the protocol.
		process.command(t, "stderr")
		process.event(t, "stderr-drained")
		payload := []byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":null}\n")
		require.NoError(t, conn.Write(ctx, websocket.MessageBinary, payload))
		requireACPBytes(ctx, t, conn, payload)
		process.command(t, "exit")
		requireACPClose(ctx, t, conn, websocket.StatusNormalClosure)
	})

	t.Run("NonzeroExit", func(t *testing.T) {
		t.Parallel()
		agentID, execer := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		conn, _, err := dialWorkspaceAgentACP(ctx, t, owner, agentID, nil)
		require.NoError(t, err)
		process := execer.accept(ctx, t)
		process.command(t, "error")
		requireACPClose(ctx, t, conn, websocket.StatusInternalError)
	})

	t.Run("DisconnectClosesSSHWithoutKillingRuntime", func(t *testing.T) {
		t.Parallel()
		agentID, execer := setup(t)
		ctx := testutil.Context(t, testutil.WaitLong)
		conn, _, err := dialWorkspaceAgentACP(ctx, t, owner, agentID, nil)
		require.NoError(t, err)
		process := execer.accept(ctx, t)
		require.NoError(t, conn.Close(websocket.StatusNormalClosure, "disconnect"))

		process.event(t, "stdin-closed")
		testutil.TryReceive(ctx, t, process.sshContext.Done())
		// The stdio owner remains alive after EOF, modeling a runtime that
		// must not receive a kill signal when the browser disconnects.
		process.command(t, "ping")
		process.event(t, "pong")

		// The cached agent connection also remains usable by a new browser.
		reconnected, _, err := dialWorkspaceAgentACP(ctx, t, owner, agentID, nil)
		require.NoError(t, err)
		second := execer.accept(ctx, t)
		payload := []byte("{\"jsonrpc\":\"2.0\",\"method\":\"session/update\"}\n")
		require.NoError(t, reconnected.Write(ctx, websocket.MessageBinary, payload))
		requireACPBytes(ctx, t, reconnected, payload)
		second.command(t, "exit")
		requireACPClose(ctx, t, reconnected, websocket.StatusNormalClosure)
	})
}

func acpTestOptions(t *testing.T) *coderdtest.Options {
	t.Helper()
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentNativeAgents)}
	return &coderdtest.Options{DeploymentValues: dv}
}

func newACPWorkspace(t *testing.T, db database.Store, orgID, ownerID uuid.UUID, agentOS string) dbfake.WorkspaceResponse {
	t.Helper()
	return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
		OrganizationID: orgID,
		OwnerID:        ownerID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].OperatingSystem = agentOS
		return agents
	}).Do()
}

func dialWorkspaceAgentACP(ctx context.Context, t *testing.T, client *codersdk.Client, agentID uuid.UUID, headers http.Header) (*websocket.Conn, int, error) {
	t.Helper()
	headers = headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	if client.SessionToken() != "" {
		headers.Set(codersdk.SessionTokenHeader, client.SessionToken())
	}
	u := client.URL.JoinPath("api", "v2", "workspaceagents", agentID.String(), "experimental", "acp")
	//nolint:bodyclose // websocket.Dial closes the HTTP response body.
	conn, response, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{
		HTTPClient: client.HTTPClient,
		HTTPHeader: headers,
	})
	if conn != nil {
		conn.SetReadLimit(-1)
		t.Cleanup(func() { _ = conn.CloseNow() })
	}
	var statusCode int
	if response != nil {
		statusCode = response.StatusCode
	}
	return conn, statusCode, err
}

func requireACPBytes(ctx context.Context, t *testing.T, conn *websocket.Conn, want []byte) {
	t.Helper()
	var got []byte
	for len(got) < len(want) {
		messageType, data, err := conn.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, websocket.MessageBinary, messageType)
		got = append(got, data...)
	}
	require.Equal(t, want, got)
}

func requireACPClose(ctx context.Context, t *testing.T, conn *websocket.Conn, status websocket.StatusCode) {
	t.Helper()
	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	require.Equal(t, status, websocket.CloseStatus(err), "%v", err)
}

type acpTestCommand struct {
	ctx  context.Context
	args []string
}

// acpTestExecer substitutes only the agent's session command. The API still
// connects through the real tailnet and SSH server, with no host PATH changes.
type acpTestExecer struct {
	agentexec.Execer
	binary   string
	listener net.Listener
	commands chan acpTestCommand
}

func newACPTestExecer(t *testing.T) *acpTestExecer {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	return &acpTestExecer{
		Execer:   agentexec.DefaultExecer,
		binary:   binary,
		listener: listener,
		commands: make(chan acpTestCommand, 4),
	}
}

func (e *acpTestExecer) PTYCommandContext(ctx context.Context, _ string, args ...string) *pty.Cmd {
	e.commands <- acpTestCommand{ctx: ctx, args: slices.Clone(args)}
	return pty.CommandContext(ctx, e.binary, "-test.run=^TestWorkspaceAgentACPProcess$", "--", "--acp-test-process", e.listener.Addr().String())
}

func (e *acpTestExecer) accept(ctx context.Context, t *testing.T) *acpTestProcess {
	t.Helper()
	command := testutil.TryReceive(ctx, t, e.commands)
	require.Equal(t, []string{"-c", "coder-acp acp"}, command.args)
	stop := context.AfterFunc(ctx, func() { _ = e.listener.Close() })
	defer stop()
	conn, err := e.listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.NoError(t, conn.SetDeadline(deadline))
	process := &acpTestProcess{conn: conn, scanner: bufio.NewScanner(conn), sshContext: command.ctx}
	process.event(t, "ready")
	return process
}

type acpTestProcess struct {
	conn       net.Conn
	scanner    *bufio.Scanner
	sshContext context.Context
}

func (p *acpTestProcess) command(t *testing.T, command string) {
	t.Helper()
	_, err := fmt.Fprintln(p.conn, command)
	require.NoError(t, err)
}

func (p *acpTestProcess) event(t *testing.T, want string) {
	t.Helper()
	require.True(t, p.scanner.Scan(), "waiting for %q: %v", want, p.scanner.Err())
	require.Equal(t, want, p.scanner.Text())
}

// TestWorkspaceAgentACPProcess runs in a child test binary as the fake coder-acp
// command. Its control connection lets tests observe EOF and process lifetime
// without polling files or relying on timers. It does not implement inference.
func TestWorkspaceAgentACPProcess(t *testing.T) {
	t.Parallel()
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "--acp-test-process" {
		return
	}
	//nolint:revive // Avoid test-runner output on the child's protocol stdout.
	os.Exit(runACPTestProcess(t.Context(), os.Args[len(os.Args)-1]))
}

func runACPTestProcess(ctx context.Context, address string) int {
	if term.IsTerminal(int(os.Stdin.Fd())) || term.IsTerminal(int(os.Stdout.Fd())) {
		_, _ = fmt.Fprintln(os.Stderr, "ACP must not allocate a PTY")
		return 1
	}
	dialer := &net.Dialer{Timeout: testutil.WaitLong}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer conn.Close()
	_, _ = fmt.Fprintln(conn, "ready")
	go func() {
		_, err := io.Copy(os.Stdout, os.Stdin)
		if err != nil {
			_, _ = fmt.Fprintln(conn, "stdio-error")
			return
		}
		_, _ = fmt.Fprintln(conn, "stdin-closed")
	}()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		switch scanner.Text() {
		case "stderr":
			if _, err := io.WriteString(os.Stderr, strings.Repeat("private stderr, not protocol\n", 1<<18)); err != nil {
				return 1
			}
			_, _ = fmt.Fprintln(conn, "stderr-drained")
		case "ping":
			_, _ = fmt.Fprintln(conn, "pong")
		case "exit":
			return 0
		case "error":
			return 17
		default:
			return 1
		}
	}
	return 0
}
