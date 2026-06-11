package chatloop

import (
	"context"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestPrepareMessagesForRequest_AnthropicPDFPreflightClassifiesInvalidPDF(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{ProviderName: fantasyanthropic.Name, ModelName: "claude-test"}
	messages := []fantasy.Message{
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.FilePart{
					Filename:  "bad.pdf",
					Data:      []byte("not a pdf"),
					MediaType: "application/pdf",
				},
			},
		},
	}

	canonical, prompt, err := prepareMessagesForRequest(
		context.Background(),
		RunOptions{
			Model:                model,
			ContextLimitFallback: 200_000,
			Logger:               slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		},
		messages,
		model.Provider(),
		model.Model(),
		0,
		1,
	)

	require.Error(t, err)
	require.Equal(t, messages, canonical)
	require.Nil(t, prompt)
	require.Contains(t, err.Error(), "bad.pdf")
	require.Contains(t, err.Error(), "data_bytes=9")

	classified := chaterror.Classify(err)
	require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
	require.Equal(t, fantasyanthropic.Name, classified.Provider)
	require.False(t, classified.Retryable)
	require.Contains(t, classified.Message, "bad.pdf")
	require.Contains(t, classified.Message, "not a valid PDF")
}
