package chattool

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
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
	BoxMCPMaxCallsPerRun     = 200
	BoxMCPMaxResultBytes     = 8 << 20
	BoxMCPMaxTotalResultByte = 64 << 20
	// boxMCPCallTimeout bounds one call; the run deadline usually ends it
	// sooner.
	boxMCPCallTimeout = 60 * time.Second
	// boxMCPMaxCallEntries caps the mcp_calls list in a box_run result.
	boxMCPMaxCallEntries = 25
	// boxMCPLogNameLimit truncates guest-supplied tool names in logs.
	boxMCPLogNameLimit = 128
)

// Envelope codes returned to the guest by the dispatcher.
const (
	BoxMCPCodeUnknownTool    = "unknown_tool"
	BoxMCPCodeAmbiguousTool  = "ambiguous_tool"
	BoxMCPCodeInvalidArgs    = "invalid_args"
	BoxMCPCodeInvalidRequest = "invalid_request"
	BoxMCPCodeCallLimit      = "call_limit"
	BoxMCPCodeResultBudget   = "result_budget"
	BoxMCPCodeResultTooLarge = "result_too_large"
	BoxMCPCodeTimeout        = "timeout"
	BoxMCPCodeCallFailed     = "call_failed"
)

// BoxMCPOptions configures guest MCP calls from a box.
type BoxMCPOptions struct {
	// Tools returns the turn's executable tools after the plan and Explore
	// filters. Only tools implementing mcpclient.RawCaller are reachable.
	// It is resolved on the first guest call of each run.
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
		"mcp.call throws an MCPError whose message starts with a code ([unknown_tool], [call_limit], [result_too_large], [timeout], [call_failed], ...) on an unknown tool, a limit, or a transport failure; a result with isError true is returned, not thrown." + servers +
		" Calls run one at a time and their waiting time counts against the run's time limit, so write intermediate results to /box between runs and keep only what you need from each result. " +
		"Limits: " + strconv.Itoa(BoxMCPMaxCallsPerRun) + " calls per run, " + byteCountString(BoxMCPMaxResultBytes) + " per result, " + byteCountString(BoxMCPMaxTotalResultByte) + " of results per run. " +
		"The result lists each call in mcp_calls (n, tool, ok, code, duration_ms, result_bytes as the redacted JSON size; at most " + strconv.Itoa(boxMCPMaxCallEntries) + " entries, failures kept first) with mcp_calls_total and mcp_calls_failed; limits_hit lists \"mcp_request\" when a request exceeded " + byteCountString(agentbox.MaxHostCallRequestBytes) + "."
}

// BoxMCPTool is one entry of mcp.tools().
type BoxMCPTool struct {
	Name        string `json:"name"`
	Server      string `json:"server,omitempty"`
	Description string `json:"description,omitempty"`
}

