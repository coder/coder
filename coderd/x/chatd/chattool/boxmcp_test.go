package chattool_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

// fakeRawCaller is an MCP tool whose CallRaw is scripted.
type fakeRawCaller struct {
	fantasy.AgentTool
	info  fantasy.ToolInfo
	calls atomic.Int64
	// lastArgs and lastID record the most recent call.
	lastArgs atomic.Value
	lastID   atomic.Value
	call     func(ctx context.Context, args map[string]any) (mcpclient.RawResult, error)
}

func (f *fakeRawCaller) Info() fantasy.ToolInfo { return f.info }

func (*fakeRawCaller) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse("model path"), nil
}

func (f *fakeRawCaller) CallRaw(ctx context.Context, args map[string]any, callID string) (mcpclient.RawResult, error) {
	f.calls.Add(1)
	f.lastArgs.Store(args)
	f.lastID.Store(callID)
	return f.call(ctx, args)
}

func textResult(text string) mcpclient.RawResult {
	return mcpclient.RawResult{Content: []map[string]any{{"type": "text", "text": text}}}
}

// plainTool is a built-in tool that must stay unreachable from the box.
type plainTool struct{ fantasy.AgentTool }

func (plainTool) Info() fantasy.ToolInfo { return fantasy.ToolInfo{Name: "read_file"} }
func (plainTool) Run(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse(""), nil
}

func newMCPHarness(t *testing.T, tools ...fantasy.AgentTool) *boxHarness {
	t.Helper()
	h := newBoxHarness(t)
	h.options.MCP = chattool.BoxMCPOptions{
		Tools: func() []fantasy.AgentTool { return tools },
		ServerName: func(tool fantasy.AgentTool) string {
			server, _, _ := strings.Cut(tool.Info().Name, "__")
			return server
		},
		Servers: []string{"gh"},
		Logger:  testutil.Logger(t),
	}
	return h
}

func runCode(t *testing.T, h *boxHarness, code string) map[string]any {
	t.Helper()
	input, err := json.Marshal(chattool.BoxRunArgs{Language: "javascript", Code: code})
	require.NoError(t, err)
	_, result := runBoxTool(t, chattool.BoxRun(h.options), string(input))
	require.NotNil(t, result)
	return result
}

