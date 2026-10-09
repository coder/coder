package agentacp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
)

// TestACPHarness runs the test binary as a real deterministic ACP subprocess.
//
//nolint:paralleltest // The subprocess branch owns ACP stdout and exits directly.
func TestACPHarness(t *testing.T) {
	if os.Getenv("CODER_ACP_TEST_HELPER") != "1" {
		t.Parallel()
		return
	}
	mode := fakeHarnessMode(os.Getenv("CODER_ACP_TEST_MODE"))
	f := &fakeHarness{mode: mode, value: "default"}
	if value := os.Getenv("CODER_ACP_TEST_MODEL"); value != "" {
		f.value = value
	}
	if address := os.Getenv("CODER_ACP_TEST_READY_ADDR"); address != "" {
		ready, err := net.Dial("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		f.ready = ready
	}
	f.conn = acp.NewAgentSideConnection(f, os.Stdout, os.Stdin)
	<-f.conn.Done()
	// Keep the readiness connection open until the helper process exits.
	runtime.KeepAlive(f.ready)
	os.Exit(0) //nolint:revive // The subprocess must not write test output to ACP stdout.
}

type fakeNativeSession struct {
	Value string `json:"value"`
}

type fakeHarnessMode string

const (
	// fakeHarnessModeBoth advertises load and resume capabilities.
	fakeHarnessModeBoth fakeHarnessMode = "both"
	// fakeHarnessModeFail returns an initialization error to test reporting a
	// failed probe without preventing discovery of other harnesses.
	fakeHarnessModeFail fakeHarnessMode = "fail"
	// fakeHarnessModeHang blocks initialization until canceled to test probe
	// deadlines.
	fakeHarnessModeHang fakeHarnessMode = "hang"
)

type fakeHarness struct {
	acp.Agent
	conn  *acp.AgentSideConnection
	mode  fakeHarnessMode
	value string
	ready net.Conn
}

func (f *fakeHarness) Initialize(ctx context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	if req.ClientCapabilities.Fs.ReadTextFile || req.ClientCapabilities.Fs.WriteTextFile || req.ClientCapabilities.Terminal {
		return acp.InitializeResponse{}, xerrors.New("unexpected client capabilities")
	}
	if f.mode == fakeHarnessModeHang {
		if f.ready != nil {
			if _, err := f.ready.Write([]byte{1}); err != nil {
				return acp.InitializeResponse{}, err
			}
		}
		<-ctx.Done()
		return acp.InitializeResponse{}, ctx.Err()
	}
	var result acp.InitializeResponse
	_ = json.Unmarshal([]byte(`{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"resume":{}}}}`), &result)
	if f.mode == fakeHarnessModeFail {
		return result, xerrors.New("probe failed")
	}
	return result, nil
}

func (f *fakeHarness) options() []acp.SessionConfigOption {
	var options []acp.SessionConfigOption
	_ = json.Unmarshal([]byte(fmt.Sprintf(`[
		{"type":"select","id":"model","name":"Model","description":"Choose a model","currentValue":%q,"options":[{"value":"default","name":"Default","description":"Default model"},{"value":"other","name":"Other"}]},
		{"type":"select","id":"mode","name":"Mode","description":"Choose a mode","category":"mode","currentValue":"code","options":[
			{"group":"write","name":"Write","options":[{"value":"code","name":"Code","description":"Edit files"}]},
			{"group":"read","name":"Read","options":[{"value":"ask","name":"Ask"}]}
		]}
	]`, f.value)), &options)
	return options
}

func (*fakeHarness) check(cwd string, servers []acp.McpServer) error {
	actual, err := os.Getwd()
	if err != nil || cwd != actual || len(servers) != 0 {
		return xerrors.New("unexpected directory or MCP configuration")
	}
	return nil
}

func (f *fakeHarness) NewSession(_ context.Context, req acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if err := f.check(req.Cwd, req.McpServers); err != nil {
		return acp.NewSessionResponse{}, err
	}
	id := uuid.NewString()
	raw, _ := json.Marshal(fakeNativeSession{Value: f.value})
	err := os.WriteFile(id+".json", raw, 0o600)
	return acp.NewSessionResponse{SessionId: acp.SessionId(id), ConfigOptions: f.options()}, err
}

func fakeCommand(t *testing.T, mode fakeHarnessMode) string {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("CODER_ACP_TEST_HELPER=1 CODER_ACP_TEST_MODE=%s %s -test.run=^TestACPHarness$", mode, shellQuote(filepath.Clean(binary)))
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
