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

// executeToolBatch runs ExecuteLocalTools in a goroutine. Callers trap the
// mock clock's Now: the first trapped call is the batch start and each
// subsequent one is a tool completion, released one tool at a time so
// parallel tool goroutines never race the clock advances.
func executeToolBatch(
	t *testing.T,
	clock *quartz.Mock,
	opts chatloop.ExecuteLocalToolsOptions,
) <-chan chatloop.ToolExecutionOutcome {
	t.Helper()
	opts.Clock = clock
	resultCh := make(chan chatloop.ToolExecutionOutcome, 1)
	go func() {
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), opts)
		assert.NoError(t, err)
		resultCh <- outcome
	}()
	return resultCh
}

// blockingTool returns a tool that parks until release is closed, so the
// test controls exactly when its completion timestamp is recorded.
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

// Parallel billed tools bill one shared window ending at the slowest
// tool's completion, never the sum of their durations. The slower tool
// returns an error result: errored tools bill their wall clock too.
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

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	// The fast tool completes 10 seconds in.
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	// The slow tool errors out at 60 seconds.
	clock.Advance(50 * time.Second)
	close(slowGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 60*time.Second, outcome.BatchRuntime)
	require.Equal(t, "call-slow", outcome.BatchRuntimeToolCallID)
}

// Simultaneous completions tie-break to the earliest call in call order,
// and N parallel calls of the same duration bill that duration once.
func TestExecuteLocalTools_SimultaneousCompletionsBillOnceByCallOrder(t *testing.T) {
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

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	// All three calls complete together 10 seconds in.
	clock.Advance(10 * time.Second)
	close(release)
	for range 3 {
		trap.MustWait(ctx).MustRelease(ctx)
	}

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 10*time.Second, outcome.BatchRuntime)
	require.Equal(t, "call-1", outcome.BatchRuntimeToolCallID)
}

// An unbilled tool never extends the window, even when it runs longest:
// the batch bills up to the last billed tool's completion.
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

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	// execute completes 10 seconds in.
	clock.Advance(10 * time.Second)
	close(executeGo)
	trap.MustWait(ctx).MustRelease(ctx)
	// wait_agent keeps blocking on its child until 60 seconds.
	clock.Advance(50 * time.Second)
	close(waitGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 10*time.Second, outcome.BatchRuntime)
	require.Equal(t, "call-execute", outcome.BatchRuntimeToolCallID)
}

// A batch of only unbilled tools bills nothing.
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

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	clock.Advance(60 * time.Second)
	close(waitGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Zero(t, outcome.BatchRuntime)
	require.Empty(t, outcome.BatchRuntimeToolCallID)
}

// Billing classifies on the name as called: a deprecated alias listed in
// UnbilledToolNames stays unbilled even though dispatch resolves it to
// its canonical tool through ToolNameAliases.
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

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	// execute completes 10 seconds in.
	clock.Advance(10 * time.Second)
	close(executeGo)
	trap.MustWait(ctx).MustRelease(ctx)
	// The aliased call completes at 60 seconds and must not bill.
	clock.Advance(50 * time.Second)
	close(legacyGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 10*time.Second, outcome.BatchRuntime)
	require.Equal(t, "call-execute", outcome.BatchRuntimeToolCallID)
}

// Duplicate tool call IDs, which reach execution when lifecycle hooks
// are disabled, must not corrupt the window: completions are tracked per
// occurrence, so a later short duplicate cannot overwrite an earlier
// long one and shrink the bill.
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
		// Both calls share one ID: an ID-keyed completion map would
		// let the fast occurrence overwrite the slow one.
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-dup", ToolName: "slow_tool", Input: "{}"},
			{ToolCallID: "call-dup", ToolName: "fast_tool", Input: "{}"},
		},
	})

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	// The fast occurrence completes at 10 seconds.
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	// The slow occurrence completes at 60 seconds and must define the
	// window even though the fast occurrence shares its ID.
	clock.Advance(50 * time.Second)
	close(slowGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 60*time.Second, outcome.BatchRuntime)
	require.Equal(t, "call-dup", outcome.BatchRuntimeToolCallID)
}