// BoxMCPCall is one entry of a box_run result's mcp_calls.
type BoxMCPCall struct {
	// N is the call's position in the run, starting at 1.
	N           int    `json:"n"`
	Tool        string `json:"tool"`
	OK          bool   `json:"ok"`
	Code        string `json:"code,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	ResultBytes int    `json:"result_bytes"`
}

// BoxMCPSummary is what a run's guest calls add to the box_run result.
type BoxMCPSummary struct {
	Calls  []BoxMCPCall
	Total  int
	Failed int
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

	once   sync.Once
	tools  []mcpclient.RawCaller
	byName map[string][]mcpclient.RawCaller

	mu      sync.Mutex
	summary BoxMCPSummary
	entries []BoxMCPCall
}

// newBoxMCPRun returns a dispatcher when the turn has at least one MCP
// tool, otherwise nil so the run leaves /mcp unmounted.
func newBoxMCPRun(opts BoxMCPOptions, callID string) *boxMCPRun {
	if opts.Tools == nil {
		return nil
	}
	if !slices.ContainsFunc(opts.Tools(), func(tool fantasy.AgentTool) bool {
		_, ok := tool.(mcpclient.RawCaller)
		return ok
	}) {
		return nil
	}
	return &boxMCPRun{opts: opts, callID: callID}
}

func (r *boxMCPRun) load() {
	r.once.Do(func() {
		r.byName = make(map[string][]mcpclient.RawCaller)
		for _, tool := range r.opts.Tools() {
			caller, ok := tool.(mcpclient.RawCaller)
			if !ok {
				continue
			}
			r.tools = append(r.tools, caller)
			name := tool.Info().Name
			r.byName[name] = append(r.byName[name], caller)
		}
	})
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
		return agentbox.HostCallError(BoxMCPCodeInvalidRequest, "malformed request: "+err.Error()), nil
	}
	r.load()
	switch req.Op {
	case "tools":
		return r.listTools()
	case "schema":
		return r.schema(req.Tool)
	case "call":
		return r.call(ctx, req), nil
	default:
		return agentbox.HostCallError(BoxMCPCodeInvalidRequest, "unknown op "+strconv.Quote(req.Op)), nil
	}
}

func (r *boxMCPRun) listTools() ([]byte, error) {
	list := make([]BoxMCPTool, 0, len(r.tools))
	for _, tool := range r.tools {
		info := tool.Info()
		list = append(list, BoxMCPTool{Name: info.Name, Server: r.serverName(tool), Description: info.Description})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	out, err := json.Marshal(list)
	if err != nil {
		return nil, xerrors.Errorf("encode tools: %w", err)
	}
	return agentbox.HostCallResult(out), nil
}

func (r *boxMCPRun) lookup(name string) (mcpclient.RawCaller, []byte) {
	matches := r.byName[name]
	switch len(matches) {
	case 0:
		return nil, agentbox.HostCallError(BoxMCPCodeUnknownTool, "unknown tool "+strconv.Quote(truncateForLog(name))+"; mcp.tools() lists the tools available in this run")
	case 1:
		return matches[0], nil
	default:
		return nil, agentbox.HostCallError(BoxMCPCodeAmbiguousTool, "tool name "+strconv.Quote(name)+" belongs to more than one server in this turn")
	}
}

func (r *boxMCPRun) schema(name string) ([]byte, error) {
	tool, errEnvelope := r.lookup(name)
	if errEnvelope != nil {
		return errEnvelope, nil
	}
	info := tool.Info()
	params, required := info.Parameters, info.Required
	// Org tools with model_intent wrap their real schema under
	// "properties"; unwrap so the guest sees what the server expects.
	if slices.Equal(required, []string{"model_intent", "properties"}) {
		if inner, ok := params["properties"].(map[string]any); ok {
			innerProps, _ := inner["properties"].(map[string]any)
			params = innerProps
			required = nil
			if req, ok := inner["required"].([]string); ok {
				required = req
			}
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

func (r *boxMCPRun) call(ctx context.Context, req boxMCPRequest) []byte {
	r.mu.Lock()
	if r.summary.Total >= BoxMCPMaxCallsPerRun {
		r.mu.Unlock()
		return agentbox.HostCallError(BoxMCPCodeCallLimit, "this run already made "+strconv.Itoa(BoxMCPMaxCallsPerRun)+" MCP calls")
	}
	if r.summary.ResultBytes >= BoxMCPMaxTotalResultByte {
		r.mu.Unlock()
		return agentbox.HostCallError(BoxMCPCodeResultBudget, "this run already received "+byteCountString(BoxMCPMaxTotalResultByte)+" of MCP results")
	}
	r.summary.Total++
	n := r.summary.Total
	r.mu.Unlock()

	var args map[string]any
	if len(req.Args) > 0 && string(req.Args) != "null" {
		if err := json.Unmarshal(req.Args, &args); err != nil {
			return r.record(ctx, req.Tool, n, 0, 0, BoxMCPCodeInvalidArgs, "args must be a JSON object: "+err.Error())
		}
	}
	tool, errEnvelope := r.lookup(req.Tool)
	if errEnvelope != nil {
		return r.recordEnvelope(ctx, req.Tool, n, 0, 0, errEnvelope)
	}

	callCtx, cancel := context.WithTimeout(ctx, boxMCPCallTimeout)
	defer cancel()
	start := time.Now()
	raw, err := tool.CallRaw(callCtx, args, r.callID+":"+strconv.Itoa(n))
	elapsed := time.Since(start)
	if err != nil {
		code := BoxMCPCodeCallFailed
		switch {
		case errors.Is(err, mcpclient.ErrResultTooLarge):
			code = BoxMCPCodeResultTooLarge
		case errors.Is(err, context.DeadlineExceeded):
			code = BoxMCPCodeTimeout
		}
		return r.record(ctx, req.Tool, n, elapsed, 0, code, err.Error())
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return r.record(ctx, req.Tool, n, elapsed, 0, BoxMCPCodeCallFailed, "encode result: "+err.Error())
	}
	if len(out) > BoxMCPMaxResultBytes {
		return r.record(ctx, req.Tool, n, elapsed, len(out), BoxMCPCodeResultTooLarge,
			"result is "+byteCountString(int64(len(out)))+", over the "+byteCountString(BoxMCPMaxResultBytes)+" per-call limit; request less data")
	}
	r.mu.Lock()
	if r.summary.ResultBytes+int64(len(out)) > BoxMCPMaxTotalResultByte {
		r.mu.Unlock()
		return r.record(ctx, req.Tool, n, elapsed, len(out), BoxMCPCodeResultBudget,
			"result would take this run past "+byteCountString(BoxMCPMaxTotalResultByte)+" of MCP results")
	}
	r.summary.ResultBytes += int64(len(out))
	r.summary.Duration += elapsed
	r.entries = append(r.entries, BoxMCPCall{N: n, Tool: req.Tool, OK: true, DurationMS: elapsed.Milliseconds(), ResultBytes: len(out)})
	r.mu.Unlock()
	r.opts.Logger.Debug(ctx, "agent box mcp call",
		slog.F("call", n), slog.F("tool", req.Tool), slog.F("ok", true),
		slog.F("duration_ms", elapsed.Milliseconds()), slog.F("result_bytes", len(out)))
	return agentbox.HostCallResult(out)
}

func (r *boxMCPRun) record(ctx context.Context, tool string, n int, elapsed time.Duration, resultBytes int, code, msg string) []byte {
	return r.recordEnvelope(ctx, tool, n, elapsed, resultBytes, agentbox.HostCallError(code, msg))
}

// recordEnvelope records a failed call and returns its envelope.
func (r *boxMCPRun) recordEnvelope(ctx context.Context, tool string, n int, elapsed time.Duration, resultBytes int, envelope []byte) []byte {
	var env agentbox.HostCallEnvelope
	_ = json.Unmarshal(envelope, &env)
	r.mu.Lock()
	r.summary.Failed++
	r.summary.Duration += elapsed
	r.entries = append(r.entries, BoxMCPCall{N: n, Tool: truncateForLog(tool), Code: env.Code, DurationMS: elapsed.Milliseconds(), ResultBytes: resultBytes})
	r.mu.Unlock()
	r.opts.Logger.Debug(ctx, "agent box mcp call",
		slog.F("call", n), slog.F("tool", truncateForLog(tool)), slog.F("ok", false), slog.F("code", env.Code),
		slog.F("duration_ms", elapsed.Milliseconds()))
	return envelope
}

// Summary returns the run's calls, at most boxMCPMaxCallEntries of them.
// Failures are kept in preference to successes so a long run does not
// hide them; the kept entries are in call order.
func (r *boxMCPRun) Summary() BoxMCPSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.summary
	kept := make([]BoxMCPCall, 0, min(len(r.entries), boxMCPMaxCallEntries))
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
	sort.Slice(kept, func(i, j int) bool { return kept[i].N < kept[j].N })
	s.Calls = kept
	return s
}

func truncateForLog(name string) string {
	if len(name) <= boxMCPLogNameLimit {
		return name
	}
	return strings.ToValidUTF8(name[:boxMCPLogNameLimit], "") + "..."
}
