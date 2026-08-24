package chatloop_test

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestGenerateAssistant_RecordsModelInvocationRuntime(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "hello"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "t"}) {
					return
				}
				clock.Advance(1500 * time.Millisecond)
				yield(fantasy.StreamPart{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonStop,
				})
			}, nil
		},
	}

	outcome, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model: model,
		Clock: clock,
	})
	require.NoError(t, err)
	require.Equal(t, 1500*time.Millisecond, outcome.Step.Runtime)
}

// The interrupt path bills the window OnModelStreamStart opens, so that
// hook must fire at the instant PersistedStep.Runtime starts measuring.
func TestGenerateAssistant_ModelStreamStartMatchesRuntimeWindow(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "hello"}) {
					return
				}
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "t"}) {
					return
				}
				clock.Advance(1500 * time.Millisecond)
				yield(fantasy.StreamPart{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonStop,
				})
			}, nil
		},
	}

	var startedAt []time.Time
	outcome, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model: model,
		Clock: clock,
		OnModelStreamStart: func() {
			startedAt = append(startedAt, clock.Now())
		},
	})
	require.NoError(t, err)
	require.Len(t, startedAt, 1)
	require.Equal(t, outcome.Step.Runtime, clock.Since(startedAt[0]))
}

func TestGenerateAssistant_ErroredStreamReturnsNoStep(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t"}) {
					return
				}
				clock.Advance(1500 * time.Millisecond)
				yield(fantasy.StreamPart{
					Type:  fantasy.StreamPartTypeError,
					Error: xerrors.New("stream blew up"),
				})
			}, nil
		},
	}

	outcome, err := chatloop.GenerateAssistant(context.Background(), chatloop.GenerateAssistantOptions{
		Model: model,
		Clock: clock,
	})
	require.Error(t, err)
	require.Zero(t, outcome.Step.Runtime)
	require.Empty(t, outcome.Step.Content)
}

func TestGenerateCompaction_RecordsRuntime(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	model := &chattest.FakeModel{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			clock.Advance(1500 * time.Millisecond)
			return &fantasy.Response{
				Content: []fantasy.Content{
					fantasy.TextContent{Text: "summary"},
				},
			}, nil
		},
	}

	var startedAt []time.Time
	result, err := chatloop.GenerateCompaction(context.Background(), chatloop.GenerateCompactionOptions{
		Model: model,
		Messages: []fantasy.Message{{
			Role:    fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
		}},
		ThresholdPercent: 70,
		ContextLimit:     100,
		StepUsage:        fantasy.Usage{InputTokens: 90},
		Clock:            clock,
		OnModelStreamStart: func() {
			startedAt = append(startedAt, clock.Now())
		},
	})
	require.NoError(t, err)
	require.Equal(t, "summary", result.SummaryReport)
	require.Equal(t, 1500*time.Millisecond, result.Runtime)
	require.Len(t, startedAt, 1)
	require.Equal(t, result.Runtime, clock.Since(startedAt[0]))
}

// executeToolBatch lets tests release trapped clock events in order, so
// goroutines cannot race clock advances.
func executeToolBatch(
	t *testing.T,
	clock *quartz.Mock,
	opts chatloop.ExecuteLocalToolsOptions,
) <-chan chatloop.PersistedStep {
	t.Helper()
	opts.Clock = clock
	resultCh := make(chan chatloop.PersistedStep, 1)
	go func() {
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), opts)
		assert.NoError(t, err)
		resultCh <- outcome
	}()
	return resultCh
}

func blockingTool(name string, release <-chan struct{}, response fantasy.ToolResponse) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		name,
		"test tool that completes when released",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			<-release
			return response, nil
		},
	)
}

type serialTool struct {
	fantasy.AgentTool
}

func (serialTool) SerialToolCalls() bool { return true }