// OnBatchStart seeds billing state only once dispatch is guaranteed: it
// fires before the tool goroutines launch, and never fires when
// cancellation or the exclusive policy stops the batch, so an interrupt
// racing those paths finds no batch to bill.
func TestExecuteLocalTools_OnBatchStartFiresOnlyOnDispatch(t *testing.T) {
	t.Parallel()

	t.Run("dispatched batch seeds before tools run", func(t *testing.T) {
		t.Parallel()

		starts := 0
		seededWhenToolRan := false
		tool := fantasy.NewAgentTool(
			"fast_tool",
			"test tool",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				seededWhenToolRan = starts > 0
				return fantasy.NewTextResponse("done"), nil
			},
		)
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), chatloop.ExecuteLocalToolsOptions{
			Clock:        quartz.NewMock(t),
			Tools:        []fantasy.AgentTool{tool},
			ActiveTools:  []string{"fast_tool"},
			OnBatchStart: func() { starts++ },
			ToolCalls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "fast_tool", Input: "{}"},
			},
		})
		require.NoError(t, err)
		require.Len(t, outcome.Step.Content, 1)
		require.Equal(t, 1, starts)
		require.True(t, seededWhenToolRan, "the batch must be seeded before any tool goroutine runs")
	})

	t.Run("canceled context never seeds", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		started := false
		_, err := chatloop.ExecuteLocalTools(ctx, chatloop.ExecuteLocalToolsOptions{
			Clock:        quartz.NewMock(t),
			OnBatchStart: func() { started = true },
			ToolCalls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "fast_tool", Input: "{}"},
			},
		})
		require.ErrorIs(t, err, context.Canceled)
		require.False(t, started, "a canceled batch dispatches nothing and must not seed billing state")
	})

	t.Run("exclusive violation never seeds", func(t *testing.T) {
		t.Parallel()

		started := false
		outcome, err := chatloop.ExecuteLocalTools(context.Background(), chatloop.ExecuteLocalToolsOptions{
			Clock:              quartz.NewMock(t),
			ExclusiveToolNames: map[string]bool{"exclusive_tool": true},
			OnBatchStart:       func() { started = true },
			ToolCalls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "exclusive_tool", Input: "{}"},
				{ToolCallID: "call-2", ToolName: "fast_tool", Input: "{}"},
			},
		})
		require.NoError(t, err)
		require.Len(t, outcome.Step.Content, 2, "the whole batch resolves to synthesized policy errors")
		require.False(t, started, "a policy-rejected batch dispatches nothing and must not seed billing state")
	})
}

// A call without a tool call ID, which providers can emit, still
// executes: its completion defines the window like any other billed
// call instead of being discarded as a missing result.
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

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	// The identified call completes at 10 seconds.
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	// The ID-less call completes at 60 seconds and defines the window.
	clock.Advance(50 * time.Second)
	close(idlessGo)
	trap.MustWait(ctx).MustRelease(ctx)

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, 60*time.Second, outcome.BatchRuntime)
	require.Empty(t, outcome.BatchRuntimeToolCallID)
}

// OnToolComplete reports each tool's completion the instant it finishes,
// while slower siblings are still running, with the same instants the
// outcome's ToolResultCreatedAt later carries. The interrupt path
// depends on this live signal: results publish only after the whole
// batch finishes, so without it an interrupt could not tell finished
// tools from still-running ones.
func TestExecuteLocalTools_OnToolCompleteReportsLiveCompletions(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	trap := clock.Trap().Now()
	defer trap.Close()

	type completion struct {
		callIndex   int
		toolCallID  string
		completedAt time.Time
	}
	completionCh := make(chan completion, 2)
	fastGo := make(chan struct{})
	slowGo := make(chan struct{})
	resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
		Tools: []fantasy.AgentTool{
			blockingTool("fast_tool", fastGo, fantasy.NewTextResponse("done")),
			blockingTool("slow_tool", slowGo, fantasy.NewTextResponse("done")),
		},
		ActiveTools: []string{"fast_tool", "slow_tool"},
		OnToolComplete: func(callIndex int, toolCallID string, completedAt time.Time) {
			completionCh <- completion{callIndex: callIndex, toolCallID: toolCallID, completedAt: completedAt}
		},
		ToolCalls: []fantasy.ToolCallContent{
			{ToolCallID: "call-fast", ToolName: "fast_tool", Input: "{}"},
			{ToolCallID: "call-slow", ToolName: "slow_tool", Input: "{}"},
		},
	})

	// Batch start.
	trap.MustWait(ctx).MustRelease(ctx)
	// The fast tool completes 10 seconds in. Its completion arrives
	// while the slow tool is still parked on its release channel.
	clock.Advance(10 * time.Second)
	close(fastGo)
	trap.MustWait(ctx).MustRelease(ctx)
	fast := testutil.RequireReceive(ctx, t, completionCh)
	require.Equal(t, "call-fast", fast.toolCallID)
	require.Equal(t, 0, fast.callIndex, "callIndex is the call's position in dispatch order")
	// The slow tool completes at 60 seconds.
	clock.Advance(50 * time.Second)
	close(slowGo)
	trap.MustWait(ctx).MustRelease(ctx)
	slow := testutil.RequireReceive(ctx, t, completionCh)
	require.Equal(t, "call-slow", slow.toolCallID)
	require.Equal(t, 1, slow.callIndex, "callIndex is the call's position in dispatch order")
	require.Equal(t, 50*time.Second, slow.completedAt.Sub(fast.completedAt))

	outcome := testutil.RequireReceive(ctx, t, resultCh)
	require.Equal(t, map[string]time.Time{
		"call-fast": fast.completedAt,
		"call-slow": slow.completedAt,
	}, outcome.Step.ToolResultCreatedAt)
}
