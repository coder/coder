package chattool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/slack-go/slack"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// fakeSlackAPI records calls and returns configurable results.
type fakeSlackAPI struct {
	postChannel string
	postOptions []slack.MsgOption
	postTS      string
	postErr     error

	updateChannel string
	updateTS      string
	updateErr     error

	addedReactions   []string
	removedReactions []string
	reactionRefs     []slack.ItemRef
	reactionErr      error

	repliesParams *slack.GetConversationRepliesParameters
	replies       []slack.Message
	repliesErr    error

	userID  string
	user    *slack.User
	userErr error

	uploads   []slack.UploadFileParameters
	uploadErr error

	statusParams []slack.AssistantThreadsSetStatusParameters
	statusErr    error
}

func (f *fakeSlackAPI) SetAssistantThreadsStatusContext(_ context.Context, params slack.AssistantThreadsSetStatusParameters) error {
	f.statusParams = append(f.statusParams, params)
	return f.statusErr
}

func (f *fakeSlackAPI) PostMessageContext(_ context.Context, channelID string, options ...slack.MsgOption) (respChannel string, respTS string, err error) {
	f.postChannel = channelID
	f.postOptions = options
	if f.postErr != nil {
		return "", "", f.postErr
	}
	return channelID, f.postTS, nil
}

func (f *fakeSlackAPI) UpdateMessageContext(_ context.Context, channelID, timestamp string, _ ...slack.MsgOption) (respChannel string, respTS string, respText string, err error) {
	f.updateChannel = channelID
	f.updateTS = timestamp
	return channelID, timestamp, "", f.updateErr
}

func (f *fakeSlackAPI) AddReactionContext(_ context.Context, name string, item slack.ItemRef) error {
	f.addedReactions = append(f.addedReactions, name)
	f.reactionRefs = append(f.reactionRefs, item)
	return f.reactionErr
}

func (f *fakeSlackAPI) RemoveReactionContext(_ context.Context, name string, item slack.ItemRef) error {
	f.removedReactions = append(f.removedReactions, name)
	f.reactionRefs = append(f.reactionRefs, item)
	return f.reactionErr
}

func (f *fakeSlackAPI) GetConversationRepliesContext(_ context.Context, params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	f.repliesParams = params
	return f.replies, false, "", f.repliesErr
}

func (f *fakeSlackAPI) GetUserInfoContext(_ context.Context, user string) (*slack.User, error) {
	f.userID = user
	return f.user, f.userErr
}

func (f *fakeSlackAPI) UploadFileContext(_ context.Context, params slack.UploadFileParameters) (*slack.FileSummary, error) {
	f.uploads = append(f.uploads, params)
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	return &slack.FileSummary{}, nil
}

func slackTestOptions(api *fakeSlackAPI) SlackToolsOptions {
	return SlackToolsOptions{
		API:      api,
		Channel:  "C123",
		ThreadTS: "1700000000.000100",
	}
}

func runSlackTool(t *testing.T, tool fantasy.AgentTool, args any) map[string]any {
	t.Helper()
	input, err := json.Marshal(args)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  tool.Info().Name,
		Input: string(input),
	})
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}

