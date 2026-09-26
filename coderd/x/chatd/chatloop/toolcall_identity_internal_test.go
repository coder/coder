package chatloop

import (
	"context"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestExecuteLocalToolsToolCallIdentity(t *testing.T) {
	t.Parallel()

	// seen holds what a tool read from its context. Age is read inside
	// the tool so the comparison does not dump the mock clock.
	type seen struct {
		chatID     uuid.UUID
		messageID  int64
		toolCallID string
		age        time.Duration
		ok         bool
	}
	// runBatch runs two parallel calls and one serial call. The parallel
	// calls wait for each other, so each reads its identity while its
	// sibling is running. clock advances 5s after both parallel calls
	// read their identity and before the serial call starts.
	runBatch := func(t *testing.T, opts ExecuteLocalToolsOptions, clock *quartz.Mock) map[string]seen {
		ctx := testutil.Context(t, testutil.WaitShort)
		var (
			mu     sync.Mutex
			got    = map[string]seen{}
			record = func(ctx context.Context, call fantasy.ToolCall) {
				identity, ok := chattool.ToolCallIdentityFromContext(ctx)
				mu.Lock()
				got[call.ID] = seen{
					chatID:     identity.ChatID,
					messageID:  identity.MessageID,
					toolCallID: identity.ToolCallID,
					age:        identity.Age.Now(),
					ok:         ok,
				}
				mu.Unlock()
			}
			arrived    = make(chan struct{}, 2)
			allArrived = make(chan struct{})
			recorded   = make(chan struct{}, 2)
			advanced   = make(chan struct{})
		)
		await := func(ctx context.Context, ch chan struct{}) error {
			select {
			case <-ch:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		go func() {
			for range 2 {
				if await(ctx, arrived) != nil {
					return
				}
			}
			close(allArrived)
			for range 2 {
				if await(ctx, recorded) != nil {
					return
				}
			}
			clock.Advance(5 * time.Second)
			close(advanced)
		}()
		parallel := fantasy.NewAgentTool("parallel_probe", "records its identity",
			func(ctx context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				arrived <- struct{}{}
				if err := await(ctx, allArrived); err != nil {
					return fantasy.ToolResponse{}, err
				}
				record(ctx, call)
				recorded <- struct{}{}
				if err := await(ctx, advanced); err != nil {
					return fantasy.ToolResponse{}, err
				}
				return fantasy.NewTextResponse("ok"), nil
			})
		serial := serialMarkerTool{fantasy.NewAgentTool("serial_probe", "records its identity",
			func(ctx context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				record(ctx, call)
				return fantasy.NewTextResponse("ok"), nil
			})}
		opts.Tools = []fantasy.AgentTool{parallel, serial}
		opts.ActiveTools = []string{"parallel_probe", "serial_probe"}
		opts.ToolCalls = []fantasy.ToolCallContent{
			{ToolCallID: "call-1", ToolName: "parallel_probe", Input: "{}"},
			{ToolCallID: "call-2", ToolName: "parallel_probe", Input: "{}"},
			{ToolCallID: "call-3", ToolName: "serial_probe", Input: "{}"},
		}
		opts.Clock = quartz.NewReal()
		outcome, err := ExecuteLocalTools(ctx, opts)
		require.NoError(t, err)
		for _, content := range outcome.Content {
			tr, ok := content.(fantasy.ToolResultContent)
			require.True(t, ok)
			require.IsType(t, fantasy.ToolResultOutputContentText{}, tr.Result, "call %s", tr.ToolCallID)
		}
		return got
	}

	t.Run("EachCallSeesItsOwnIdentity", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		dbNow := time.Now()
		clock := quartz.NewMock(t)
		got := runBatch(t, ExecuteLocalToolsOptions{
			ToolCallIdentity: chattool.ToolCallIdentity{
				ChatID:    chatID,
				MessageID: 42,
				Age:       chattool.NewToolCallAge(clock, dbNow, dbNow.Add(-time.Minute)),
			},
		}, clock)

		// The serial call starts after the clock advanced, and its age
		// is measured when it reads it.
		assert.Equal(t, map[string]seen{
			"call-1": {chatID: chatID, messageID: 42, toolCallID: "call-1", age: time.Minute, ok: true},
			"call-2": {chatID: chatID, messageID: 42, toolCallID: "call-2", age: time.Minute, ok: true},
			"call-3": {chatID: chatID, messageID: 42, toolCallID: "call-3", age: time.Minute + 5*time.Second, ok: true},
		}, got)
	})

	t.Run("NoChatIDNoIdentity", func(t *testing.T) {
		t.Parallel()

		got := runBatch(t, ExecuteLocalToolsOptions{}, quartz.NewMock(t))
		assert.Equal(t, map[string]seen{"call-1": {}, "call-2": {}, "call-3": {}}, got)
	})
}
