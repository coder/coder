package agentacp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	f := &fakeHarness{mode: mode, ready: make(chan struct{}), value: "default"}
	f.conn = acp.NewAgentSideConnection(f, os.Stdout, os.Stdin)
	close(f.ready)
	<-f.conn.Done()
	os.Exit(0) //nolint:revive // The subprocess must not write test output to ACP stdout.
}

type fakeNativeSession struct {
	History []string `json:"history"`
	Value   string   `json:"value"`
}

type fakeHarnessMode string

const (
	// fakeHarnessModeBoth advertises load, resume, and steering support to test
	// normal operation and recovery method selection.
	fakeHarnessModeBoth fakeHarnessMode = "both"
	// fakeHarnessModeLoad advertises only load for recovery, testing history
	// replay and transcript epoch resets.
	fakeHarnessModeLoad fakeHarnessMode = "load"
	// fakeHarnessModeResume advertises only resume for recovery, testing
	// restoration without history replay.
	fakeHarnessModeResume fakeHarnessMode = "resume"
	// fakeHarnessModeNone disables load, resume, and steering to test unavailable
	// recovery and conflicts when messaging a busy session.
	fakeHarnessModeNone fakeHarnessMode = "none"
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
	ready chan struct{}
	mode  fakeHarnessMode
	value string
}

func (f *fakeHarness) Initialize(ctx context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	if req.ClientCapabilities.Fs.ReadTextFile || req.ClientCapabilities.Fs.WriteTextFile || req.ClientCapabilities.Terminal {
		return acp.InitializeResponse{}, xerrors.New("unexpected client capabilities")
	}
	if f.mode == fakeHarnessModeHang {
		<-ctx.Done()
		return acp.InitializeResponse{}, ctx.Err()
	}
	var result acp.InitializeResponse
	_ = json.Unmarshal([]byte(`{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"resume":{}}},"_meta":{"steering":{"supported":true}}}`), &result)
	if f.mode == fakeHarnessModeLoad {
		result.AgentCapabilities.SessionCapabilities.Resume = nil
	}
	if f.mode == fakeHarnessModeResume {
		result.AgentCapabilities.LoadSession = false
	}
	if f.mode == fakeHarnessModeNone {
		result.AgentCapabilities.LoadSession = false
		result.AgentCapabilities.SessionCapabilities.Resume = nil
		result.Meta = nil
	}
	if f.mode == fakeHarnessModeFail {
		return result, xerrors.New("probe failed")
	}
	return result, nil
}

func (f *fakeHarness) options() []acp.SessionConfigOption {
	var options []acp.SessionConfigOption
	_ = json.Unmarshal([]byte(fmt.Sprintf(`[{"type":"select","id":"model","name":"Model","description":"Choose a model","currentValue":%q,"options":[{"value":"default","name":"Default","description":"Default model"},{"value":"other","name":"Other"}]}]`, f.value)), &options)
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

func (f *fakeHarness) update(ctx context.Context, id acp.SessionId, kind, text string) error {
	<-f.ready
	raw, _ := json.Marshal(map[string]any{"sessionUpdate": kind, "content": map[string]any{"type": "text", "text": text}})
	var update acp.SessionUpdate
	_ = json.Unmarshal(raw, &update)
	return f.conn.SessionUpdate(ctx, acp.SessionNotification{SessionId: id, Update: update})
}

func (f *fakeHarness) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	text := req.Prompt[0].Text.Text
	if text == "exit" {
		os.Exit(7) //nolint:revive // Exercise a real harness process failure.
	}
	_ = f.update(ctx, req.SessionId, "agent_thought_chunk", "thinking")
	var tool acp.SessionUpdate
	_ = json.Unmarshal([]byte(`{"sessionUpdate":"tool_call","toolCallId":"call","title":"Inspect","kind":"read","status":"in_progress"}`), &tool)
	_ = f.conn.SessionUpdate(ctx, acp.SessionNotification{SessionId: req.SessionId, Update: tool})
	tool = acp.SessionUpdate{}
	_ = json.Unmarshal([]byte(`{"sessionUpdate":"tool_call_update","toolCallId":"call","status":"completed","content":[{"type":"content","content":{"type":"text","text":"tool result"}}]}`), &tool)
	_ = f.conn.SessionUpdate(ctx, acp.SessionNotification{SessionId: req.SessionId, Update: tool})
	if strings.HasPrefix(text, "hold") {
		_ = f.update(ctx, req.SessionId, "agent_message_chunk", "started")
		<-ctx.Done()
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	response := "answer:" + text + ":" + f.value
	_ = f.update(ctx, req.SessionId, "agent_message_chunk", response)
	raw, _ := os.ReadFile(string(req.SessionId) + ".json")
	var saved fakeNativeSession
	_ = json.Unmarshal(raw, &saved)
	saved.History = append(saved.History, text, response)
	raw, _ = json.Marshal(saved)
	_ = os.WriteFile(string(req.SessionId)+".json", raw, 0o600)
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (*fakeHarness) Cancel(context.Context, acp.CancelNotification) error { return nil }

func (f *fakeHarness) LoadSession(ctx context.Context, req acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	if err := f.check(req.Cwd, req.McpServers); err != nil {
		return acp.LoadSessionResponse{}, err
	}
	raw, err := os.ReadFile(string(req.SessionId) + ".json")
	if err != nil {
		return acp.LoadSessionResponse{}, err
	}
	var saved fakeNativeSession
	_ = json.Unmarshal(raw, &saved)
	f.value = saved.Value
	for i, text := range saved.History {
		kind := "user_message_chunk"
		if i%2 == 1 {
			kind = "agent_message_chunk"
		}
		_ = f.update(ctx, req.SessionId, kind, text)
	}
	return acp.LoadSessionResponse{ConfigOptions: f.options()}, nil
}

func (f *fakeHarness) ResumeSession(_ context.Context, req acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	if err := f.check(req.Cwd, req.McpServers); err != nil {
		return acp.ResumeSessionResponse{}, err
	}
	raw, err := os.ReadFile(string(req.SessionId) + ".json")
	var saved fakeNativeSession
	_ = json.Unmarshal(raw, &saved)
	f.value = saved.Value
	return acp.ResumeSessionResponse{ConfigOptions: f.options()}, err
}

func (f *fakeHarness) SetSessionConfigOption(_ context.Context, req acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	if req.ValueId == nil || req.ValueId.ConfigId != "model" {
		return acp.SetSessionConfigOptionResponse{}, xerrors.New("unknown config")
	}
	f.value = string(req.ValueId.Value)
	raw, _ := os.ReadFile(string(req.ValueId.SessionId) + ".json")
	var saved fakeNativeSession
	_ = json.Unmarshal(raw, &saved)
	saved.Value = f.value
	raw, _ = json.Marshal(saved)
	_ = os.WriteFile(string(req.ValueId.SessionId)+".json", raw, 0o600)
	return acp.SetSessionConfigOptionResponse{ConfigOptions: f.options()}, nil
}

func (*fakeHarness) HandleExtensionMethod(_ context.Context, method string, req json.RawMessage) (any, error) {
	if method != "_session/steering" {
		return nil, acp.NewMethodNotFound(method)
	}
	var message struct {
		Prompt []acp.ContentBlock `json:"prompt"`
	}
	_ = json.Unmarshal(req, &message)
	outcome := "injected"
	if message.Prompt[0].Text.Text == "new" {
		outcome = "startedNewTurn"
	}
	return map[string]any{"outcome": outcome}, nil
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