func TestExecuteLocalTools_BatchWindowIsMaxNotSum(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	fastGo := make(chan struct{})
	slowGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("fast_tool", fastGo, fantasy.NewTextResponse("done")),
			blockingTool("slow_tool", slowGo, fantasy.NewTextErrorResponse("blew up")),
		},
		ActiveTools: []string{"fast_tool", "slow_tool"},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-fast", ToolName: "fast_tool", Input: "{}"},
			{ToolCallID: "call-slow", ToolName: "slow_tool", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(50 * time.Second)
	close(slowGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 60*time.Second, outcome.BatchRuntime)
}

func TestExecuteLocalTools_SimultaneousCompletionsBillOnce(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	release := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("read_tool", release, fantasy.NewTextResponse("done")),
		},
		ActiveTools: []string{"read_tool"},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-1", ToolName: "read_tool", Input: "{}"},
			{ToolCallID: "call-2", ToolName: "read_tool", Input: "{}"},
			{ToolCallID: "call-3", ToolName: "read_tool", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Second)
	close(release)
	for range 3 {
		trap.MustWait(ctx).MustRelease(ctx)
	}

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 10*time.Second, outcome.BatchRuntime)
}

func TestExecuteLocalTools_UnbilledToolNeverExtendsWindow(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	executeGo := make(chan struct{})
	waitGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("execute", executeGo, fantasy.NewTextResponse("done")),
			blockingTool("wait_agent", waitGo, fantasy.NewTextResponse("child report")),
		},
		ActiveTools:       []string{"execute", "wait_agent"},
		UnbilledToolNames: map[string]bool{"wait_agent": true},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-execute", ToolName: "execute", Input: "{}"},
			{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Second)
	close(executeGo)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(50 * time.Second)
	close(waitGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 10*time.Second, outcome.BatchRuntime)
}

func TestExecuteLocalTools_UnbilledOnlyBatchBillsNothing(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	waitGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("wait_agent", waitGo, fantasy.NewTextResponse("child report")),
		},
		ActiveTools:       []string{"wait_agent"},
		UnbilledToolNames: map[string]bool{"wait_agent": true},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(60 * time.Second)
	close(waitGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Zero(t, outcome.BatchRuntime)
}

func TestExecuteLocalTools_AliasNamesClassifyAsCalled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	executeGo := make(chan struct{})
	legacyGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("execute", executeGo, fantasy.NewTextResponse("done")),
			blockingTool("interrupt_agent", legacyGo, fantasy.NewTextResponse("stopped")),
		},
		ActiveTools:     []string{"execute", "interrupt_agent"},
		ToolNameAliases: map[string]string{"close_agent": "interrupt_agent"},
		UnbilledToolNames: map[string]bool{
			"interrupt_agent": true,
			"close_agent":     true,
		},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-execute", ToolName: "execute", Input: "{}"},
			{ToolCallID: "call-legacy", ToolName: "close_agent", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Second)
	close(executeGo)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(50 * time.Second)
	close(legacyGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 10*time.Second, outcome.BatchRuntime)
}

func TestExecuteLocalTools_DuplicateToolCallIDsKeepOccurrenceCompletions(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	slowGo := make(chan struct{})
	fastGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("slow_tool", slowGo, fantasy.NewTextResponse("done")),
			blockingTool("fast_tool", fastGo, fantasy.NewTextResponse("done")),
		},
		ActiveTools: []string{"slow_tool", "fast_tool"},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-dup", ToolName: "slow_tool", Input: "{}"},
			{ToolCallID: "call-dup", ToolName: "fast_tool", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(50 * time.Second)
	close(slowGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 60*time.Second, outcome.BatchRuntime)
}

// recordingToolBillingRecorder is a test ToolBillingRecorder that
// counts start/complete calls and can publish live completions.
type recordingToolBillingRecorder struct {
	starts      int
	completions int
	completeCh  chan recordedToolCompletion
}

type recordedToolCompletion struct {
	dispatchIndex int
	completedAt   time.Time
}

func (r *recordingToolBillingRecorder) RecordStart(int, time.Time) {
	r.starts++
}

func (r *recordingToolBillingRecorder) RecordComplete(dispatchIndex int, completedAt time.Time) {
	r.completions++
	if r.completeCh != nil {
		r.completeCh <- recordedToolCompletion{
			dispatchIndex: dispatchIndex,
			completedAt:   completedAt,
		}
	}
}

