// Package agentacp manages ephemeral ACP adapter sessions for Coder Agents.
package agentacp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// Manager owns adapter processes independently of tool and HTTP request contexts.
type Manager struct {
	mu             sync.Mutex
	sessions       map[uuid.UUID]*session
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	directory      func() string
	environment    func([]string) ([]string, error)
	commandContext func(context.Context, string, ...string) *exec.Cmd
	agentID        func() uuid.UUID
	clock          quartz.Clock
}

type session struct {
	mu            sync.Mutex
	data          codersdk.ACPSession
	organization  uuid.UUID
	conn          *acp.ClientSideConnection
	acpID         acp.SessionId
	queue         []string
	changed       chan struct{}
	active        bool
	dead          bool
	ctx           context.Context
	promptWritten chan struct{}
}

// New creates a manager whose sessions last until Close or parent cancellation.
func New(ctx context.Context, directory func() string, environment func([]string) ([]string, error), agentID func() uuid.UUID) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{sessions: make(map[uuid.UUID]*session), ctx: ctx, cancel: cancel, directory: directory, environment: environment, commandContext: exec.CommandContext, agentID: agentID, clock: quartz.NewReal()}
}

// Close stops adapters and waits for their processes to be reaped.
func (m *Manager) Close() {
	m.mu.Lock()
	m.cancel()
	m.mu.Unlock()
	m.wg.Wait()
	m.mu.Lock()
	clear(m.sessions)
	m.mu.Unlock()
}

func command(agent string) (string, error) {
	switch agent {
	case "claude_code":
		return "exec npx -y @agentclientprotocol/claude-agent-acp", nil
	case "codex":
		return "exec npx -y @agentclientprotocol/codex-acp", nil
	default:
		return "", xerrors.New("agent must be claude_code or codex")
	}
}

// Spawn initializes an adapter and accepts its first prompt.
func (m *Manager) Spawn(ctx context.Context, parent, organization uuid.UUID, req codersdk.ACPSpawnRequest) (codersdk.ACPSession, error) {
	script, err := command(req.Agent)
	if err != nil {
		return codersdk.ACPSession{}, err
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return codersdk.ACPSession{}, xerrors.New("prompt is required")
	}
	dir := m.directory()
	if dir == "" {
		dir, err = os.UserHomeDir()
		if err != nil {
			return codersdk.ACPSession{}, err
		}
	}
	if strings.HasPrefix(dir, "~/") {
		home, e := os.UserHomeDir()
		if e != nil {
			return codersdk.ACPSession{}, e
		}
		dir = filepath.Join(home, dir[2:])
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return codersdk.ACPSession{}, err
	}
	env, err := m.environment(os.Environ())
	if err != nil {
		return codersdk.ACPSession{}, xerrors.Errorf("workspace environment: %w", err)
	}
	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		return codersdk.ACPSession{}, xerrors.New("workspace agent is shutting down")
	}
	m.wg.Add(1)
	m.mu.Unlock()
	processCtx, cancel := context.WithCancel(m.ctx)
	cmd := m.commandContext(processCtx, "bash", "-c", script)
	cmd.Dir = dir
	cmd.Env = env
	configureProcess(cmd)
	var stderr tailBuffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		m.wg.Done()
		return codersdk.ACPSession{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cancel()
		m.wg.Done()
		return codersdk.ACPSession{}, err
	}
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		m.wg.Done()
		return codersdk.ACPSession{}, xerrors.Errorf("start ACP adapter: %w", err)
	}
	now := time.Now()
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = req.Agent
	}
	s := &session{data: codersdk.ACPSession{SessionID: uuid.New(), WorkspaceAgentID: m.agentID(), ParentChatID: parent, Agent: req.Agent, Title: title, Status: "starting", CreatedAt: now, UpdatedAt: now, Entries: []codersdk.ACPEntry{}}, organization: organization, changed: make(chan struct{}), ctx: processCtx}
	s.conn = acp.NewClientSideConnection(s, &promptWriter{Writer: stdin, session: s}, stdout)
	go func() {
		defer m.wg.Done()
		defer cancel()
		defer stdin.Close()
		defer stdout.Close()
		err := cmd.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.dead = true
		s.data.Status = "error"
		s.data.Error = "ACP adapter exited. Spawn a new agent to continue."
		if err != nil {
			s.data.Error = fmt.Sprintf("ACP adapter exited: %v. %s", err, stderr.String())
		}
		s.notifyLocked()
	}()
	initCtx, initCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer initCancel()
	initialized, err := s.conn.Initialize(initCtx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber})
	if err != nil {
		cancel()
		return codersdk.ACPSession{}, xerrors.Errorf("initialize ACP adapter: %w; %s", err, stderr.String())
	}
	if initialized.ProtocolVersion != acp.ProtocolVersionNumber {
		cancel()
		return codersdk.ACPSession{}, xerrors.Errorf("unsupported ACP protocol version %d", initialized.ProtocolVersion)
	}
	created, err := s.conn.NewSession(initCtx, acp.NewSessionRequest{Cwd: dir, McpServers: []acp.McpServer{}})
	if err != nil {
		cancel()
		return codersdk.ACPSession{}, xerrors.Errorf("create ACP session (check workspace credentials): %w", err)
	}
	s.mu.Lock()
	s.acpID = created.SessionId
	s.data.Status = "waiting"
	s.mu.Unlock()
	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		cancel()
		return codersdk.ACPSession{}, xerrors.New("workspace agent is shutting down")
	}
	m.sessions[s.data.SessionID] = s
	m.mu.Unlock()
	if err = s.message(req.Prompt, false); err != nil {
		return codersdk.ACPSession{}, err
	}
	return s.snapshot(), nil
}

