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
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			clock.Advance(1500 * time.Millisecond)
			return func(yield func(fantasy.StreamPart) bool) {
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"})
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "summary"})
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"})
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
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

// Each case scripts one tool batch through the trapped clock: the batch
// start timestamp is released first, then every step advances the clock,
// lets one tool's calls finish, and releases the Now() calls they trap.
func TestExecuteLocalTools_BatchRuntime(t *testing.T) {
	t.Parallel()

	type tool struct {
		name     string
		response fantasy.ToolResponse
		serial   bool
	}
	type step struct {
		advance time.Duration
		finish  string
		// releases counts the trapped Now() calls that follow finish: one
		// per completing call, plus one for the next serial call's start.
		releases int
	}
	cases := []struct {
		name            string
		tools           []tool
		unbilled        map[string]bool
		aliases         map[string]string
		calls           []fantasy.ToolCallContent
		steps           []step
		wantRuntime     time.Duration
		wantBilledCalls int
	}{
		{
			name: "BatchWindowIsMaxNotSum",
			tools: []tool{
				{name: "fast_tool", response: fantasy.NewTextResponse("done")},
				{name: "slow_tool", response: fantasy.NewTextErrorResponse("blew up")},
			},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-fast", ToolName: "fast_tool", Input: "{}"},
				{ToolCallID: "call-slow", ToolName: "slow_tool", Input: "{}"},
			},
			steps: []step{
				{advance: 10 * time.Second, finish: "fast_tool", releases: 1},
				{advance: 50 * time.Second, finish: "slow_tool", releases: 1},
			},
			wantRuntime:     60 * time.Second,
			wantBilledCalls: 2,
		},
		{
			name: "SimultaneousCompletionsBillOnce",
			tools: []tool{
				{name: "read_tool", response: fantasy.NewTextResponse("done")},
			},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-1", ToolName: "read_tool", Input: "{}"},
				{ToolCallID: "call-2", ToolName: "read_tool", Input: "{}"},
				{ToolCallID: "call-3", ToolName: "read_tool", Input: "{}"},
			},
			steps: []step{
				{advance: 10 * time.Second, finish: "read_tool", releases: 3},
			},
			wantRuntime:     10 * time.Second,
			wantBilledCalls: 3,
		},
		{
			name: "UnbilledToolNeverExtendsWindow",
			tools: []tool{
				{name: "execute", response: fantasy.NewTextResponse("done")},
				{name: "wait_agent", response: fantasy.NewTextResponse("child report")},
			},
			unbilled: map[string]bool{"wait_agent": true},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-execute", ToolName: "execute", Input: "{}"},
				{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
			},
			steps: []step{
				{advance: 10 * time.Second, finish: "execute", releases: 1},
				{advance: 50 * time.Second, finish: "wait_agent", releases: 1},
			},
			wantRuntime:     10 * time.Second,
			wantBilledCalls: 1,
		},
		{
			name: "UnbilledOnlyBatchBillsNothing",
			tools: []tool{
				{name: "wait_agent", response: fantasy.NewTextResponse("child report")},
			},
			unbilled: map[string]bool{"wait_agent": true},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
			},
			steps: []step{
				{advance: 60 * time.Second, finish: "wait_agent", releases: 1},
			},
			wantRuntime:     0,
			wantBilledCalls: 0,
		},
		{
			name: "AliasNamesClassifyAsCalled",
			tools: []tool{
				{name: "execute", response: fantasy.NewTextResponse("done")},
				{name: "interrupt_agent", response: fantasy.NewTextResponse("stopped")},
			},
			aliases: map[string]string{"close_agent": "interrupt_agent"},
			unbilled: map[string]bool{
				"interrupt_agent": true,
				"close_agent":     true,
			},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-execute", ToolName: "execute", Input: "{}"},
				{ToolCallID: "call-legacy", ToolName: "close_agent", Input: "{}"},
			},
			steps: []step{
				{advance: 10 * time.Second, finish: "execute", releases: 1},
				{advance: 50 * time.Second, finish: "interrupt_agent", releases: 1},
			},
			wantRuntime:     10 * time.Second,
			wantBilledCalls: 1,
		},
		{
			name: "DuplicateToolCallIDsKeepOccurrenceCompletions",
			tools: []tool{
				{name: "slow_tool", response: fantasy.NewTextResponse("done")},
				{name: "fast_tool", response: fantasy.NewTextResponse("done")},
			},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-dup", ToolName: "slow_tool", Input: "{}"},
				{ToolCallID: "call-dup", ToolName: "fast_tool", Input: "{}"},
			},
			steps: []step{
				{advance: 10 * time.Second, finish: "fast_tool", releases: 1},
				{advance: 50 * time.Second, finish: "slow_tool", releases: 1},
			},
			wantRuntime:     60 * time.Second,
			wantBilledCalls: 2,
		},
		{
			name: "EmptyToolCallIDStillBillsWindow",
			tools: []tool{
				{name: "idless_tool", response: fantasy.NewTextResponse("done")},
				{name: "fast_tool", response: fantasy.NewTextResponse("done")},
			},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "", ToolName: "idless_tool", Input: "{}"},
				{ToolCallID: "call-fast", ToolName: "fast_tool", Input: "{}"},
			},
			steps: []step{
				{advance: 10 * time.Second, finish: "fast_tool", releases: 1},
				{advance: 50 * time.Second, finish: "idless_tool", releases: 1},
			},
			wantRuntime:     60 * time.Second,
			wantBilledCalls: 2,
		},
		{
			// A serial call bills its own execution, not the unbilled wait
			// that delayed its launch.
			name: "SerialCallBillsFromItsOwnStart",
			tools: []tool{
				{name: "wait_agent", response: fantasy.NewTextResponse("child report")},
				{name: "serial_tool", response: fantasy.NewTextResponse("done"), serial: true},
			},
			unbilled: map[string]bool{"wait_agent": true},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
				{ToolCallID: "call-serial", ToolName: "serial_tool", Input: "{}"},
			},
			steps: []step{
				{advance: 10 * time.Minute, finish: "wait_agent", releases: 2},
				{advance: 2 * time.Second, finish: "serial_tool", releases: 1},
			},
			wantRuntime:     2 * time.Second,
			wantBilledCalls: 1,
		},
		{
			// The 3s concurrent window and the 2s serial window bill; the 7s
			// span where only wait_agent ran does not.
			name: "SerialAfterBilledSiblingBillsUnion",
			tools: []tool{
				{name: "execute", response: fantasy.NewTextResponse("done")},
				{name: "wait_agent", response: fantasy.NewTextResponse("child report")},
				{name: "serial_tool", response: fantasy.NewTextResponse("done"), serial: true},
			},
			unbilled: map[string]bool{"wait_agent": true},
			calls: []fantasy.ToolCallContent{
				{ToolCallID: "call-execute", ToolName: "execute", Input: "{}"},
				{ToolCallID: "call-wait", ToolName: "wait_agent", Input: "{}"},
				{ToolCallID: "call-serial", ToolName: "serial_tool", Input: "{}"},
			},
			steps: []step{
				{advance: 3 * time.Second, finish: "execute", releases: 1},
				{advance: 7 * time.Second, finish: "wait_agent", releases: 2},
				{advance: 2 * time.Second, finish: "serial_tool", releases: 1},
			},
			wantRuntime:     5 * time.Second,
			wantBilledCalls: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			clock := quartz.NewMock(t)
			trap := clock.Trap().Now()
			defer trap.Close()

			releases := make(map[string]chan struct{}, len(tc.tools))
			tools := make([]fantasy.AgentTool, 0, len(tc.tools))
			activeTools := make([]string, 0, len(tc.tools))
			for _, tl := range tc.tools {
				release := make(chan struct{})
				releases[tl.name] = release
				agentTool := blockingTool(tl.name, release, tl.response)
				if tl.serial {
					agentTool = serialTool{agentTool}
				}
				tools = append(tools, agentTool)
				activeTools = append(activeTools, tl.name)
			}
			resultCh := executeToolBatch(t, clock, chatloop.ExecuteLocalToolsOptions{
				Tools:             tools,
				ActiveTools:       activeTools,
				ToolNameAliases:   tc.aliases,
				UnbilledToolNames: tc.unbilled,
				ToolCalls:         tc.calls,
			})

			trap.MustWait(ctx).MustRelease(ctx)
			for _, st := range tc.steps {
				clock.Advance(st.advance)
				close(releases[st.finish])
				for range st.releases {
					trap.MustWait(ctx).MustRelease(ctx)
				}
			}

			outcome := testutil.RequireReceive(ctx, t, resultCh)
			require.Equal(t, tc.wantRuntime, outcome.BatchRuntime)
			require.Equal(t, tc.wantBilledCalls, outcome.BatchBilledCalls)
		})
	}
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