func TestExecuteLocalTools_BillingRecorderRecordsOnlyRuns(t *testing.T) {
	t.Parallel()

	t.Run("started call records a paired lifecycle", func(t *testing.T) {
		t.Parallel()

		recorder := &recordingToolBillingRecorder{}
		startedWhenToolRan := false
		tool := fantasy.NewAgentTool(
			"fast_tool",
			"test tool",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				startedWhenToolRan = recorder.starts > 0
				return fantasy.NewTextResponse("done"), nil
			},
		)
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), chatloop.ExecuteLocalToolsOptions{
			Clock:           quartz.NewMock(t),
			Tools:           []fantasy.AgentTool{tool},
			ActiveTools:     []string{"fast_tool"},
			BillingRecorder: recorder,
			ToolCalls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "fast_tool", Input: "{}"},
			},
		})
		require.NoError(t, err)
		require.Len(t, outcome.Content, 1)
		require.Equal(t, 1, recorder.starts)
		require.Equal(t, 1, recorder.completions)
		require.True(t, startedWhenToolRan, "RecordStart must run before the tool runs")
	})

	t.Run("canceled context records no lifecycle", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recorder := &recordingToolBillingRecorder{}
		_, err := chatloop.ExecuteLocalTools(ctx, chatloop.ExecuteLocalToolsOptions{
			Clock:           quartz.NewMock(t),
			BillingRecorder: recorder,
			ToolCalls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "fast_tool", Input: "{}"},
			},
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, recorder.starts)
		require.Zero(t, recorder.completions)
	})

	t.Run("exclusive violation records no lifecycle", func(t *testing.T) {
		t.Parallel()

		recorder := &recordingToolBillingRecorder{}
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), chatloop.ExecuteLocalToolsOptions{
			Clock:              quartz.NewMock(t),
			ExclusiveToolNames: map[string]bool{"exclusive_tool": true},
			BillingRecorder:    recorder,
			ToolCalls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "exclusive_tool", Input: "{}"},
				{ToolCallID: "call-2", ToolName: "fast_tool", Input: "{}"},
			},
		})
		require.NoError(t, err)
		require.Len(t, outcome.Content, 2, "the whole batch resolves to synthesized policy errors")
		require.Zero(t, recorder.starts)
		require.Zero(t, recorder.completions)
	})
}

func TestExecuteLocalTools_EmptyToolCallIDStillBillsWindow(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	idlessGo := make(chan struct{})
	fastGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("idless_tool", idlessGo, fantasy.NewTextResponse("done")),
			blockingTool("fast_tool", fastGo, fantasy.NewTextResponse("done")),
		},
		ActiveTools: []string{"idless_tool", "fast_tool"},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "", ToolName: "idless_tool", Input: "{}"},
			{ToolCallID: "call-fast", ToolName: "fast_tool", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(50 * time.Second)
	close(idlessGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 60*time.Second, outcome.BatchRuntime)
}

func TestExecuteLocalTools_BillingRecorderReportsLiveCompletions(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	recorder := &recordingToolBillingRecorder{
		completeCh: make(chan recordedToolCompletion, 2),
	}
	fastGo := make(chan struct{})
	slowGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("fast_tool", fastGo, fantasy.NewTextResponse("done")),
			blockingTool("slow_tool", slowGo, fantasy.NewTextResponse("done")),
		},
		ActiveTools:     []string{"fast_tool", "slow_tool"},
		BillingRecorder: recorder,
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-fast", ToolName: "fast_tool", Input: "{}"},
			{ToolCallID: "call-slow", ToolName: "slow_tool", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	fast := testutil.RequireReceive(ctx, t, recorder.completeCh)
	require.Equal(t, 0, fast.dispatchIndex)
	clock.Advance(50 * time.Second)
	close(slowGo)
	trap.MustWait(ctx).MustRelease(ctx)
	slow := testutil.RequireReceive(ctx, t, recorder.completeCh)
	require.Equal(t, 1, slow.dispatchIndex)
	require.Equal(t, 50*time.Second, slow.completedAt.Sub(fast.completedAt))

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, map[string]time.Time{
		"call-fast": fast.completedAt,
		"call-slow": slow.completedAt,
	}, outcome.ToolResultCreatedAt)
}

