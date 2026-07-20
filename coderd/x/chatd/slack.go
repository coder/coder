package chatd

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

// Labels and metadata keys that bind a chat to a Slack thread. They are
// stamped by the slackd integration (coderd/x/slackd) when it creates
// chats for Slack app mentions, and chatd uses them to decide whether
// to enable the Slack tools for a turn. They live here because slackd
// depends on chatd, so chatd cannot import slackd.
const (
	// LabelSlackd marks chats managed by slackd.
	LabelSlackd = "slackd"
	// LabelSlackThread stores the "<channel>:<thread_ts>" key that
	// binds a chat to a Slack thread.
	LabelSlackThread = "slack_thread"
	// LabelSlackShared marks chats owned by the configured fallback
	// chat owner because the Slack sender is not linked to a Coder
	// user. propose_mcp_server refuses in shared mode and asks the
	// agent to have the user connect their Coder account to Slack.
	LabelSlackShared = "slack_shared"
	// MetadataKeySlackEventID is the content-part metadata key that
	// stores the unique Slack event id used for deduplication.
	MetadataKeySlackEventID = "slack_event_id"
	// MetadataKeySlackMessageTS is the content-part metadata key that
	// stores the Slack message timestamp of an ingested thread message.
	// slackd stamps it on each <slack-message> block part so later
	// events can compute which thread messages the chat has not seen.
	MetadataKeySlackMessageTS = "slack_message_ts"
	// MetadataKeySlackPostedMessageTS is the content-part metadata key
	// that stores the Slack timestamp of a reply this chat posted via
	// the slack_send_message tool. chatd stamps it on the tool-result
	// part at persistence time; slackd uses it to exclude the chat's
	// own replies from thread ingestion.
	MetadataKeySlackPostedMessageTS = "slack_posted_message_ts"
	// MetadataKeySlackSenderID is the content-part metadata key that
	// stores the Slack user id of the sender whose message triggered
	// the submission. slackd stamps it on the header part so the MCP
	// proposal tool can resolve the Coder requester at row creation.
	MetadataKeySlackSenderID = "slack_sender_id"
)

// slackSendMessageToolName is the name of the chattool Slack tool
// whose successful results carry the posted message timestamp.
const slackSendMessageToolName = "slack_send_message"

// stampSlackPostedMessageTS records the Slack ts of a reply posted by
// the slack_send_message tool as content-part metadata on its
// tool-result part. Only successful results ({"ok": true, "ts": ...})
// are stamped; errored or malformed results are left untouched.
func stampSlackPostedMessageTS(part *codersdk.ChatMessagePart) {
	if part.Type != codersdk.ChatMessagePartTypeToolResult ||
		part.ToolName != slackSendMessageToolName ||
		part.IsError || len(part.Result) == 0 {
		return
	}
	var result struct {
		OK bool   `json:"ok"`
		TS string `json:"ts"`
	}
	if err := json.Unmarshal(part.Result, &result); err != nil || !result.OK || result.TS == "" {
		return
	}
	if part.Metadata == nil {
		part.Metadata = map[string]string{}
	}
	part.Metadata[MetadataKeySlackPostedMessageTS] = result.TS
}

// parseSlackThreadLabel splits a LabelSlackThread value into its
// channel and thread timestamp. It reports ok=false when either part
// is empty or the separator is missing.
func parseSlackThreadLabel(value string) (channel, threadTS string, ok bool) {
	channel, threadTS, found := strings.Cut(value, ":")
	if !found || channel == "" || threadTS == "" {
		return "", "", false
	}
	return channel, threadTS, true
}

// appendSlackTools adds the Slack tools when the chat is bound to a
// Slack thread via the slackd labels and the deployment has a Slack
// client configured. Plan-mode turns only get the read-only tools.
// A malformed slack_thread label disables the tools for the turn
// instead of failing it.
func (p *Server) appendSlackTools(
	ctx context.Context,
	tools []fantasy.AgentTool,
	opts rootChatToolsOptions,
) []fantasy.AgentTool {
	if p.slackAPI == nil {
		return tools
	}
	if opts.chat.Labels[LabelSlackd] != "true" {
		return tools
	}
	threadLabel, ok := opts.chat.Labels[LabelSlackThread]
	if !ok {
		return tools
	}
	channel, threadTS, ok := parseSlackThreadLabel(threadLabel)
	if !ok {
		p.logger.Warn(ctx, "chat has a malformed slack thread label, skipping slack tools",
			slog.F("chat_id", opts.chat.ID),
			slog.F("label", threadLabel),
		)
		return tools
	}
	slackOpts := chattool.SlackToolsOptions{
		API:      p.slackAPI,
		Channel:  channel,
		ThreadTS: threadTS,
		Logger:   p.logger,
	}
	mcpOpts := chattool.MCPServerToolsOptions{
		DB:               p.db,
		Logger:           p.logger,
		ChatID:           opts.chat.ID,
		ChatOwnerID:      opts.chat.OwnerID,
		AccessURL:        p.accessURL,
		SlackAPI:         p.slackAPI,
		Channel:          channel,
		ThreadTS:         threadTS,
		SlackSenderID:    opts.slackSenderID,
		ResolveSlackUser: p.slackUserResolver,
		SharedMode:       opts.chat.Labels[LabelSlackShared] == "true",
		ChatMCPServerIDs: func(ctx context.Context) ([]uuid.UUID, error) {
			chat, err := p.db.GetChatByID(ctx, opts.chat.ID)
			if err != nil {
				return nil, err
			}
			return chat.MCPServerIDs, nil
		},
		EnableMCPServer: func(ctx context.Context, serverID uuid.UUID) error {
			_, err := p.AddChatMCPServerID(ctx, opts.chat.ID, serverID)
			return err
		},

		ValidateOAuth2Discovery: p.validateOAuth2Discovery,
	}
	if opts.isPlanModeTurn {
		tools = append(tools, chattool.SlackReadOnlyTools(slackOpts)...)
		return append(tools, chattool.MCPServerReadOnlyTools(mcpOpts)...)
	}
	tools = append(tools, chattool.SlackTools(slackOpts)...)
	return append(tools, chattool.MCPServerTools(mcpOpts)...)
}

// latestSlackSenderID returns the Slack user id stamped on the newest
// user message, i.e. the sender of the message that started the
// current turn. It returns "" when no message carries the metadata
// (e.g. chats predating sender stamping).
func latestSlackSenderID(prompt []database.ChatMessage) string {
	for i := len(prompt) - 1; i >= 0; i-- {
		msg := prompt[i]
		if msg.Role != database.ChatMessageRoleUser || !msg.Content.Valid {
			continue
		}
		var parts []codersdk.ChatMessagePart
		if err := json.Unmarshal(msg.Content.RawMessage, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if sender := part.Metadata[MetadataKeySlackSenderID]; sender != "" {
				return sender
			}
		}
	}
	return ""
}
