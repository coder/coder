package chatd

import (
	"context"
	"testing"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

// stubSlackAPI is a no-op chattool.SlackAPI used to enable the Slack
// tools in gating tests. The tools are never run.
type stubSlackAPI struct{}

func (stubSlackAPI) PostMessageContext(context.Context, string, ...slack.MsgOption) (respChannel string, respTS string, err error) {
	return "", "", nil
}

func (stubSlackAPI) UpdateMessageContext(context.Context, string, string, ...slack.MsgOption) (respChannel string, respTS string, respText string, err error) {
	return "", "", "", nil
}

func (stubSlackAPI) AddReactionContext(context.Context, string, slack.ItemRef) error { return nil }

func (stubSlackAPI) RemoveReactionContext(context.Context, string, slack.ItemRef) error { return nil }

func (stubSlackAPI) GetConversationRepliesContext(context.Context, *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	return nil, false, "", nil
}

func (stubSlackAPI) GetUserInfoContext(context.Context, string) (*slack.User, error) {
	return &slack.User{}, nil
}

func (stubSlackAPI) SetAssistantThreadsStatusContext(context.Context, slack.AssistantThreadsSetStatusParameters) error {
	return nil
}

func (stubSlackAPI) UploadFileContext(context.Context, slack.UploadFileParameters) (*slack.FileSummary, error) {
	return &slack.FileSummary{}, nil
}

func TestParseSlackThreadLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		value       string
		wantChannel string
		wantThread  string
		wantOK      bool
	}{
		{"Valid", "C123:1700000000.000100", "C123", "1700000000.000100", true},
		{"MissingSeparator", "C123", "", "", false},
		{"EmptyChannel", ":1700000000.000100", "", "", false},
		{"EmptyThread", "C123:", "", "", false},
		{"Empty", "", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			channel, threadTS, ok := parseSlackThreadLabel(tc.value)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantChannel, channel)
			require.Equal(t, tc.wantThread, threadTS)
		})
	}
}

func TestAppendSlackTools(t *testing.T) {
	t.Parallel()

	slackLabels := database.StringMap{
		LabelSlackd:      "true",
		LabelSlackThread: "C123:1700000000.000100",
	}
	allSlackToolNames := []string{
		"slack_send_message",
		"slack_edit_message",
		"slack_react_to_message",
		"slack_get_thread_replies",
		"slack_get_user_info",
	}
	readOnlySlackToolNames := []string{
		"slack_get_thread_replies",
		"slack_get_user_info",
	}

	appended := func(server *Server, labels database.StringMap, planTurn bool) []string {
		tools := server.appendSlackTools(context.Background(), nil, rootChatToolsOptions{
			chat:           database.Chat{Labels: labels},
			isPlanModeTurn: planTurn,
		})
		names := make([]string, 0, len(tools))
		for _, tool := range tools {
			names = append(names, tool.Info().Name)
		}
		return names
	}

	t.Run("EnabledForSlackChat", func(t *testing.T) {
		t.Parallel()
		server := &Server{slackAPI: stubSlackAPI{}}
		require.ElementsMatch(t, allSlackToolNames, appended(server, slackLabels, false))
	})

	t.Run("ReadOnlyOnPlanTurn", func(t *testing.T) {
		t.Parallel()
		server := &Server{slackAPI: stubSlackAPI{}}
		require.ElementsMatch(t, readOnlySlackToolNames, appended(server, slackLabels, true))
	})

	t.Run("DisabledWithoutSlackAPI", func(t *testing.T) {
		t.Parallel()
		server := &Server{}
		require.Empty(t, appended(server, slackLabels, false))
	})

	t.Run("DisabledWithoutLabels", func(t *testing.T) {
		t.Parallel()
		server := &Server{slackAPI: stubSlackAPI{}}
		require.Empty(t, appended(server, nil, false))
	})

	t.Run("DisabledWithoutThreadLabel", func(t *testing.T) {
		t.Parallel()
		server := &Server{slackAPI: stubSlackAPI{}}
		require.Empty(t, appended(server, database.StringMap{LabelSlackd: "true"}, false))
	})

	t.Run("DisabledOnMalformedThreadLabel", func(t *testing.T) {
		t.Parallel()
		server := &Server{slackAPI: stubSlackAPI{}}
		require.Empty(t, appended(server, database.StringMap{
			LabelSlackd:      "true",
			LabelSlackThread: "no-separator",
		}, false))
	})
}