func TestExecuteLocalTools_SerialCallBillsFromItsOwnStart(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	waitGo := make(chan struct{})
	serialGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("wait_agent", waitGo, fantasy.NewTextResponse("child report")),
			serialTool{blockingTool("serial_tool", serialGo, fantasy.NewTextResponse("done"))},
		},
		ActiveTools:       []string{"wait_agent", "serial_tool"},
		UnbilledToolNames: map[string]bool{"wait_agent": true},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
			{ToolCallID: "call-serial", ToolName: "serial_tool", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(10 * time.Minute)
	close(waitGo)
	trap.MustWait(ctx).MustRelease(ctx)
	// Release the serial start timestamp.
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(2 * time.Second)
	close(serialGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 2*time.Second, outcome.BatchRuntime,
		"a serial call bills its own execution, not the unbilled wait that delayed its launch")
}

func TestExecuteLocalTools_SerialAfterBilledSiblingBillsUnion(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	execGo := make(chan struct{})
	waitGo := make(chan struct{})
	serialGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("execute", execGo, fantasy.NewTextResponse("done")),
			blockingTool("wait_agent", waitGo, fantasy.NewTextResponse("child report")),
			serialTool{blockingTool("serial_tool", serialGo, fantasy.NewTextResponse("done"))},
		},
		ActiveTools:       []string{"execute", "wait_agent", "serial_tool"},
		UnbilledToolNames: map[string]bool{"wait_agent": true},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-execute", ToolName: "execute", Input: "{}"},
			{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
			{ToolCallID: "call-serial", ToolName: "serial_tool", Input: "{}"},
		},
	})

	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(3 * time.Second)
	close(execGo)
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(7 * time.Second)
	close(waitGo)
	trap.MustWait(ctx).MustRelease(ctx)
	// Release the serial start timestamp.
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(2 * time.Second)
	close(serialGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 5*time.Second, outcome.BatchRuntime,
		"the 3s concurrent window and the 2s serial window bill; the 7s span where only wait_agent ran does not")
}

func TestBilledIntervalsDuration(t *testing.T) {
	t.Parallel()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	at := base.Add
	for _, tc := range []struct {
		name      string
		intervals []chatloop.BilledInterval
		want      time.Duration
	}{
		{name: "empty", want: 0},
		{
			name: "overlapping intervals bill once",
			intervals: []chatloop.BilledInterval{
				{Start: at(0), End: at(10 * time.Second)},
				{Start: at(0), End: at(4 * time.Second)},
			},
			want: 10 * time.Second,
		},
		{
			name: "gap between intervals is not billed",
			intervals: []chatloop.BilledInterval{
				{Start: at(0), End: at(3 * time.Second)},
				{Start: at(10 * time.Second), End: at(12 * time.Second)},
			},
			want: 5 * time.Second,
		},
		{
			name: "unsorted contained interval adds nothing",
			intervals: []chatloop.BilledInterval{
				{Start: at(2 * time.Second), End: at(4 * time.Second)},
				{Start: at(0), End: at(10 * time.Second)},
			},
			want: 10 * time.Second,
		},
		{
			name: "touching intervals merge without a gap",
			intervals: []chatloop.BilledInterval{
				{Start: at(0), End: at(3 * time.Second)},
				{Start: at(3 * time.Second), End: at(5 * time.Second)},
			},
			want: 5 * time.Second,
		},
		{
			name: "inverted interval is ignored",
			intervals: []chatloop.BilledInterval{
				{Start: at(5 * time.Second), End: at(0)},
				{Start: at(0), End: at(2 * time.Second)},
			},
			want: 2 * time.Second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, chatloop.BilledIntervalsDuration(tc.intervals))
		})
	}
}
