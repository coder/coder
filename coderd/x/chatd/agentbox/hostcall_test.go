package agentbox_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// echoHostCall answers every request with {"echo": <request>, "n": <call number>}.
func echoHostCall(calls *atomic.Int64) agentbox.HostCallFunc {
	return func(_ context.Context, request []byte) ([]byte, error) {
		n := calls.Add(1)
		var req json.RawMessage
		if err := json.Unmarshal(request, &req); err != nil {
			return nil, xerrors.Errorf("bad request: %w", err)
		}
		out, _ := json.Marshal(map[string]any{"echo": req, "n": n})
		return agentbox.HostCallResult(out), nil
	}
}

func runHostCall(t *testing.T, box *agentbox.Box, call agentbox.HostCallFunc, code string) agentbox.RunResult {
	t.Helper()
	result, err := box.Run(t.Context(), agentbox.RunRequest{
		Language: agentbox.LanguageJavaScript,
		Code:     code,
		HostCall: call,
	})
	require.NoError(t, err)
	return result
}

func TestHostCall(t *testing.T) {
	t.Parallel()
	engine := newEngine(t, agentbox.Options{})

	t.Run("RoundTrip", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		var calls atomic.Int64
		result := runHostCall(t, box, echoHostCall(&calls), `
			const r = mcp.call("srv__tool", {page: 2});
			console.log(JSON.stringify(r));
			const t = mcp.tools();
			console.log(JSON.stringify(t.echo));
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, `{"echo":{"op":"call","tool":"srv__tool","args":{"page":2}},"n":1}`+"\n"+
			`{"op":"tools"}`+"\n", result.Stdout)
		assert.EqualValues(t, 2, calls.Load())
		assert.False(t, result.HostCallRequestTooLarge)
	})

	t.Run("SequentialCalls", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		var calls atomic.Int64
		result := runHostCall(t, box, echoHostCall(&calls), `
			let last = 0;
			for (let i = 0; i < 50; i++) { last = mcp.call("t", {i}).n; }
			console.log(last);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "50\n", result.Stdout)
		assert.EqualValues(t, 50, calls.Load())
	})

	t.Run("LargeResponse", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		const size = 8 << 20
		call := func(context.Context, []byte) ([]byte, error) {
			out, _ := json.Marshal(strings.Repeat("x", size))
			return agentbox.HostCallResult(out), nil
		}
		result := runHostCall(t, box, call, `
			const r = mcp.call("t", {});
			console.log(r.length, r[r.length - 1]);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "8388608 x\n", result.Stdout)
	})

	t.Run("RequestTooLarge", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		var calls atomic.Int64
		result := runHostCall(t, box, echoHostCall(&calls), `
			try {
				mcp.call("t", {blob: "y".repeat(`+strconv.Itoa(agentbox.MaxHostCallRequestBytes)+`)});
				console.log("no error");
			} catch (e) {
				console.log(e.name, e.code, e.message);
			}
			console.log(mcp.call("t", {}).n);
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "MCPError request_too_large [request_too_large] request exceeds the host call request limit\n1\n", result.Stdout)
		assert.True(t, result.HostCallRequestTooLarge)
		assert.EqualValues(t, 1, calls.Load(), "the oversized request must not reach the host")
	})

	t.Run("HostError", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		call := func(context.Context, []byte) ([]byte, error) {
			return nil, xerrors.New("boom")
		}
		result := runHostCall(t, box, call, `
			try { mcp.call("t", {}); } catch (e) { console.log(e.code, e.message); }
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "internal [internal] boom\n", result.Stdout)
	})

	t.Run("ErrorEnvelope", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		call := func(context.Context, []byte) ([]byte, error) {
			return agentbox.HostCallError("unknown_tool", "no such tool"), nil
		}
		result := runHostCall(t, box, call, `
			try { mcp.call("t", {}); } catch (e) { console.log(e.name, e.code, e.message); }
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "MCPError unknown_tool [unknown_tool] no such tool\n", result.Stdout)
	})

	t.Run("UncaughtShowsCode", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		call := func(context.Context, []byte) ([]byte, error) {
			return agentbox.HostCallError("call_limit", "too many calls"), nil
		}
		result := runHostCall(t, box, call, `mcp.call("t", {});`)
		require.NotEqual(t, 0, result.ExitCode)
		assert.Contains(t, result.Stderr, "MCPError: [call_limit] too many calls")
	})

	t.Run("Unavailable", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		result := runJS(t, box, `
			try { mcp.call("t", {}); } catch (e) { console.log(e.name, e.code); }
			console.log(typeof mcp.text, mcp.text({content: [{type: "text", text: "a"}, {type: "image"}, {type: "text", text: "b"}]}));
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, "MCPError unavailable\nfunction a\nb\n", result.Stdout)
	})

	t.Run("ReadWithoutWrite", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		var got atomic.Value
		call := func(_ context.Context, request []byte) ([]byte, error) {
			got.Store(string(request))
			return agentbox.HostCallResult(json.RawMessage(`1`)), nil
		}
		result := runHostCall(t, box, call, `
			const f = std.open("/mcp/call", "r");
			console.log(f.readAsString());
			f.close();
		`)
		require.Equal(t, 0, result.ExitCode, result.Stderr)
		assert.Equal(t, `{"ok":true,"result":1}`+"\n", result.Stdout)
		assert.Equal(t, "", got.Load())
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().AfterFunc("agentbox", "run-timeout")
		defer trap.Close()
		engine := newEngine(t, agentbox.Options{Clock: mClock, Limits: agentbox.Limits{RunTimeout: time.Minute}})
		box := newBox(t, engine)

		started := make(chan struct{})
		sawDone := make(chan struct{})
		call := func(ctx context.Context, _ []byte) ([]byte, error) {
			close(started)
			<-ctx.Done()
			close(sawDone)
			return nil, ctx.Err()
		}
		done := make(chan agentbox.RunResult, 1)
		go func() {
			result, err := box.Run(context.Background(), agentbox.RunRequest{
				Language: agentbox.LanguageJavaScript,
				Code:     `mcp.call("t", {}); for (;;) {}`,
				HostCall: call,
			})
			assert.NoError(t, err)
			done <- result
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		trap.MustWait(ctx).MustRelease(ctx)
		testutil.TryReceive(ctx, t, started)
		mClock.Advance(time.Minute).MustWait(ctx)
		result := testutil.RequireReceive(ctx, t, done)
		testutil.TryReceive(ctx, t, sawDone)
		assert.True(t, result.TimedOut)
		assert.False(t, result.Canceled)
		assert.Equal(t, agentbox.ExitCodeInterrupted, result.ExitCode)
	})

	t.Run("CloseMidCall", func(t *testing.T) {
		t.Parallel()
		box := newBox(t, engine)
		started := make(chan struct{})
		// The callee ignores ctx so the run must stop without it.
		release := make(chan struct{})
		call := func(context.Context, []byte) ([]byte, error) {
			close(started)
			<-release
			return agentbox.HostCallResult(json.RawMessage(`1`)), nil
		}
		done := make(chan agentbox.RunResult, 1)
		go func() {
			result, err := box.Run(context.Background(), agentbox.RunRequest{
				Language: agentbox.LanguageJavaScript,
				Code:     `mcp.call("t", {}); for (;;) {}`,
				HostCall: call,
			})
			assert.NoError(t, err)
			done <- result
		}()
		ctx := testutil.Context(t, testutil.WaitLong)
		testutil.TryReceive(ctx, t, started)
		require.NoError(t, box.Close())
		result := testutil.RequireReceive(ctx, t, done)
		close(release)
		assert.True(t, result.Canceled)
		assert.False(t, result.TimedOut)
		assert.Equal(t, agentbox.ExitCodeInterrupted, result.ExitCode)
	})
}