func TestBoxMCP(t *testing.T) {
	t.Parallel()

	newPager := func() *fakeRawCaller {
		return &fakeRawCaller{
			info: fantasy.ToolInfo{Name: "gh__list", Description: "lists things", Parameters: map[string]any{"page": map[string]any{"type": "integer"}}, Required: []string{"page"}},
			call: func(_ context.Context, args map[string]any) (mcpclient.RawResult, error) {
				page, _ := args["page"].(float64)
				r := textResult("page " + strconv.Itoa(int(page)))
				r.StructuredContent = map[string]any{"page": page, "next": page < 2}
				return r, nil
			},
		}
	}
	newIsError := func() *fakeRawCaller {
		return &fakeRawCaller{
			info: fantasy.ToolInfo{Name: "gh__iserror"},
			call: func(context.Context, map[string]any) (mcpclient.RawResult, error) {
				r := textResult("bad input")
				r.IsError = true
				return r, nil
			},
		}
	}
	wrapped := &fakeRawCaller{
		info: fantasy.ToolInfo{
			Name: "gh__intent",
			Parameters: map[string]any{
				"model_intent": map[string]any{"type": "string"},
				"properties":   map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}, "required": []string{"q"}},
			},
			Required: []string{"model_intent", "properties"},
		},
		call: func(context.Context, map[string]any) (mcpclient.RawResult, error) { return textResult("ok"), nil },
	}
	failing := &fakeRawCaller{
		info: fantasy.ToolInfo{Name: "gh__fail"},
		call: func(context.Context, map[string]any) (mcpclient.RawResult, error) {
			return mcpclient.RawResult{}, xerrors.New("upstream down")
		},
	}
	t.Run("NoMCPToolsLeavesMountOut", func(t *testing.T) {
		t.Parallel()
		h := newMCPHarness(t, plainTool{})
		result := runCode(t, h, `
			try { mcp.call("read_file", {}); } catch (e) { console.log(e.code); }
			console.log(JSON.stringify(os.stat("/mcp")[1] !== 0));
		`)
		assert.Equal(t, "unavailable\ntrue\n", result["stdout"], result["stderr"])
		assert.NotContains(t, result, "mcp_calls")
		assert.NotContains(t, result, "mcp_calls_total")
	})

	t.Run("CallAndPage", func(t *testing.T) {
		t.Parallel()
		pager := newPager()
		h := newMCPHarness(t, plainTool{}, pager, failing)
		result := runCode(t, h, `
			let page = 0, texts = [];
			for (;;) {
				const r = mcp.call("gh__list", {page});
				texts.push(mcp.text(r));
				if (!r.structuredContent.next) break;
				page++;
			}
			console.log(texts.join("|"));
		`)
		assert.Equal(t, "page 0|page 1|page 2\n", result["stdout"], result["stderr"])
		assert.EqualValues(t, 3, result["mcp_calls_total"])
		assert.EqualValues(t, 0, result["mcp_calls_failed"])
		calls, ok := result["mcp_calls"].([]any)
		require.True(t, ok)
		require.Len(t, calls, 3)
		first := calls[0].(map[string]any)
		assert.EqualValues(t, 1, first["n"])
		assert.Equal(t, "gh__list", first["tool"])
		assert.Equal(t, true, first["ok"])
		assert.Greater(t, first["result_bytes"], float64(0))
		assert.Equal(t, "call-1:3", pager.lastID.Load())
	})

	t.Run("ToolsAndSchema", func(t *testing.T) {
		t.Parallel()
		h := newMCPHarness(t, wrapped, plainTool{}, newPager())
		result := runCode(t, h, `
			console.log(JSON.stringify(mcp.tools()));
			console.log(JSON.stringify(mcp.schema("gh__intent")));
			console.log(JSON.stringify(mcp.schema("gh__list")));
		`)
		assert.Equal(t,
			`[{"name":"gh__intent","server":"gh"},{"name":"gh__list","server":"gh","description":"lists things"}]`+"\n"+
				`{"properties":{"q":{"type":"string"}},"required":["q"],"type":"object"}`+"\n"+
				`{"properties":{"page":{"type":"integer"}},"required":["page"],"type":"object"}`+"\n",
			result["stdout"], result["stderr"])
		assert.NotContains(t, result, "mcp_calls_total", "listing and schema are not calls")
	})

	t.Run("ErrorsAndCodes", func(t *testing.T) {
		t.Parallel()
		isError := newIsError()
		h := newMCPHarness(t, newPager(), failing, isError)
		result := runCode(t, h, `
			const codes = [];
			const attempt = (f) => { try { f(); codes.push("ok"); } catch (e) { codes.push(e.code); } };
			attempt(() => mcp.call("nope", {}));
			attempt(() => mcp.call("gh__fail", {}));
			attempt(() => mcp.call("gh__list", [1, 2]));
			attempt(() => { const r = mcp.call("gh__iserror"); if (!r.isError) throw new Error("expected isError"); });
			attempt(() => mcp.schema("nope"));
			console.log(codes.join(","));
		`)
		assert.Equal(t, "unknown_tool,call_failed,invalid_args,ok,unknown_tool\n", result["stdout"], result["stderr"])
		assert.EqualValues(t, 4, result["mcp_calls_total"])
		assert.EqualValues(t, 3, result["mcp_calls_failed"])
		calls := result["mcp_calls"].([]any)
		require.Len(t, calls, 4)
		assert.Equal(t, "unknown_tool", calls[0].(map[string]any)["code"])
		assert.Equal(t, "call_failed", calls[1].(map[string]any)["code"])
		assert.Equal(t, "invalid_args", calls[2].(map[string]any)["code"])
		assert.Equal(t, true, calls[3].(map[string]any)["ok"])
		assert.Equal(t, map[string]any{}, isError.lastArgs.Load(), "missing args become an empty object")
	})

	t.Run("AmbiguousName", func(t *testing.T) {
		t.Parallel()
		pager := newPager()
		dup := &fakeRawCaller{info: fantasy.ToolInfo{Name: "gh__list"}, call: pager.call}
		h := newMCPHarness(t, pager, dup)
		result := runCode(t, h, `try { mcp.call("gh__list", {page: 0}); } catch (e) { console.log(e.code); }`)
		assert.Equal(t, "ambiguous_tool\n", result["stdout"], result["stderr"])
	})

	t.Run("CallLimitAndEntryCap", func(t *testing.T) {
		t.Parallel()
		h := newMCPHarness(t, newPager(), failing)
		result := runCode(t, h, `
			let limited = 0, failed = 0;
			for (let i = 0; i < `+strconv.Itoa(chattool.BoxMCPMaxCallsPerRun+5)+`; i++) {
				const name = i % 50 === 0 ? "gh__fail" : "gh__list";
				try { mcp.call(name, {page: 0}); } catch (e) { if (e.code === "call_limit") limited++; else if (e.code === "call_failed") failed++; }
			}
			console.log(limited, failed);
		`)
		assert.Equal(t, "5 4\n", result["stdout"], result["stderr"])
		assert.EqualValues(t, chattool.BoxMCPMaxCallsPerRun, result["mcp_calls_total"])
		assert.EqualValues(t, 4, result["mcp_calls_failed"])
		calls := result["mcp_calls"].([]any)
		require.Len(t, calls, 25)
		// All four failures are kept even though they are spread across
		// the run, and entries are in call order.
		var failures []float64
		prev := 0.0
		for _, c := range calls {
			entry := c.(map[string]any)
			assert.Greater(t, entry["n"], prev)
			prev = entry["n"].(float64)
			if ok, _ := entry["ok"].(bool); !ok {
				failures = append(failures, entry["n"].(float64))
			}
		}
		assert.Equal(t, []float64{1, 51, 101, 151}, failures)
	})

	t.Run("ResultTooLargeAndBudget", func(t *testing.T) {
		t.Parallel()
		big := &fakeRawCaller{
			info: fantasy.ToolInfo{Name: "gh__big"},
			call: func(_ context.Context, args map[string]any) (mcpclient.RawResult, error) {
				size, _ := args["size"].(float64)
				return textResult(strings.Repeat("x", int(size))), nil
			},
		}
		h := newMCPHarness(t, big)
		result := runCode(t, h, `
			const codes = [];
			try { mcp.call("gh__big", {size: `+strconv.Itoa(chattool.BoxMCPMaxResultBytes)+`}); codes.push("ok"); } catch (e) { codes.push(e.code); }
			// Eight results just under the per-call cap fit the run budget; the ninth does not.
			for (let i = 0; i < 9; i++) {
				try { mcp.call("gh__big", {size: `+strconv.Itoa(chattool.BoxMCPMaxResultBytes-100)+`}); codes.push("ok"); } catch (e) { codes.push(e.code); }
			}
			console.log(codes.join(","));
		`)
		assert.Equal(t, "result_too_large,ok,ok,ok,ok,ok,ok,ok,ok,result_budget\n", result["stdout"], result["stderr"])
	})

	t.Run("DescriptionMentionsMCPOnlyWhenPresent", func(t *testing.T) {
		t.Parallel()
		plain := newBoxHarness(t)
		assert.NotContains(t, chattool.BoxRun(plain.options).Info().Description, "mcp.call")
		assert.Contains(t, chattool.BoxRun(plain.options).Info().Description, "no workspace, network, or package access")
		h := newMCPHarness(t, newPager())
		desc := chattool.BoxRun(h.options).Info().Description
		assert.Contains(t, desc, "mcp.call(name, args)")
		assert.Contains(t, desc, "Servers: gh.")
		assert.Contains(t, desc, "no workspace, direct network, or package access")
	})
}
