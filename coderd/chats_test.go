package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/coderd/chats"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

type fakeLLMFactory struct {
	stream aisdk.DataStream
}

func (f fakeLLMFactory) New(_ string, _ *http.Client) (chats.LLMClient, error) {
	return fakeLLM(f), nil
}

type fakeLLM struct {
	stream aisdk.DataStream
}

func (f fakeLLM) StreamChat(_ context.Context, _ chats.LLMRequest) (aisdk.DataStream, error) {
	return f.stream, nil
}

func TestChats_CreateChatAndRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, _, api := coderdtest.NewWithAPI(t, nil)
	coderdtest.CreateFirstUser(t, client)

	api.ChatRunner = chats.NewRunner(chats.RunnerOptions{
		DB:         api.Database,
		Logger:     api.Logger,
		AccessURL:  api.AccessURL,
		HTTPClient: api.HTTPClient,
		LLMFactory: fakeLLMFactory{stream: simpleAssistantStream("hello from assistant")},
		Tools:      []toolsdk.GenericTool{},
		MaxSteps:   1,
	})

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		Provider: "openai",
		Model:    "gpt-4o-mini",
	})
	require.NoError(t, err)

	resp, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{Content: "hi"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.RunID)

	require.Eventually(t, func() bool {
		msgs, err := client.ChatMessages(ctx, chat.ID)
		if err != nil {
			return false
		}
		for _, m := range msgs {
			if m.Role != "assistant" {
				continue
			}
			var env chats.MessageEnvelope
			if err := json.Unmarshal(m.Content, &env); err != nil {
				continue
			}
			return env.RunID == resp.RunID && env.Message.Content == "hello from assistant"
		}
		return false
	}, testutil.WaitShort, testutil.IntervalFast)
}

func simpleAssistantStream(text string) aisdk.DataStream {
	return func(yield func(aisdk.DataStreamPart, error) bool) {
		if !yield(aisdk.TextStreamPart{Content: text}, nil) {
			return
		}
		_ = yield(aisdk.FinishMessageStreamPart{FinishReason: aisdk.FinishReasonStop}, nil)
	}
}
