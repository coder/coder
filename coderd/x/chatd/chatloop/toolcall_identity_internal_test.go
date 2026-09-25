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
	// sibling is running.
	runBatch := func(t *testing.T, opts ExecuteLocalToolsOptions) map[string]seen {
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
		)
		go func() {
			for range 2 {
				select {
				case <-arrived:
				case <-ctx.Done():
					return
				}
			}
			close(allArrived)
		}()
		parallel := fantasy.NewAgentTool("parallel_probe", "records its identity",
			func(ctx context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
				arrived <- struct{}{}
				select {
				case <-allArrived:
				case <-ctx.Done():
					return fantasy.ToolResponse{}, ctx.Err()
				}
				record(ctx, call)
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
		got := runBatch(t, ExecuteLocalToolsOptions{
			ChatID:            chatID,
			ToolCallMessageID: 42,
			ToolCallAge:       chattool.NewToolCallAge(quartz.NewMock(t), dbNow, dbNow.Add(-time.Minute)),
		})

		want := map[string]seen{}
		for _, id := range []string{"call-1", "call-2", "call-3"} {
			want[id] = seen{chatID: chatID, messageID: 42, toolCallID: id, age: time.Minute, ok: true}
		}
		assert.Equal(t, want, got)
	})

	t.Run("NoChatIDNoIdentity", func(t *testing.T) {
		t.Parallel()

		got := runBatch(t, ExecuteLocalToolsOptions{})
		assert.Equal(t, map[string]seen{"call-1": {}, "call-2": {}, "call-3": {}}, got)
	})
}
