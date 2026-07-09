package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"github.com/slack-go/slack"

	"cdr.dev/slog/v3"
)

// SlackAPI is the subset of the Slack Web API used by the Slack chat
// tools. *slack.Client satisfies it.
type SlackAPI interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
	AddReactionContext(ctx context.Context, name string, item slack.ItemRef) error
	RemoveReactionContext(ctx context.Context, name string, item slack.ItemRef) error
	GetConversationRepliesContext(ctx context.Context, params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error)
	GetUserInfoContext(ctx context.Context, user string) (*slack.User, error)
	UploadFileContext(ctx context.Context, params slack.UploadFileParameters) (*slack.FileSummary, error)
	SetAssistantThreadsStatusContext(ctx context.Context, params slack.AssistantThreadsSetStatusParameters) error
}

// SlackToolsOptions configures the Slack chat tools. API is required.
// Channel and ThreadTS identify the Slack thread the chat is bound to;
// every tool operates only within that thread and neither value is
// exposed as a tool argument.
type SlackToolsOptions struct {
	API      SlackAPI
	Channel  string
	ThreadTS string
	Logger   slog.Logger
}

// SlackTools returns all built-in Slack tools for a chat bound to a
// Slack thread, including the mutating ones.
func SlackTools(opts SlackToolsOptions) []fantasy.AgentTool {
	return append([]fantasy.AgentTool{
		slackSendMessage(opts),
		slackEditMessage(opts),
		slackReactToMessage(opts),
	}, SlackReadOnlyTools(opts)...)
}

// SlackReadOnlyTools returns only the Slack tools without side effects
// on Slack. Used on plan-mode turns.
func SlackReadOnlyTools(opts SlackToolsOptions) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		slackGetThreadReplies(opts),
		slackGetUserInfo(opts),
	}
}

type slackSendMessageAction struct {
	Text string `json:"text" description:"Button label"`
	URL  string `json:"url" description:"URL the button opens"`
}

type slackSendMessageSnippet struct {
	Name    string `json:"name" description:"Filename, e.g. main.go"`
	Content string `json:"content" description:"The snippet content"`
	Type    string `json:"type,omitempty" description:"Language for syntax highlighting, e.g. go or python. Omit for plain text."`
}

type slackSendMessageImage struct {
	URL     string `json:"url" description:"Public image URL"`
	AltText string `json:"alt_text,omitempty" description:"Alt text for the image"`
}

type slackSendMessageArgs struct {
	Text         string                    `json:"text" description:"The message text in Slack mrkdwn. User mentions must be <@USER_ID> (e.g. <@U01UBAM2C4D>). Never use @username."`
	Actions      []slackSendMessageAction  `json:"actions,omitempty" description:"Optional buttons to attach to the message."`
	TextSnippets []slackSendMessageSnippet `json:"text_snippets,omitempty" description:"Optional code or text snippets uploaded as files in the thread."`
	ImageURLs    []slackSendMessageImage   `json:"image_urls,omitempty" description:"Optional images to embed in the message."`
}

type slackEditMessageArgs struct {
	TS   string `json:"ts" description:"The timestamp of the message to edit (returned as ts from slack_send_message)"`
	Text string `json:"text" description:"The new message text in Slack mrkdwn. Same formatting rules as slack_send_message."`
}

type slackReactArgs struct {
	TS       string `json:"ts" description:"The message timestamp to react to"`
	Reaction string `json:"reaction" description:"Emoji name, e.g. :thumbsup: or thumbsup (colons are stripped automatically)"`
	Remove   bool   `json:"remove_reaction,omitempty" description:"Set to true to remove the reaction instead of adding it"`
}

type slackGetThreadRepliesArgs struct{}

type slackGetUserArgs struct {
	UserID string `json:"user_id" description:"The Slack user ID, e.g. U0123456789"`
}

func slackErrorResponse(err error) fantasy.ToolResponse {
	return toolResponse(map[string]any{"error": "slack error: " + err.Error()})
}

