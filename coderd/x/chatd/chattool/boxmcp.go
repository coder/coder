package chattool

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// Limits on guest-initiated MCP calls in one box_run.
const (
	BoxMCPMaxCallsPerRun      = 200
	BoxMCPMaxResultBytes      = 8 << 20
	BoxMCPMaxTotalResultBytes = 64 << 20
	// boxMCPCallTimeout bounds one call; the run deadline usually ends it
	// sooner.
	boxMCPCallTimeout = 60 * time.Second
	// boxMCPMaxCallEntries caps the mcp_calls list in a box_run result.
	boxMCPMaxCallEntries = 25
	// boxMCPLogNameLimit truncates guest-supplied tool names in logs.
	boxMCPLogNameLimit = 128
)

// Envelope codes returned to the guest by the dispatcher. The channel
// itself adds request_too_large, canceled, and internal, and the prelude
// adds unavailable.
const (
	boxMCPCodeUnknownTool    = "unknown_tool"
	boxMCPCodeAmbiguousTool  = "ambiguous_tool"
	boxMCPCodeInvalidArgs    = "invalid_args"
	boxMCPCodeInvalidRequest = "invalid_request"
	boxMCPCodeCallLimit      = "call_limit"
	boxMCPCodeResultBudget   = "result_budget"
	boxMCPCodeResultTooLarge = "result_too_large"
	boxMCPCodeTimeout        = "timeout"
	boxMCPCodeCallFailed     = "call_failed"
)

// BoxMCPOptions configures guest MCP calls from a box.
type BoxMCPOptions struct {
	// Tools returns the turn's executable tools after the plan and Explore
	// filters. Only tools implementing mcpclient.RawCaller are reachable.
	// It is called once per box_run.
	Tools func() []fantasy.AgentTool
	// ServerName returns the display name of the server behind tool, or
	// "" when unknown.
	ServerName func(tool fantasy.AgentTool) string
	// Servers lists the names of the servers reachable in the turn, for
	// the box_run description.
	Servers []string
	Logger  slog.Logger
}

// boxMCPDescription is appended to the box_run description when the turn
// has MCP tools.
func boxMCPDescription(opts BoxMCPOptions) string {
	servers := ""
	if len(opts.Servers) > 0 {
		servers = " Servers: " + strings.Join(opts.Servers, ", ") + "."
	}
	return "The program can call this chat's MCP tools synchronously: mcp.tools() lists them as {name, server, description} (including tools not yet loaded with find_tools), " +
		"mcp.schema(name) returns a tool's input schema, mcp.call(name, args) returns the MCP result as {content, structuredContent, isError}, and mcp.text(result) joins its text blocks. " +
		"A result with isError true is returned, not thrown. mcp.call throws an MCPError whose message starts with one of these codes: " +
		"[unknown_tool] or [ambiguous_tool] (check mcp.tools()), [invalid_args] (args must be an object), [call_limit] (no more calls this run), " +
		"[result_too_large] (one result over the per-call limit; request less), [result_budget] (the run's total result limit; stop calling), " +
		"[request_too_large] (args over " + byteCountString(agentbox.MaxHostCallRequestBytes) + "), [timeout], [canceled] (the run was stopped), [call_failed] (the server or transport failed), [unavailable] (no MCP in this run)." + servers +
		" Calls run one at a time and their waiting time counts against the run's time limit, so write intermediate results to /box between runs and keep only what you need from each result. " +
		"Limits: " + strconv.Itoa(BoxMCPMaxCallsPerRun) + " calls per run, " + byteCountString(BoxMCPMaxResultBytes) + " per result, " + byteCountString(BoxMCPMaxTotalResultBytes) + " of results per run. " +
		"The result lists calls in mcp_calls (n, tool, ok, code, duration_ms, result_bytes as the redacted JSON size) in call order, keeping at most " + strconv.Itoa(boxMCPMaxCallEntries) + " entries with failures preferred, " +
		"with mcp_calls_total (calls made), mcp_calls_failed, and mcp_calls_rejected (calls refused by the call or budget limit, not counted in the total); limits_hit lists \"mcp_request\" when a request exceeded " + byteCountString(agentbox.MaxHostCallRequestBytes) + "."
}

// boxMCPTool is one entry of mcp.tools().
type boxMCPTool struct {
	Name        string `json:"name"`
	Server      string `json:"server,omitempty"`
	Description string `json:"description,omitempty"`
}

