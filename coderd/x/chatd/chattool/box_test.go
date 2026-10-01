package chattool_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

type boxHarness struct {
	engine  *agentbox.Engine
	box     *agentbox.Box
	reset   bool
	options chattool.BoxOptions
}

func newBoxHarness(t *testing.T) *boxHarness {
	t.Helper()
	engine, err := agentbox.NewEngine(t.Context(), agentbox.Options{
		Logger:  testutil.Logger(t),
		RootDir: t.TempDir(),
		Limits:  agentbox.Limits{OutputBytes: 64 << 10},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	h := &boxHarness{engine: engine}
	h.options = chattool.BoxOptions{
		GetBox: func(context.Context) (*agentbox.Box, bool, error) {
			if h.box == nil {
				box, err := engine.NewBox()
				if err != nil {
					return nil, false, err
				}
				t.Cleanup(func() { _ = box.Close() })
				h.box = box
			}
			return h.box, h.reset, nil
		},
		Languages: engine.Languages(),
		Limits:    engine.Limits(),
		StoreFile: func(_ context.Context, name string, _ string, data []byte) (chattool.AttachmentMetadata, error) {
			return chattool.AttachmentMetadata{FileID: uuid.New(), MediaType: "text/plain", Name: name + ":" + string(data)}, nil
		},
	}
	return h
}

func runBoxTool(t *testing.T, tool fantasy.AgentTool, input string) (fantasy.ToolResponse, map[string]any) {
	t.Helper()
	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "call-1", Name: tool.Info().Name, Input: input})
	require.NoError(t, err)
	if resp.IsError {
		return resp, nil
	}
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result), resp.Content)
	return resp, result
}

