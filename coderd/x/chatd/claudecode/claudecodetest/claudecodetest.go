// Package claudecodetest provides an in-memory fake ACP agent and
// transport for testing the claudecode runtime without a workspace.
package claudecodetest

import (
	"context"
	"io"
	stdslog "log/slog"
	"slices"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/x/chatd/claudecode"
)

// FakeAgent implements the agent side of ACP for tests. Behavior is
// configured per test via the handler funcs; nil handlers use
// permissive defaults. Recorded requests are readable through the
// accessor methods once the turn has finished.
type FakeAgent struct {
	mu   sync.Mutex
	conn *acp.AgentSideConnection

	// Capabilities is advertised in the initialize response.
	Capabilities acp.AgentCapabilities

	// OnPrompt handles session/prompt. The conn parameter sends
	// session updates back to the client.
	OnPrompt func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error)
	// OnResumeSession rejects or accepts session/resume.
	OnResumeSession func(params acp.ResumeSessionRequest) error
	// OnLoadSession replays history for session/load.
	OnLoadSession func(ctx context.Context, conn *acp.AgentSideConnection, params acp.LoadSessionRequest) error

	newSessions    []acp.NewSessionRequest
	resumeSessions []acp.ResumeSessionRequest
	loadSessions   []acp.LoadSessionRequest
	prompts        []acp.PromptRequest
	cancels        []acp.CancelNotification
	modes          []acp.SetSessionModeRequest
}

var (
	_ acp.Agent       = (*FakeAgent)(nil)
	_ acp.AgentLoader = (*FakeAgent)(nil)
)

func (a *FakeAgent) setConn(conn *acp.AgentSideConnection) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.conn = conn
}

// NewSessions returns recorded session/new requests.
func (a *FakeAgent) NewSessions() []acp.NewSessionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.newSessions)
}

// ResumeSessions returns recorded session/resume requests.
func (a *FakeAgent) ResumeSessions() []acp.ResumeSessionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.resumeSessions)
}

// LoadSessions returns recorded session/load requests.
func (a *FakeAgent) LoadSessions() []acp.LoadSessionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.loadSessions)
}

// Prompts returns recorded session/prompt requests.
func (a *FakeAgent) Prompts() []acp.PromptRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.prompts)
}

// Cancels returns recorded session/cancel notifications.
func (a *FakeAgent) Cancels() []acp.CancelNotification {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.cancels)
}

// Modes returns recorded session/set_mode requests.
func (a *FakeAgent) Modes() []acp.SetSessionModeRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.modes)
}

func (a *FakeAgent) Initialize(_ context.Context, _ acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion:   acp.ProtocolVersionNumber,
		AgentCapabilities: a.Capabilities,
	}, nil
}

func (a *FakeAgent) NewSession(_ context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.newSessions = append(a.newSessions, params)
	return acp.NewSessionResponse{SessionId: "session-new"}, nil
}

func (a *FakeAgent) ResumeSession(_ context.Context, params acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	a.mu.Lock()
	a.resumeSessions = append(a.resumeSessions, params)
	handler := a.OnResumeSession
	a.mu.Unlock()
	if handler != nil {
		if err := handler(params); err != nil {
			return acp.ResumeSessionResponse{}, err
		}
	}
	return acp.ResumeSessionResponse{}, nil
}

func (a *FakeAgent) LoadSession(ctx context.Context, params acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	a.mu.Lock()
	a.loadSessions = append(a.loadSessions, params)
	handler := a.OnLoadSession
	conn := a.conn
	a.mu.Unlock()
	if handler != nil {
		if err := handler(ctx, conn, params); err != nil {
			return acp.LoadSessionResponse{}, err
		}
	}
	return acp.LoadSessionResponse{}, nil
}

func (a *FakeAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	a.mu.Lock()
	a.prompts = append(a.prompts, params)
	handler := a.OnPrompt
	conn := a.conn
	a.mu.Unlock()
	if handler != nil {
		return handler(ctx, conn, params)
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (a *FakeAgent) Cancel(_ context.Context, params acp.CancelNotification) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancels = append(a.cancels, params)
	return nil
}

func (a *FakeAgent) SetSessionMode(_ context.Context, params acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.modes = append(a.modes, params)
	return acp.SetSessionModeResponse{}, nil
}

func (*FakeAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (*FakeAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}

func (*FakeAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (*FakeAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (*FakeAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

// PipeTransport serves the fake agent over in-memory pipes, standing
// in for the SSH exec channel. Each Start call wires a fresh
// agent-side connection, mirroring the process-per-turn lifecycle.
type PipeTransport struct {
	Agent *FakeAgent
}

var _ claudecode.Transport = (*PipeTransport)(nil)

func (t *PipeTransport) Start(_ context.Context) (claudecode.Process, error) {
	clientReads, agentWrites := io.Pipe()
	agentReads, clientWrites := io.Pipe()
	conn := acp.NewAgentSideConnection(t.Agent, agentWrites, agentReads)
	// Mirrors RunTurn: slog.Default()'s handler is not race-safe.
	conn.SetLogger(stdslog.New(stdslog.DiscardHandler))
	t.Agent.setConn(conn)
	return &pipeProcess{
		stdin:  clientWrites,
		stdout: clientReads,
		conn:   conn,
	}, nil
}

type pipeProcess struct {
	stdin  io.WriteCloser
	stdout *io.PipeReader
	conn   *acp.AgentSideConnection
}

func (p *pipeProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *pipeProcess) Stdout() io.Reader     { return p.stdout }

func (p *pipeProcess) Wait() error {
	<-p.conn.Done()
	return nil
}

func (p *pipeProcess) Close() error {
	_ = p.stdin.Close()
	_ = p.stdout.Close()
	return nil
}