func slackSendMessage(opts SlackToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"slack_send_message",
		`Send a message to the Slack thread this chat is bound to. NEVER include citations, footnotes, or reference links like [1], [2], etc.

FORMATTING RULES:
- *text* = bold (NOT italics like in standard markdown)
- _text_ = italics
- ~text~ = strikethrough
- <http://example.com|link text> = links
- tables must be in a code block
- user mentions must be in the format <@user_id> (e.g. <@U01UBAM2C4D>)
- messages are limited to 3000 characters; if your response is longer, it will be truncated and you'll need to send follow-up messages

NEVER USE:
- Headings (#, ##, ###, etc.)
- Double asterisks (**text**) - Slack doesn't support this
- Standard markdown bold/italic conventions`,
		func(ctx context.Context, args slackSendMessageArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			formatted, truncated, originalLen := formatSlackMessage(args.Text)

			blocks := []slack.Block{
				slack.NewSectionBlock(
					slack.NewTextBlockObject(slack.MarkdownType, formatted, false, false),
					nil, nil,
				),
			}
			for _, img := range args.ImageURLs {
				alt := img.AltText
				if alt == "" {
					alt = "image"
				}
				blocks = append(blocks, slack.NewImageBlock(img.URL, alt, "", nil))
			}
			if len(args.Actions) > 0 {
				var elems []slack.BlockElement
				for _, a := range args.Actions {
					btn := slack.NewButtonBlockElement("", "",
						slack.NewTextBlockObject(slack.PlainTextType, a.Text, false, false))
					btn.URL = a.URL
					elems = append(elems, btn)
				}
				blocks = append(blocks, slack.NewActionBlock("", elems...))
			}

			_, msgTS, err := opts.API.PostMessageContext(ctx, opts.Channel,
				slack.MsgOptionText(formatted, false),
				slack.MsgOptionBlocks(blocks...),
				slack.MsgOptionTS(opts.ThreadTS),
			)
			if err != nil {
				return slackErrorResponse(err), nil
			}

			var snippetErrs []string
			for _, s := range args.TextSnippets {
				if _, err := opts.API.UploadFileContext(ctx, slack.UploadFileParameters{
					Channel:         opts.Channel,
					ThreadTimestamp: opts.ThreadTS,
					Content:         s.Content,
					Filename:        s.Name,
					Title:           s.Name,
					SnippetType:     s.Type,
				}); err != nil {
					snippetErrs = append(snippetErrs, s.Name+": "+err.Error())
				}
			}

			result := map[string]any{"ok": true, "ts": msgTS}
			if truncated {
				result["truncated"] = true
				result["original_length"] = originalLen
				result["note"] = "Your message was truncated to 3000 characters. Send a follow-up message to continue."
			}
			if len(snippetErrs) > 0 {
				result["snippet_errors"] = strings.Join(snippetErrs, "; ")
			}
			return toolResponse(result), nil
		})
}

func slackEditMessage(opts SlackToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"slack_edit_message",
		"Edit a previously sent Slack message in the bound thread. Use the ts returned by slack_send_message to identify the message. Same formatting rules apply.",
		func(ctx context.Context, args slackEditMessageArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			formatted, _, _ := formatSlackMessage(args.Text)
			blocks := []slack.Block{
				slack.NewSectionBlock(
					slack.NewTextBlockObject(slack.MarkdownType, formatted, false, false),
					nil, nil,
				),
			}
			_, _, _, err := opts.API.UpdateMessageContext(ctx, opts.Channel, args.TS,
				slack.MsgOptionText(formatted, false),
				slack.MsgOptionBlocks(blocks...),
			)
			if err != nil {
				return slackErrorResponse(err), nil
			}
			return toolResponse(map[string]any{"ok": true}), nil
		})
}

func slackReactToMessage(opts SlackToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"slack_react_to_message",
		"Add or remove an emoji reaction on a Slack message in the bound thread. Common reactions: :thumbsup:, :thumbsdown:, :eyes:, :heart:, :laughing:, :thinking_face:, :white_check_mark:, :rocket:",
		func(ctx context.Context, args slackReactArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			name := strings.Trim(args.Reaction, ":")
			ref := slack.ItemRef{Channel: opts.Channel, Timestamp: args.TS}
			var err error
			if args.Remove {
				err = opts.API.RemoveReactionContext(ctx, name, ref)
			} else {
				err = opts.API.AddReactionContext(ctx, name, ref)
			}
			if err != nil {
				return slackErrorResponse(err), nil
			}
			return toolResponse(map[string]any{"ok": true}), nil
		})
}

func slackGetThreadReplies(opts SlackToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"slack_get_thread_replies",
		"Read all replies in the Slack thread this chat is bound to for additional context.",
		func(ctx context.Context, _ slackGetThreadRepliesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			msgs, _, _, err := opts.API.GetConversationRepliesContext(ctx,
				&slack.GetConversationRepliesParameters{
					ChannelID: opts.Channel,
					Timestamp: opts.ThreadTS,
				})
			if err != nil {
				return slackErrorResponse(err), nil
			}
			type reply struct {
				User string `json:"user"`
				Text string `json:"text"`
				TS   string `json:"ts"`
			}
			replies := make([]reply, 0, len(msgs))
			for _, m := range msgs {
				replies = append(replies, reply{User: m.User, Text: m.Text, TS: m.Timestamp})
			}
			return marshalToolResponse(map[string]any{"replies": replies}), nil
		})
}

func slackGetUserInfo(opts SlackToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"slack_get_user_info",
		"Get profile information about a Slack user by their ID.",
		func(ctx context.Context, args slackGetUserArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			user, err := opts.API.GetUserInfoContext(ctx, args.UserID)
			if err != nil {
				return slackErrorResponse(err), nil
			}
			return toolResponse(map[string]any{
				"id":        user.ID,
				"name":      user.Name,
				"real_name": user.RealName,
				"email":     user.Profile.Email,
				"is_bot":    user.IsBot,
				"is_admin":  user.IsAdmin,
				"tz":        user.TZ,
			}), nil
		})
}