func TestBoxTools(t *testing.T) {
	t.Parallel()

	t.Run("Schema", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)
		for _, tool := range []fantasy.AgentTool{
			chattool.BoxRun(h.options),
			chattool.BoxWriteFile(h.options),
			chattool.BoxReadFile(h.options),
			chattool.BoxAttachFile(h.options),
		} {
			info := tool.Info()
			assert.Contains(t, chattool.BoxToolNames(), info.Name)
			serial, ok := tool.(interface{ SerialToolCalls() bool })
			require.True(t, ok, info.Name)
			assert.True(t, serial.SerialToolCalls(), info.Name)
		}
		run := chattool.BoxRun(h.options).Info()
		assert.ElementsMatch(t, []string{"language", "code"}, run.Required)
		assert.Contains(t, run.Description, "javascript")
		assert.Contains(t, run.Description, "1m0s per run")
		assert.Contains(t, run.Description, "256 MiB memory")
		assert.ElementsMatch(t, []string{"path", "content"}, chattool.BoxWriteFile(h.options).Info().Required)
		assert.ElementsMatch(t, []string{"path"}, chattool.BoxReadFile(h.options).Info().Required)
		assert.ElementsMatch(t, []string{"path"}, chattool.BoxAttachFile(h.options).Info().Required)
	})

	t.Run("RunAndFiles", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)

		_, result := runBoxTool(t, chattool.BoxWriteFile(h.options), `{"path":"/box/in.txt","content":"a\nb\n"}`)
		assert.Equal(t, "/box/in.txt", result["path"])
		assert.EqualValues(t, 4, result["bytes"])
		assert.Equal(t, h.box.ID(), result["box_id"])
		assert.NotContains(t, result, "box_reset")

		_, result = runBoxTool(t, chattool.BoxRun(h.options), `{"language":"javascript","code":"const s = std.loadFile('/box/in.txt'); const f = std.open('/box/out.txt', 'w'); f.puts(s.toUpperCase()); f.close(); console.log('done'); std.err.puts('warn'); std.exit(2);"}`)
		assert.EqualValues(t, 2, result["exit_code"])
		assert.Equal(t, "done\n", result["stdout"])
		assert.Equal(t, "warn", result["stderr"])
		assert.Equal(t, false, result["stdout_truncated"])
		assert.Equal(t, false, result["timed_out"])
		assert.Equal(t, false, result["canceled"])
		assert.Contains(t, result, "duration_ms")
		assert.Equal(t, h.box.ID(), result["box_id"])

		_, result = runBoxTool(t, chattool.BoxReadFile(h.options), `{"path":"out.txt"}`)
		assert.Equal(t, "1\tA\n2\tB\n3\t", result["content"])
		assert.EqualValues(t, 3, result["total_lines"])
		assert.EqualValues(t, 3, result["lines_read"])
		assert.EqualValues(t, 4, result["file_size"])

		_, result = runBoxTool(t, chattool.BoxReadFile(h.options), `{"path":"out.txt","offset":2,"limit":1}`)
		assert.Equal(t, "2\tB", result["content"])
	})

	t.Run("OutputTruncation", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)
		_, result := runBoxTool(t, chattool.BoxRun(h.options), `{"language":"javascript","code":"console.log('x'.repeat(40000));"}`)
		assert.EqualValues(t, 0, result["exit_code"])
		assert.Equal(t, true, result["stdout_truncated"])
		assert.Len(t, result["stdout"], 32<<10)
		assert.Equal(t, false, result["stderr_truncated"])
	})

	t.Run("ResultBudget", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)
		const budget = 16 << 10
		h.options.ResultBudgetBytes = budget
		// Both characters expand to six bytes when JSON-encoded.
		code := `std.out.puts("<".repeat(32 << 10)); std.err.puts("\x01".repeat(32 << 10));`
		input, err := json.Marshal(map[string]string{"language": "javascript", "code": code})
		require.NoError(t, err)
		resp, result := runBoxTool(t, chattool.BoxRun(h.options), string(input))
		assert.LessOrEqual(t, len(resp.Content), budget)
		assert.Equal(t, true, result["stdout_truncated"])
		assert.Equal(t, true, result["stderr_truncated"])
		assert.NotEmpty(t, result["stdout"])
		assert.NotEmpty(t, result["stderr"])
		assert.Equal(t, h.box.ID(), result["box_id"])

		// Output that fits is returned whole.
		_, result = runBoxTool(t, chattool.BoxRun(h.options), `{"language":"javascript","code":"console.log('<ok>')"}`)
		assert.Equal(t, "<ok>\n", result["stdout"])
		assert.Equal(t, false, result["stdout_truncated"])
	})

	t.Run("ResetOnError", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)
		h.reset = true
		_, result := runBoxTool(t, chattool.BoxReadFile(h.options), `{"path":"lost.txt"}`)
		assert.Contains(t, result["error"], "open")
		assert.Equal(t, true, result["box_reset"])
		assert.Equal(t, h.box.ID(), result["box_id"])
	})

	t.Run("Reset", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)
		h.reset = true
		_, result := runBoxTool(t, chattool.BoxRun(h.options), `{"language":"javascript","code":"console.log(1)"}`)
		assert.Equal(t, true, result["box_reset"])
		assert.Equal(t, h.box.ID(), result["box_id"])
	})

	t.Run("Errors", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)

		resp, _ := runBoxTool(t, chattool.BoxRun(h.options), `{"language":"","code":"1"}`)
		assert.True(t, resp.IsError)
		resp, _ = runBoxTool(t, chattool.BoxRun(h.options), `{"language":"javascript","code":""}`)
		assert.True(t, resp.IsError)

		_, result := runBoxTool(t, chattool.BoxRun(h.options), `{"language":"cobol","code":"1"}`)
		assert.Contains(t, result["error"], "unknown language")
		assert.Contains(t, result, "hint")
		assert.NotContains(t, result, "retryable")

		_, result = runBoxTool(t, chattool.BoxReadFile(h.options), `{"path":"/etc/passwd"}`)
		assert.Contains(t, result["error"], "must be under /box")
		_, result = runBoxTool(t, chattool.BoxWriteFile(h.options), `{"path":"../x","content":""}`)
		assert.Contains(t, result["error"], "..")
		_, result = runBoxTool(t, chattool.BoxReadFile(h.options), `{"path":"missing"}`)
		assert.Contains(t, result["error"], "open")
	})

	t.Run("CapacityErrors", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)
		h.options.GetBox = func(context.Context) (*agentbox.Box, bool, error) {
			return nil, false, agentbox.ErrTooManyBoxes
		}
		_, result := runBoxTool(t, chattool.BoxRun(h.options), `{"language":"javascript","code":"1"}`)
		assert.Equal(t, true, result["retryable"])
		assert.Contains(t, result["error"], "too many")

		h.options.GetBox = func(context.Context) (*agentbox.Box, bool, error) {
			return nil, false, agentbox.ErrBusy
		}
		_, result = runBoxTool(t, chattool.BoxWriteFile(h.options), `{"path":"a","content":"b"}`)
		assert.Equal(t, true, result["retryable"])

		_, result = runBoxTool(t, chattool.BoxRun(chattool.BoxOptions{}), `{"language":"javascript","code":"1"}`)
		assert.Contains(t, result["error"], "not configured")
	})

	t.Run("AttachFile", func(t *testing.T) {
		t.Parallel()
		h := newBoxHarness(t)
		runBoxTool(t, chattool.BoxWriteFile(h.options), `{"path":"/box/out/report.txt","content":"hello"}`)

		resp, result := runBoxTool(t, chattool.BoxAttachFile(h.options), `{"path":"/box/out/report.txt"}`)
		assert.Equal(t, true, result["ok"])
		assert.Equal(t, "report.txt:hello", result["name"])
		assert.EqualValues(t, 5, result["size"])
		assert.Equal(t, h.box.ID(), result["box_id"])
		attachments, err := chattool.AttachmentsFromMetadata(resp.Metadata)
		require.NoError(t, err)
		require.Len(t, attachments, 1)
		assert.Equal(t, result["file_id"], attachments[0].FileID.String())
		assert.Equal(t, "text/plain", attachments[0].MediaType)

		_, result = runBoxTool(t, chattool.BoxAttachFile(h.options), `{"path":"out/report.txt","name":"custom.txt"}`)
		assert.Equal(t, "custom.txt:hello", result["name"])

		resp, _ = runBoxTool(t, chattool.BoxAttachFile(h.options), `{"path":"missing.txt"}`)
		var errResult map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &errResult))
		assert.Contains(t, errResult["error"], "open")

		h.options.StoreFile = nil
		resp, _ = runBoxTool(t, chattool.BoxAttachFile(h.options), `{"path":"out/report.txt"}`)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "not configured")
	})
}
