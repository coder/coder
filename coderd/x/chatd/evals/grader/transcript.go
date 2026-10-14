package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

const (
	transcriptSchema   = "aegis.coderagents.transcript/1"
	transcriptFile     = "agent_native.ndjson"
	trialFile          = "trial.json"
	sourceAgentNative  = "agent_native"
	sourceTrialJSON    = "trial_json"
	maxTranscriptBytes = 256 << 20
	maxTranscriptLine  = 64 << 20
)

// transcript is one trial's chat as the coder-agents driver exported it,
// reduced to what the checks need.
type transcript struct {
	Source     string
	Chat       chatState
	Messages   []codersdk.ChatMessage
	Calls      []toolCall
	Workspaces []workspaceState
	// Notes are caveats a reader of the verdict needs, such as a missing
	// export.
	Notes []string
}

type chatState struct {
	ID           string          `json:"id"`
	Status       string          `json:"status"`
	WorkspaceID  *string         `json:"workspace_id"`
	PlanMode     string          `json:"plan_mode"`
	LastError    json.RawMessage `json:"last_error"`
	MessageCount int             `json:"message_count"`
}

type workspaceState struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	TemplateName string `json:"template_name"`
	LatestBuild  struct {
		Transition string `json:"transition"`
		Status     string `json:"status"`
		JobStatus  string `json:"job_status"`
		JobError   string `json:"job_error"`
	} `json:"latest_build"`
	Agents []struct {
		Name           string `json:"name"`
		Status         string `json:"status"`
		LifecycleState string `json:"lifecycle_state"`
	} `json:"agents"`
}

// toolCall pairs a model tool call with its result.
type toolCall struct {
	Name      string
	Args      json.RawMessage
	Result    json.RawMessage
	HasResult bool
	IsError   bool
}

// failure classifies a call as failed and returns the reason, or "" when
// the call succeeded. chatd reports several failures as successful tool
// results, so the result body is inspected too.
func (c toolCall) failure() string {
	if !c.HasResult {
		return "no_result"
	}
	if c.IsError {
		return "is_error"
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(c.Result, &body); err != nil {
		return ""
	}
	if raw, ok := body["error"]; ok && !emptyJSON(raw) {
		return "error"
	}
	switch jsonString(body["agent_status"]) {
	case "not_ready", "selection_error":
		return "agent_status:" + jsonString(body["agent_status"])
	}
	switch jsonString(body["startup_scripts"]) {
	case "startup_scripts_failed", "startup_scripts_timeout":
		return "startup_scripts:" + jsonString(body["startup_scripts"])
	}
	return ""
}

func (c toolCall) succeeded() bool { return c.failure() == "" }

// command returns the execute tool's shell command, or "".
func (c toolCall) command() string {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(c.Args, &args); err != nil {
		return ""
	}
	return args.Command
}

func (c toolCall) resultField(key string) string { return objectField(c.Result, key) }

func (c toolCall) argField(key string) string { return objectField(c.Args, key) }

func objectField(raw json.RawMessage, key string) string {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	return jsonString(body[key])
}

func emptyJSON(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null" || s == `""` || s == "{}" || s == "[]" || s == "false"
}

func jsonString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// loadTranscript reads agent_native.ndjson from the bundle directory and
// falls back to the proxy-observed tool calls in trial.json when the
// export is missing.
func loadTranscript(bundleDir, tracePath string) (*transcript, error) {
	data, err := readCapped(filepath.Join(bundleDir, transcriptFile), maxTranscriptBytes)
	if err == nil {
		return parseTranscript(data)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if tracePath == "" {
		tracePath = filepath.Join(bundleDir, trialFile)
	}
	trial, terr := readCapped(tracePath, maxTranscriptBytes)
	if terr != nil {
		return nil, xerrors.Errorf("no %s and no trial.json: %w", transcriptFile, terr)
	}
	tr, perr := parseTrialToolCalls(trial)
	if perr != nil {
		return nil, perr
	}
	tr.Notes = append(tr.Notes, transcriptFile+" missing, graded from trial.json tool calls without results or workspace state")
	return tr, nil
}

func readCapped(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, xerrors.Errorf("read %s: %w", path, err)
	}
	if int64(len(data)) > limit {
		return nil, xerrors.Errorf("%s exceeds %d bytes", path, limit)
	}
	return data, nil
}

func parseTranscript(data []byte) (*transcript, error) {
	tr := &transcript{Source: sourceAgentNative}
	sawChat := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1<<20), maxTranscriptLine)
	line := 0
	for scanner.Scan() {
		line++
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var rec struct {
			Schema     string           `json:"schema"`
			Type       string           `json:"type"`
			Chat       json.RawMessage  `json:"chat"`
			Message    json.RawMessage  `json:"message"`
			Workspaces []workspaceState `json:"workspaces"`
		}
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, xerrors.Errorf("transcript line %d: %w", line, err)
		}
		if rec.Schema != transcriptSchema {
			return nil, xerrors.Errorf("transcript line %d: schema %q, want %q", line, rec.Schema, transcriptSchema)
		}
		switch rec.Type {
		case "chat":
			if err := json.Unmarshal(rec.Chat, &tr.Chat); err != nil {
				return nil, xerrors.Errorf("transcript line %d: chat: %w", line, err)
			}
			sawChat = true
		case "message":
			var msg codersdk.ChatMessage
			if err := json.Unmarshal(rec.Message, &msg); err != nil {
				return nil, xerrors.Errorf("transcript line %d: message: %w", line, err)
			}
			tr.Messages = append(tr.Messages, msg)
		case "workspaces":
			tr.Workspaces = rec.Workspaces
		default:
			// Newer drivers may add record types; they carry nothing the
			// checks read.
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, xerrors.Errorf("scan transcript: %w", err)
	}
	if !sawChat {
		tr.Notes = append(tr.Notes, "transcript has no chat record")
	}
	sort.SliceStable(tr.Messages, func(i, j int) bool { return tr.Messages[i].ID < tr.Messages[j].ID })
	tr.Calls = pairToolCalls(tr.Messages)
	return tr, nil
}