func findSlackTool(t *testing.T, tools []fantasy.AgentTool, name string) fantasy.AgentTool {
	t.Helper()
	for _, tool := range tools {
		if tool.Info().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func TestSlackToolSets(t *testing.T) {
	t.Parallel()

	opts := slackTestOptions(&fakeSlackAPI{})

	var allNames []string
	for _, tool := range SlackTools(opts) {
		allNames = append(allNames, tool.Info().Name)
	}
	require.ElementsMatch(t, []string{
		"slack_send_message",
		"slack_edit_message",
		"slack_react_to_message",
		"slack_get_thread_replies",
		"slack_get_user_info",
	}, allNames)

	var readOnlyNames []string
	for _, tool := range SlackReadOnlyTools(opts) {
		readOnlyNames = append(readOnlyNames, tool.Info().Name)
	}
	require.ElementsMatch(t, []string{
		"slack_get_thread_replies",
		"slack_get_user_info",
	}, readOnlyNames)
}

func TestSlackSendMessage(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{postTS: "1700000001.000200"}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_send_message")

		result := runSlackTool(t, tool, map[string]any{"text": "hello"})
		require.Equal(t, true, result["ok"])
		require.Equal(t, "1700000001.000200", result["ts"])
		require.NotContains(t, result, "truncated")
		require.Equal(t, "C123", api.postChannel)
	})

	t.Run("Truncated", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{postTS: "1"}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_send_message")

		long := strings.Repeat("a", slackMessageMaxLen+100)
		result := runSlackTool(t, tool, map[string]any{"text": long})
		require.Equal(t, true, result["ok"])
		require.Equal(t, true, result["truncated"])
		require.EqualValues(t, slackMessageMaxLen+100, result["original_length"])
	})

	t.Run("Snippets", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{postTS: "1"}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_send_message")

		result := runSlackTool(t, tool, map[string]any{
			"text": "see snippet",
			"text_snippets": []map[string]string{
				{"name": "main.go", "content": "package main", "type": "go"},
			},
		})
		require.Equal(t, true, result["ok"])
		require.Len(t, api.uploads, 1)
		require.Equal(t, "C123", api.uploads[0].Channel)
		require.Equal(t, "1700000000.000100", api.uploads[0].ThreadTimestamp)
		require.Equal(t, "main.go", api.uploads[0].Filename)
	})

	t.Run("SnippetErrorReported", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{postTS: "1", uploadErr: xerrors.New("too big")}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_send_message")

		result := runSlackTool(t, tool, map[string]any{
			"text": "see snippet",
			"text_snippets": []map[string]string{
				{"name": "main.go", "content": "package main"},
			},
		})
		require.Equal(t, true, result["ok"])
		require.Contains(t, result["snippet_errors"], "main.go")
	})

	t.Run("SlackError", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{postErr: xerrors.New("channel_not_found")}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_send_message")

		result := runSlackTool(t, tool, map[string]any{"text": "hello"})
		require.Contains(t, result["error"], "channel_not_found")
	})
}

func TestSlackEditMessage(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_edit_message")

		result := runSlackTool(t, tool, map[string]any{"ts": "1700.1", "text": "updated"})
		require.Equal(t, true, result["ok"])
		require.Equal(t, "C123", api.updateChannel)
		require.Equal(t, "1700.1", api.updateTS)
	})

	t.Run("SlackError", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{updateErr: xerrors.New("message_not_found")}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_edit_message")

		result := runSlackTool(t, tool, map[string]any{"ts": "1700.1", "text": "updated"})
		require.Contains(t, result["error"], "message_not_found")
	})
}

func TestSlackReactToMessage(t *testing.T) {
	t.Parallel()

	t.Run("AddStripsColons", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_react_to_message")

		result := runSlackTool(t, tool, map[string]any{"ts": "1700.1", "reaction": ":thumbsup:"})
		require.Equal(t, true, result["ok"])
		require.Equal(t, []string{"thumbsup"}, api.addedReactions)
		require.Equal(t, "C123", api.reactionRefs[0].Channel)
		require.Equal(t, "1700.1", api.reactionRefs[0].Timestamp)
	})

	t.Run("Remove", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_react_to_message")

		result := runSlackTool(t, tool, map[string]any{
			"ts": "1700.1", "reaction": "eyes", "remove_reaction": true,
		})
		require.Equal(t, true, result["ok"])
		require.Empty(t, api.addedReactions)
		require.Equal(t, []string{"eyes"}, api.removedReactions)
	})

	t.Run("SlackError", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{reactionErr: xerrors.New("already_reacted")}
		tool := findSlackTool(t, SlackTools(slackTestOptions(api)), "slack_react_to_message")

		result := runSlackTool(t, tool, map[string]any{"ts": "1700.1", "reaction": "eyes"})
		require.Contains(t, result["error"], "already_reacted")
	})
}

