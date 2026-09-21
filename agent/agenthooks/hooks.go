package agenthooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

// PROTOTYPE (CODAGT-1083). This package runs user-declared hooks
// inside the workspace before a chat tool executes. Shortcuts taken
// on purpose, tracked so they do not survive promotion:
//   - the hooks file is re-read on every tool call (no caching, no
//     pinning to the context snapshot, no integrity hash);
//   - hooks inherit the agent process environment (no allowlist);
//   - matchers are exact tool names or "*" (no regex, no argument
//     filters);
//   - every failure (crash, timeout, malformed output) fails closed
//     for pre_tool_use, with no per-hook override yet;
//   - the tool input a hook sees is rebuilt by the agent handler from
//     the HTTP request, so it can lag the chattool argument schema.

const (
	// DefaultFileName is the hooks file looked up in the workspace
	// working directory and in the user's home directory.
	DefaultFileName = ".coder/hooks.json"
	// hardcoded for prototype
	defaultTimeout = 5 * time.Second
	maxOutputBytes = 64 << 10
)

// File is the on-disk hooks declaration.
type File struct {
	Version int    `json:"version"`
	Hooks   []Hook `json:"hooks"`
}

// Hook declares one command to run for one event.
type Hook struct {
	Name    string `json:"name"`
	Event   string `json:"event"`
	Matcher string `json:"matcher,omitempty"`
	Command string `json:"command"`
	Timeout string `json:"timeout,omitempty"`
}

// Runner locates and runs hooks declared in the workspace.
type Runner struct {
	logger     slog.Logger
	execer     agentexec.Execer
	fs         afero.Fs
	envInfo    usershell.EnvInfoer
	workingDir func() string
	fileName   string
}

// New returns a Runner that reads hooks from fileName relative to the
// workspace working directory and the user's home directory.
func New(logger slog.Logger, execer agentexec.Execer, fs afero.Fs, envInfo usershell.EnvInfoer, workingDir func() string, fileName string) *Runner {
	if envInfo == nil {
		envInfo = &usershell.SystemEnvInfo{}
	}
	if fileName == "" {
		fileName = DefaultFileName
	}
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return &Runner{
		logger:     logger,
		execer:     execer,
		fs:         fs,
		envInfo:    envInfo,
		workingDir: workingDir,
		fileName:   fileName,
	}
}

type located struct {
	Hook
	dir string
}

// load reads every hooks file that exists, workspace first, then
// home. A missing file is not an error; a malformed one is, because a
// guard that silently fails to load is a guard that is not there.
func (r *Runner) load() ([]located, error) {
	var roots []string
	var configured string
	if r.workingDir != nil {
		configured = r.workingDir()
	}
	if dir, err := usershell.ResolveWorkingDirectory(r.fs, r.envInfo, configured); err == nil && dir != "" {
		roots = append(roots, dir)
	}
	if home, err := r.envInfo.HomeDir(); err == nil && home != "" {
		roots = append(roots, home)
	}

	var hooks []located
	seen := map[string]bool{}
	for _, root := range roots {
		path := filepath.Join(root, r.fileName)
		if seen[path] {
			continue
		}
		seen[path] = true
		data, err := afero.ReadFile(r.fs, path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, xerrors.Errorf("read hooks file %s: %w", path, err)
		}
		var file File
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&file); err != nil {
			return nil, xerrors.Errorf("parse hooks file %s: %w", path, err)
		}
		for _, h := range file.Hooks {
			hooks = append(hooks, located{Hook: h, dir: filepath.Dir(path)})
		}
	}
	return hooks, nil
}

// ToolCall identifies the tool invocation a hook is asked about.
type ToolCall struct {
	ChatID    uuid.UUID
	ToolUseID string
	ToolName  string
	// Input is the tool input as the hook sees it. It must marshal to
	// the same JSON shape an input_override is expected to use.
	Input any
}

// Outcome is the combined result of every hook that ran for one call.
type Outcome struct {
	Decisions []workspacesdk.HookDecision
	// Input is the final tool input after any input_override, or nil
	// when no hook rewrote it.
	Input json.RawMessage
}

// Denied reports whether any hook denied the call.
func (o Outcome) Denied() bool {
	for _, d := range o.Decisions {
		if d.Decision == workspacesdk.HookDecisionDeny {
			return true
		}
	}
	return false
}

// Rewritten reports whether a hook replaced the tool input.
func (o Outcome) Rewritten() bool {
	return len(o.Input) > 0
}

// PreToolHook is the seam agent handlers call before doing tool work.
// A nil hook means hooks are disabled.
type PreToolHook func(ctx context.Context, call ToolCall) Outcome

// Gate runs hook for call and, when it denies, writes the 403 denial
// response. It returns false when the handler must stop. Handlers
// pass the request so the chat and tool call IDs come from the same
// headers chatd already sends.
func Gate(rw http.ResponseWriter, r *http.Request, hook PreToolHook, toolName string, input any) (Outcome, bool) {
	if hook == nil {
		return Outcome{}, true
	}
	ctx := r.Context()
	call := ToolCall{
		ToolUseID: r.Header.Get(workspacesdk.CoderToolCallIDHeader),
		ToolName:  toolName,
		Input:     input,
	}
	if chatContext, ok := agentchat.FromContext(ctx); ok {
		call.ChatID = chatContext.ID
	}
	outcome := hook(ctx, call)
	if !outcome.Denied() {
		return outcome, true
	}
	rw.Header().Set(workspacesdk.CoderWorkspaceHookHeader, workspacesdk.HookDecisionDeny)
	httpapi.Write(ctx, rw, http.StatusForbidden, workspacesdk.HookDeniedResponse{
		Message: (&workspacesdk.HookDeniedError{Hooks: outcome.Decisions}).Error(),
		Hooks:   outcome.Decisions,
	})
	return outcome, false
}