func pairToolCalls(messages []codersdk.ChatMessage) []toolCall {
	var calls []toolCall
	byID := map[string]int{}
	for _, msg := range messages {
		for _, part := range msg.Content {
			switch part.Type {
			case codersdk.ChatMessagePartTypeToolCall:
				calls = append(calls, toolCall{
					Name: part.ToolName,
					Args: part.Args,
				})
				if part.ToolCallID != "" {
					byID[part.ToolCallID] = len(calls) - 1
				}
			case codersdk.ChatMessagePartTypeToolResult:
				idx, ok := byID[part.ToolCallID]
				if !ok {
					continue
				}
				calls[idx].Result = part.Result
				calls[idx].HasResult = true
				calls[idx].IsError = part.IsError
			default:
			}
		}
	}
	return calls
}

// parseTrialToolCalls builds a transcript from trial.json. The proxy sees
// tool inputs and error flags but not result bodies, so success-shaped
// failures are invisible in this mode.
func parseTrialToolCalls(data []byte) (*transcript, error) {
	var trial struct {
		ToolCalls []struct {
			Name   string          `json:"name"`
			Input  json.RawMessage `json:"input"`
			Result struct {
				Status  string `json:"status"`
				IsError bool   `json:"is_error"`
			} `json:"result"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal(data, &trial); err != nil {
		return nil, xerrors.Errorf("parse trial.json: %w", err)
	}
	tr := &transcript{Source: sourceTrialJSON}
	for _, c := range trial.ToolCalls {
		tr.Calls = append(tr.Calls, toolCall{
			Name:      c.Name,
			Args:      c.Input,
			Result:    json.RawMessage("{}"),
			HasResult: c.Result.Status != "unresolved",
			IsError:   c.Result.IsError,
		})
	}
	return tr, nil
}

// assistantParts returns every non-empty assistant text and reasoning part
// in order: what the conversation shows the user, since chatd's UI shows
// reasoning as a thinking block.
func (tr *transcript) assistantParts() []codersdk.ChatMessagePart {
	var out []codersdk.ChatMessagePart
	for _, msg := range tr.Messages {
		if msg.Role != codersdk.ChatMessageRoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			switch part.Type {
			case codersdk.ChatMessagePartTypeText, codersdk.ChatMessagePartTypeReasoning:
				if strings.TrimSpace(part.Text) != "" {
					out = append(out, part)
				}
			default:
			}
		}
	}
	return out
}

// finalText is the model's closing reply: the text of the last assistant
// message that has any, from after its last tool call or tool result.
// A turn that ends with a tool call or a tool result, because generation
// failed or stopped after the call, has no closing reply, so the text
// sent before the call does not count.
func (tr *transcript) finalText() string {
	for i := len(tr.Messages) - 1; i >= 0; i-- {
		msg := tr.Messages[i]
		var text []string
		toolPart := false
		for j := len(msg.Content) - 1; j >= 0 && !toolPart; j-- {
			switch part := msg.Content[j]; part.Type {
			case codersdk.ChatMessagePartTypeToolCall, codersdk.ChatMessagePartTypeToolResult:
				toolPart = true
			case codersdk.ChatMessagePartTypeText:
				if msg.Role == codersdk.ChatMessageRoleAssistant && strings.TrimSpace(part.Text) != "" {
					text = append(text, part.Text)
				}
			default:
			}
		}
		if len(text) > 0 || toolPart {
			slices.Reverse(text)
			return strings.Join(text, "\n")
		}
	}
	return ""
}

type toolStat struct {
	Calls    int
	Failures int
	Kinds    map[string]int
}

func toolStats(calls []toolCall) map[string]*toolStat {
	out := map[string]*toolStat{}
	for _, c := range calls {
		st, ok := out[c.Name]
		if !ok {
			st = &toolStat{Kinds: map[string]int{}}
			out[c.Name] = st
		}
		st.Calls++
		if kind := c.failure(); kind != "" {
			st.Failures++
			st.Kinds[kind]++
		}
	}
	return out
}