// boxMCPCall is one entry of a box_run result's mcp_calls.
type boxMCPCall struct {
	// N is the call's position in the run, starting at 1. Rejected calls
	// share the position of the call they would have been.
	N           int    `json:"n"`
	Tool        string `json:"tool"`
	OK          bool   `json:"ok"`
	Code        string `json:"code,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	ResultBytes int    `json:"result_bytes"`
}

// boxMCPSummary is what a run's guest calls add to the box_run result.
type boxMCPSummary struct {
	Calls  []boxMCPCall
	Total  int
	Failed int
	// Rejected counts calls refused by the call or budget limit before
	// reaching a server.
	Rejected int
	// ResultBytes is the redacted, marshaled size of every result.
	ResultBytes int64
	Duration    time.Duration
}

// boxMCPRequest is what the prelude writes.
type boxMCPRequest struct {
	Op   string          `json:"op"`
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
}

// boxMCPRun dispatches one run's guest calls.
type boxMCPRun struct {
	opts   BoxMCPOptions
	callID string
	tools  []mcpclient.RawCaller
	byName map[string][]mcpclient.RawCaller

	mu      sync.Mutex
	totals  boxMCPSummary
	entries []boxMCPCall
}

// newBoxMCPRun returns a dispatcher when the turn has at least one MCP
// tool, otherwise nil so the run leaves /mcp unmounted.
func newBoxMCPRun(opts BoxMCPOptions, callID string) *boxMCPRun {
	if opts.Tools == nil {
		return nil
	}
	r := &boxMCPRun{opts: opts, callID: callID, byName: map[string][]mcpclient.RawCaller{}}
	for _, tool := range opts.Tools() {
		caller, ok := tool.(mcpclient.RawCaller)
		if !ok {
			continue
		}
		r.tools = append(r.tools, caller)
		name := tool.Info().Name
		r.byName[name] = append(r.byName[name], caller)
	}
	if len(r.tools) == 0 {
		return nil
	}
	return r
}

func (r *boxMCPRun) serverName(tool fantasy.AgentTool) string {
	if r.opts.ServerName == nil {
		return ""
	}
	return r.opts.ServerName(tool)
}

// hostCall is the RunRequest.HostCall for the run.
func (r *boxMCPRun) hostCall(ctx context.Context, request []byte) ([]byte, error) {
	var req boxMCPRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return agentbox.HostCallError(boxMCPCodeInvalidRequest, "malformed request: "+err.Error()), nil
	}
	switch req.Op {
	case "tools":
		return r.listTools()
	case "schema":
		return r.schema(req.Tool)
	case "call":
		return r.call(ctx, req), nil
	default:
		return agentbox.HostCallError(boxMCPCodeInvalidRequest, "unknown op "+strconv.Quote(truncateForLog(req.Op))), nil
	}
}

func (r *boxMCPRun) listTools() ([]byte, error) {
	list := make([]boxMCPTool, 0, len(r.tools))
	for _, tool := range r.tools {
		info := tool.Info()
		list = append(list, boxMCPTool{Name: info.Name, Server: r.serverName(tool), Description: info.Description})
	}
	slices.SortStableFunc(list, func(a, b boxMCPTool) int { return strings.Compare(a.Name, b.Name) })
	out, err := json.Marshal(list)
	if err != nil {
		return nil, xerrors.Errorf("encode tools: %w", err)
	}
	return agentbox.HostCallResult(out), nil
}

// lookup resolves a guest-supplied tool name. On failure it returns the
// envelope code and message.
func (r *boxMCPRun) lookup(name string) (tool mcpclient.RawCaller, code, msg string) {
	matches := r.byName[name]
	switch len(matches) {
	case 0:
		return nil, boxMCPCodeUnknownTool, "unknown tool " + strconv.Quote(truncateForLog(name)) + "; mcp.tools() lists the tools available in this run"
	case 1:
		return matches[0], "", ""
	default:
		return nil, boxMCPCodeAmbiguousTool, "tool name " + strconv.Quote(name) + " belongs to more than one server in this turn"
	}
}

func (r *boxMCPRun) schema(name string) ([]byte, error) {
	tool, code, msg := r.lookup(name)
	if tool == nil {
		return agentbox.HostCallError(code, msg), nil
	}
	info := tool.Info()
	params, required := info.Parameters, info.Required
	// Org tools with model_intent wrap their real schema under
	// "properties"; unwrap so the guest sees what the server expects.
	if slices.Equal(required, []string{"model_intent", "properties"}) {
		if inner, ok := params["properties"].(map[string]any); ok {
			innerProps, _ := inner["properties"].(map[string]any)
			params = innerProps
			required = stringSlice(inner["required"])
		}
	}
	if params == nil {
		params = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	out, err := json.Marshal(map[string]any{
		"type":       "object",
		"properties": params,
		"required":   required,
	})
	if err != nil {
		return nil, xerrors.Errorf("encode schema: %w", err)
	}
	return agentbox.HostCallResult(out), nil
}

// stringSlice reads a []string or a []any of strings; anything else is
// nil.
func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func (r *boxMCPRun) call(ctx context.Context, req boxMCPRequest) []byte {
	r.mu.Lock()
	if r.totals.Total >= BoxMCPMaxCallsPerRun {
		n := r.totals.Total + 1
		r.mu.Unlock()
		return r.reject(ctx, req.Tool, n, boxMCPCodeCallLimit, "this run already made "+strconv.Itoa(BoxMCPMaxCallsPerRun)+" MCP calls")
	}
	r.totals.Total++
	n := r.totals.Total
	r.mu.Unlock()

	tool, code, msg := r.lookup(req.Tool)
	if tool == nil {
		return r.fail(ctx, req.Tool, n, 0, 0, code, msg)
	}
	var args map[string]any
	if len(req.Args) > 0 && string(req.Args) != "null" {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return r.fail(ctx, req.Tool, n, 0, 0, boxMCPCodeInvalidArgs, "args must be a JSON object: "+err.Error())
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, boxMCPCallTimeout)
	defer cancel()
	start := time.Now()
	raw, err := tool.CallRaw(callCtx, args, r.callID+":"+strconv.Itoa(n))
	elapsed := time.Since(start)
	if err != nil {
		code := boxMCPCodeCallFailed
		switch {
		case errors.Is(err, mcpclient.ErrResultTooLarge):
			code = boxMCPCodeResultTooLarge
		case ctx.Err() != nil && errors.Is(err, context.Canceled):
			code = agentbox.HostCallCodeCanceled
		case errors.Is(err, context.DeadlineExceeded):
			code = boxMCPCodeTimeout
		}
		return r.fail(ctx, req.Tool, n, elapsed, 0, code, err.Error())
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return r.fail(ctx, req.Tool, n, elapsed, 0, boxMCPCodeCallFailed, "encode result: "+err.Error())
	}
	if len(out) > BoxMCPMaxResultBytes {
		return r.fail(ctx, req.Tool, n, elapsed, len(out), boxMCPCodeResultTooLarge,
			"result is "+byteCountString(int64(len(out)))+", over the "+byteCountString(BoxMCPMaxResultBytes)+" per-call limit; request less data")
	}
	r.mu.Lock()
	if r.totals.ResultBytes+int64(len(out)) > BoxMCPMaxTotalResultBytes {
		r.mu.Unlock()
		return r.fail(ctx, req.Tool, n, elapsed, len(out), boxMCPCodeResultBudget,
			"result would take this run past "+byteCountString(BoxMCPMaxTotalResultBytes)+" of MCP results")
	}
	r.totals.ResultBytes += int64(len(out))
	r.totals.Duration += elapsed
	r.entries = append(r.entries, boxMCPCall{N: n, Tool: req.Tool, OK: true, DurationMS: elapsed.Milliseconds(), ResultBytes: len(out)})
	r.mu.Unlock()
	r.opts.Logger.Debug(ctx, "agent box mcp call",
		slog.F("call", n), slog.F("tool", req.Tool), slog.F("ok", true),
		slog.F("duration_ms", elapsed.Milliseconds()), slog.F("result_bytes", len(out)))
	return agentbox.HostCallResult(out)
}

// reject records a call refused before reaching a server. It does not
// count toward the total.
func (r *boxMCPRun) reject(ctx context.Context, tool string, n int, code, msg string) []byte {
	r.mu.Lock()
	r.totals.Rejected++
	// A guest looping on the limit could otherwise grow the list without
	// bound.
	if r.totals.Rejected <= boxMCPMaxCallEntries {
		r.entries = append(r.entries, boxMCPCall{N: n, Tool: truncateForLog(tool), Code: code})
	}
	r.mu.Unlock()
	r.opts.Logger.Debug(ctx, "agent box mcp call rejected", slog.F("call", n), slog.F("tool", truncateForLog(tool)), slog.F("code", code))
	return agentbox.HostCallError(code, msg)
}

// fail records a failed call and returns its envelope.
func (r *boxMCPRun) fail(ctx context.Context, tool string, n int, elapsed time.Duration, resultBytes int, code, msg string) []byte {
	r.mu.Lock()
	r.totals.Failed++
	r.totals.Duration += elapsed
	r.entries = append(r.entries, boxMCPCall{N: n, Tool: truncateForLog(tool), Code: code, DurationMS: elapsed.Milliseconds(), ResultBytes: resultBytes})
	r.mu.Unlock()
	r.opts.Logger.Debug(ctx, "agent box mcp call",
		slog.F("call", n), slog.F("tool", truncateForLog(tool)), slog.F("ok", false), slog.F("code", code),
		slog.F("duration_ms", elapsed.Milliseconds()))
	return agentbox.HostCallError(code, msg)
}

// summary returns the run's calls, at most boxMCPMaxCallEntries of them.
// Failures and rejections are kept in preference to successes so a long
// run does not hide them; the kept entries are in call order.
func (r *boxMCPRun) summary() boxMCPSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.totals
	kept := make([]boxMCPCall, 0, min(len(r.entries), boxMCPMaxCallEntries))
	for _, e := range r.entries {
		if !e.OK && len(kept) < boxMCPMaxCallEntries {
			kept = append(kept, e)
		}
	}
	for _, e := range r.entries {
		if len(kept) >= boxMCPMaxCallEntries {
			break
		}
		if e.OK {
			kept = append(kept, e)
		}
	}
	slices.SortFunc(kept, func(a, b boxMCPCall) int { return a.N - b.N })
	s.Calls = kept
	return s
}

func truncateForLog(name string) string {
	if len(name) <= boxMCPLogNameLimit {
		return name
	}
	return strings.ToValidUTF8(name[:boxMCPLogNameLimit], "") + "..."
}