// PreToolUse runs every pre_tool_use hook matching the call, in
// declaration order, stopping at the first deny. An allow with
// input_override replaces the input for the remaining hooks and for
// the tool. A load failure is reported as a single denying decision so
// the caller does not have to distinguish "no hooks" from "hooks
// broken".
func (r *Runner) PreToolUse(ctx context.Context, call ToolCall) Outcome {
	denyAll := func(reason string, err error) Outcome {
		return Outcome{Decisions: []workspacesdk.HookDecision{{
			Event:    string(agenthooks.EventPreToolUse),
			Decision: workspacesdk.HookDecisionDeny,
			Reason:   reason,
			Error:    err.Error(),
		}}}
	}
	hooks, err := r.load()
	if err != nil {
		return denyAll("hooks file could not be loaded", err)
	}

	input, err := json.Marshal(call.Input)
	if err != nil {
		return denyAll("tool input could not be encoded", err)
	}

	var out Outcome
	for _, h := range hooks {
		if h.Event != string(agenthooks.EventPreToolUse) {
			continue
		}
		if h.Matcher != "" && h.Matcher != "*" && h.Matcher != call.ToolName {
			continue
		}
		current := input
		if out.Input != nil {
			current = out.Input
		}
		data, _ := json.Marshal(agenthooks.PreToolUseData{
			ToolUseID: call.ToolUseID,
			ToolName:  call.ToolName,
			ToolInput: current,
		})
		req := agenthooks.Request{
			Type: agenthooks.EventPreToolUse,
			Meta: agenthooks.Meta{
				DispatchID:    uuid.New(),
				SchemaVersion: agenthooks.SchemaVersion,
				ChatRef:       agenthooks.ChatRef{ChatID: call.ChatID},
			},
			Data: data,
		}
		d := r.run(ctx, h, req)
		out.Decisions = append(out.Decisions, d)
		if d.Decision == workspacesdk.HookDecisionDeny {
			break
		}
		if len(d.InputOverride) > 0 {
			out.Input = d.InputOverride
		}
	}
	return out
}

func (r *Runner) run(ctx context.Context, h located, req agenthooks.Request) workspacesdk.HookDecision {
	decision := workspacesdk.HookDecision{
		Hook:     h.Name,
		Event:    h.Event,
		Decision: string(agenthooks.PermissionAllow),
	}
	deny := func(reason string, err error) workspacesdk.HookDecision {
		decision.Decision = string(agenthooks.PermissionDeny)
		decision.Reason = reason
		if err != nil {
			decision.Error = err.Error()
		}
		return decision
	}

	timeout := defaultTimeout
	if h.Timeout != "" {
		if parsed, err := time.ParseDuration(h.Timeout); err == nil {
			timeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(req)
	if err != nil {
		return deny("hook request could not be encoded", err)
	}

	cmd := r.execer.CommandContext(ctx, "sh", "-c", h.Command)
	cmd.Dir = h.dir
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, limit: maxOutputBytes}
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: maxOutputBytes}
	cmd.Env = append(os.Environ(),
		"CODER_CHAT_ID="+req.Meta.ChatID.String(),
		"CODER_HOOK_ROOT="+h.dir,
		"CODER_HOOK_EVENT="+string(req.Type),
	)

	start := time.Now()
	runErr := cmd.Run()
	decision.DurationMs = time.Since(start).Milliseconds()

	if runErr != nil {
		if ctx.Err() != nil {
			return deny(fmt.Sprintf("hook %q timed out after %s", h.Name, timeout), runErr)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return deny(fmt.Sprintf("hook %q exited with status %d: %s", h.Name, exitErr.ExitCode(), tail(stderr.String())), runErr)
		}
		return deny(fmt.Sprintf("hook %q could not be started", h.Name), runErr)
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return decision
	}
	var resp agenthooks.Response
	dec := json.NewDecoder(strings.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&resp); err != nil {
		return deny(fmt.Sprintf("hook %q returned malformed output", h.Name), err)
	}
	decision.ModelContext = resp.ModelContext
	decision.UserMessage = resp.UserMessage
	if resp.Permission != nil {
		switch resp.Permission.Decision {
		case agenthooks.PermissionAllow:
			if len(resp.Permission.InputOverride) > 0 && !json.Valid(resp.Permission.InputOverride) {
				return deny(fmt.Sprintf("hook %q returned an invalid input_override", h.Name), nil)
			}
			decision.InputOverride = resp.Permission.InputOverride
		case agenthooks.PermissionDeny:
			decision.Decision = string(agenthooks.PermissionDeny)
			decision.Reason = resp.Permission.Reason
		default:
			return deny(fmt.Sprintf("hook %q returned unknown decision %q", h.Name, resp.Permission.Decision), nil)
		}
	}
	r.logger.Debug(ctx, "workspace hook ran",
		slog.F("hook", h.Name),
		slog.F("decision", decision.Decision),
		slog.F("duration_ms", decision.DurationMs),
	)
	return decision
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return "..." + s[len(s)-512:]
	}
	return s
}

type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return n, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = w.buf.Write(p)
	return n, nil
}