func (m *Manager) lookup(id, parent, organization uuid.UUID) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || s.data.ParentChatID != parent || s.organization != organization {
		return nil, xerrors.New("ACP session expired or does not exist")
	}
	return s, nil
}

func (s *session) notifyLocked() {
	s.data.Version++
	s.data.UpdatedAt = time.Now()
	s.data.Queued = len(s.queue)
	close(s.changed)
	s.changed = make(chan struct{})
}
func (s *session) snapshot() codersdk.ACPSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}
func (s *session) snapshotLocked() codersdk.ACPSession {
	v := s.data
	v.Entries = append([]codersdk.ACPEntry{}, v.Entries...)
	return v
}

func (s *session) message(message string, interrupt bool) error {
	if strings.TrimSpace(message) == "" {
		return xerrors.New("message is required")
	}
	s.mu.Lock()
	if s.dead || s.ctx.Err() != nil {
		s.mu.Unlock()
		return xerrors.New("ACP adapter exited; spawn a new agent")
	}
	s.queue = append(s.queue, message)
	shouldInterrupt := interrupt && s.active
	if !s.active {
		s.active = true
		s.data.Status = "running"
		s.promptWritten = make(chan struct{})
		go s.run()
	}
	s.notifyLocked()
	s.mu.Unlock()
	if shouldInterrupt {
		return s.interrupt()
	}
	return nil
}

func (s *session) run() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 || s.dead || s.ctx.Err() != nil {
			s.active = false
			if !s.dead {
				s.data.Status = "waiting"
			}
			s.notifyLocked()
			s.mu.Unlock()
			return
		}
		if s.promptWritten == nil {
			s.promptWritten = make(chan struct{})
		}
		prompt := s.queue[0]
		s.queue = s.queue[1:]
		s.data.Entries = append(s.data.Entries, codersdk.ACPEntry{ID: uuid.NewString(), Role: "user", Kind: "text", Text: prompt})
		s.data.Status = "running"
		s.data.Error = ""
		s.notifyLocked()
		s.mu.Unlock()
		_, err := s.conn.Prompt(s.ctx, acp.PromptRequest{SessionId: s.acpID, Prompt: []acp.ContentBlock{acp.TextBlock(prompt)}})
		s.mu.Lock()
		s.promptWritten = nil
		s.mu.Unlock()
		if err != nil {
			s.mu.Lock()
			s.data.Status = "error"
			s.data.Error = err.Error()
			s.active = false
			s.queue = nil
			s.notifyLocked()
			s.mu.Unlock()
			return
		}
	}
}

func (s *session) interrupt() error {
	s.mu.Lock()
	if !s.active || s.dead {
		s.mu.Unlock()
		return nil
	}
	written := s.promptWritten
	s.data.Status = "interrupting"
	s.notifyLocked()
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	if written != nil {
		select {
		case <-written:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || s.promptWritten != written {
		return nil
	}
	return s.conn.Cancel(ctx, acp.CancelNotification{SessionId: s.acpID})
}

func (m *Manager) list(parent, organization uuid.UUID) []codersdk.ACPSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []codersdk.ACPSession{}
	for _, s := range m.sessions {
		if s.data.ParentChatID == parent && s.organization == organization {
			v := s.snapshot()
			v.Entries = nil
			result = append(result, v)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result
}

type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 8192 {
		b.data = append([]byte{}, b.data[len(b.data)-8192:]...)
	}
	return len(p), nil
}
func (b *tailBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }

var _ io.Writer = (*tailBuffer)(nil)

// promptWriter orders cancellation after the prompt has reached the adapter.
type promptWriter struct {
	io.Writer
	session *session
}

func (w *promptWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	var message struct {
		Method string `json:"method"`
	}
	if err == nil && json.Unmarshal(p, &message) == nil && message.Method == "session/prompt" {
		w.session.mu.Lock()
		if ch := w.session.promptWritten; ch != nil {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
		w.session.mu.Unlock()
	}
	return n, err
}