func TestSlackGetThreadReplies(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{
			replies: []slack.Message{
				{Msg: slack.Msg{User: "U1", Text: "hi", Timestamp: "1700.1"}},
				{Msg: slack.Msg{User: "U2", Text: "yo", Timestamp: "1700.2"}},
			},
		}
		tool := findSlackTool(t, SlackReadOnlyTools(slackTestOptions(api)), "slack_get_thread_replies")

		result := runSlackTool(t, tool, map[string]any{})
		replies, ok := result["replies"].([]any)
		require.True(t, ok)
		require.Len(t, replies, 2)
		first, ok := replies[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "U1", first["user"])
		require.Equal(t, "hi", first["text"])
		require.Equal(t, "1700.1", first["ts"])
		// The bound thread is used, not a model-provided one.
		require.Equal(t, "C123", api.repliesParams.ChannelID)
		require.Equal(t, "1700000000.000100", api.repliesParams.Timestamp)
	})

	t.Run("SlackError", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{repliesErr: xerrors.New("thread_not_found")}
		tool := findSlackTool(t, SlackReadOnlyTools(slackTestOptions(api)), "slack_get_thread_replies")

		result := runSlackTool(t, tool, map[string]any{})
		require.Contains(t, result["error"], "thread_not_found")
	})
}

func TestSlackGetUserInfo(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{
			user: &slack.User{
				ID:       "U1",
				Name:     "jdoe",
				RealName: "Jane Doe",
				IsBot:    false,
				IsAdmin:  true,
				TZ:       "Europe/Warsaw",
				Profile:  slack.UserProfile{Email: "jdoe@example.com"},
			},
		}
		tool := findSlackTool(t, SlackReadOnlyTools(slackTestOptions(api)), "slack_get_user_info")

		result := runSlackTool(t, tool, map[string]any{"user_id": "U1"})
		require.Equal(t, "U1", api.userID)
		require.Equal(t, "jdoe", result["name"])
		require.Equal(t, "Jane Doe", result["real_name"])
		require.Equal(t, "jdoe@example.com", result["email"])
		require.Equal(t, true, result["is_admin"])
	})

	t.Run("SlackError", func(t *testing.T) {
		t.Parallel()
		api := &fakeSlackAPI{userErr: xerrors.New("user_not_found")}
		tool := findSlackTool(t, SlackReadOnlyTools(slackTestOptions(api)), "slack_get_user_info")

		result := runSlackTool(t, tool, map[string]any{"user_id": "U404"})
		require.Contains(t, result["error"], "user_not_found")
	})
}

func TestFormatSlackMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"BoldConverted", "this is **bold** text", "this is *bold* text"},
		{"HeadingConverted", "# Title\nbody", "*Title*\nbody"},
		{"LinkConverted", "see [docs](https://example.com)", "see <https://example.com|docs>"},
		{"MentionWrapped", "ping @U01UBAM2C4D please", "ping <@U01UBAM2C4D> please"},
		{"ExistingMentionPreserved", "ping <@U01UBAM2C4D>", "ping <@U01UBAM2C4D>"},
		{"HandleUnwrapped", "hey <@someuser>", "hey @someuser"},
		{"CodeBlockPreserved", "look:\n```go\n**not bold**\n```", "look:\n```\n**not bold**\n```"},
		{"InlineCodePreserved", "use `**raw**` here", "use `**raw**` here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, truncated, origLen := formatSlackMessage(tc.in)
			require.Equal(t, tc.want, got)
			require.False(t, truncated)
			require.Equal(t, len(tc.in), origLen)
		})
	}

	t.Run("Truncation", func(t *testing.T) {
		t.Parallel()
		in := strings.Repeat("x", slackMessageMaxLen+1)
		got, truncated, origLen := formatSlackMessage(in)
		require.True(t, truncated)
		require.Equal(t, slackMessageMaxLen+1, origLen)
		require.Len(t, got, slackMessageMaxLen)
	})
}
