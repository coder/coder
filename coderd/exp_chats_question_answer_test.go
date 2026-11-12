package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatQuestionAnswerExperimentDisabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t, withChatWorkerDisabled)
	user := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)

	_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: user.OrganizationID,
		Content: []codersdk.ChatInputPart{
			{Type: codersdk.ChatInputPartTypeQuestionAnswer},
			{Type: codersdk.ChatInputPartTypeText, Text: "Use Postgres."},
		},
	})

	sdkErr := requireSDKError(t, err, http.StatusBadRequest)
	require.Equal(t, "Async questions are not enabled on this deployment.", sdkErr.Message)
	require.Equal(t, "Enable the chat-async-questions experiment.", sdkErr.Detail)
}

func TestChatQuestionAnswer(t *testing.T) {
	t.Parallel()

	client := newChatClient(t, withChatWorkerDisabled, func(opts *coderdtest.Options) {
		opts.DeploymentValues.Experiments = append(opts.DeploymentValues.Experiments, string(codersdk.ExperimentChatAsyncQuestions))
	})
	user := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	questionID := uuid.New()
	const answer = "Use Postgres.\nKeep the cache in SQLite."
	input := []codersdk.ChatInputPart{
		{Type: codersdk.ChatInputPartTypeText, Text: answer},
		{Type: codersdk.ChatInputPartTypeQuestionAnswer, QuestionID: questionID},
	}
	wantContent := []codersdk.ChatMessagePart{
		{Type: codersdk.ChatMessagePartTypeQuestionAnswer, QuestionID: questionID.String()},
		{Type: codersdk.ChatMessagePartTypeText, Text: answer},
	}

	for _, testCase := range []struct {
		name    string
		content []codersdk.ChatInputPart
		message string
		detail  string
	}{
		{
			name: "NilQuestionID",
			content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: answer},
				{Type: codersdk.ChatInputPartTypeQuestionAnswer},
			},
			message: "Invalid input part.",
			detail:  "content[1].question_id is required.",
		},
		{
			name: "TwoQuestionAnswers",
			content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeQuestionAnswer, QuestionID: questionID},
				{Type: codersdk.ChatInputPartTypeQuestionAnswer, QuestionID: questionID},
				{Type: codersdk.ChatInputPartTypeText, Text: answer},
			},
			message: "Invalid input part.",
			detail:  "content can answer at most one question.",
		},
		{
			name: "QuestionAnswerOnly",
			content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeQuestionAnswer, QuestionID: questionID},
			},
			message: "Content is required.",
			detail:  "content must include at least one text or file part.",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: user.OrganizationID,
				Content:        testCase.content,
			})

			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equal(t, testCase.message, sdkErr.Message)
			require.Equal(t, testCase.detail, sdkErr.Detail)
		})
	}

	t.Run("AcceptedInCreate", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        input,
		})
		require.NoError(t, err)

		messages, err := client.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var userMessages []codersdk.ChatMessage
		for _, message := range messages.Messages {
			if message.Role == codersdk.ChatMessageRoleUser {
				userMessages = append(userMessages, message)
			}
		}
		require.Len(t, userMessages, 1)
		require.Equal(t, wantContent, userMessages[0].Content)
	})

	for _, testCase := range []struct {
		name string
		busy bool
	}{
		{name: "SentMessage"},
		{name: "QueuedMessage", busy: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			createRequest := codersdk.CreateChatRequest{OrganizationID: user.OrganizationID}
			if testCase.busy {
				createRequest.Content = []codersdk.ChatInputPart{
					{Type: codersdk.ChatInputPartTypeText, Text: "initial message"},
				}
			}
			chat, err := client.CreateChat(ctx, createRequest)
			require.NoError(t, err)

			created, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
				Content:      input,
				BusyBehavior: codersdk.ChatBusyBehaviorQueue,
			})
			require.NoError(t, err)
			require.Equal(t, testCase.busy, created.Queued)

			messages, err := client.GetChatMessages(ctx, chat.ID, nil)
			require.NoError(t, err)
			if testCase.busy {
				require.NotNil(t, created.QueuedMessage)
				require.Equal(t, wantContent, created.QueuedMessage.Content)
				require.Len(t, messages.QueuedMessages, 1)
				require.Equal(t, wantContent, messages.QueuedMessages[0].Content)
				return
			}

			require.NotNil(t, created.Message)
			require.Equal(t, wantContent, created.Message.Content)
			var userMessages []codersdk.ChatMessage
			for _, message := range messages.Messages {
				if message.Role == codersdk.ChatMessageRoleUser {
					userMessages = append(userMessages, message)
				}
			}
			require.Len(t, userMessages, 1)
			require.Equal(t, wantContent, userMessages[0].Content)
		})
	}
}
