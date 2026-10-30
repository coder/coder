package mcpbuiltin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
)

// Slack server option query parameters.
const (
	// SlackOptionAPIBaseURL overrides the Slack Web API base URL, for
	// example to route through a proxy.
	SlackOptionAPIBaseURL = "api_base_url"
	// SlackOptionPinThread restricts thread tools to the Slack thread
	// the chat came from, rejecting other channels and threads.
	SlackOptionPinThread = "pin_thread"
)

const slackFormattingRules = `Formatting rules (Slack mrkdwn):
- *text* is bold (single asterisks, never **text**)
- _text_ is italic
- ` + "`text`" + ` is inline code, ` + "```...```" + ` is a code block (no language tag)
- ~text~ is strikethrough
- <https://example.com|link text> is a link
- mention users as <@U01234ABCDE>
- 3000 characters at most; longer text is truncated
Never use Markdown headings (#, ##) or double asterisks.`

const slackServerInstructions = `Tools for the Slack workspace of the connected Slack app.
When this chat started from a Slack thread, the thread tools act on that thread by default; leave channel and thread_ts empty to reply there. Only pass another channel or thread when the user explicitly asks you to post somewhere else.
Report progress with slack_report_status while you work, and send your final answer with slack_send_message.`

// slackServer serves the built-in Slack MCP server.
type slackServer struct {
	h *Handler
}

func (s *slackServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opts, err := newQueryOptions(r.URL, SlackOptionAPIBaseURL, SlackOptionPinThread)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}
	info := requestInfoFrom(ctx)
	baseURL, err := s.h.parseAPIBaseURL(opts.string(SlackOptionAPIBaseURL), defaultSlackAPIBaseURL)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}
	pinThread, err := opts.bool(SlackOptionPinThread, false)
	if err != nil {
		writeOptionError(ctx, rw, err)
		return
	}

	session := &slackSession{
		h:         s.h,
		info:      info,
		pinThread: pinThread,
		api: slackAPI{
			baseURL: baseURL,
			token:   info.token,
			client:  s.h.upstream,
		},
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "coder-slack",
		Title:   "Slack",
		Version: buildinfo.Version(),
	}, &mcp.ServerOptions{
		Instructions: slackServerInstructions,
		Logger:       s.h.sdkLogger,
		SchemaCache:  s.h.schema,
	})
	session.register(server)
	streamableHandler(server).ServeHTTP(rw, r)
}

// slackSession holds the state of one MCP request to the Slack server.
type slackSession struct {
	h         *Handler
	info      requestInfo
	pinThread bool
	api       slackAPI
}

// slackTarget is where a thread tool acts.
type slackTarget struct {
	channel  string
	threadTS string
}

// resolveTarget picks the channel and thread a tool acts on. Omitted
// arguments default to the Slack thread the calling chat came from. An
// explicit channel without thread_ts targets the channel itself.
func (s *slackSession) resolveTarget(ctx context.Context, channel, threadTS string) (slackTarget, error) {
	channel = strings.TrimSpace(channel)
	threadTS = strings.TrimSpace(threadTS)
	origin, hasOrigin, err := s.h.originSlackThread(ctx, s.info)
	if err != nil {
		return slackTarget{}, err
	}
	if !hasOrigin {
		if s.pinThread {
			return slackTarget{}, toolErr("This Slack server only acts on the Slack thread a chat started from, and this chat did not start from one.")
		}
		if channel == "" {
			return slackTarget{}, toolErr("This chat did not start from a Slack thread, so channel is required.")
		}
		return slackTarget{channel: channel, threadTS: threadTS}, nil
	}
	if channel == "" {
		channel = origin.Channel
		if threadTS == "" {
			threadTS = origin.ThreadTS
		}
	}
	if s.pinThread && (channel != origin.Channel || threadTS != origin.ThreadTS) {
		return slackTarget{}, toolErr("This Slack server only acts on the Slack thread this chat started from. Leave channel and thread_ts empty.")
	}
	return slackTarget{channel: channel, threadTS: threadTS}, nil
}

// targetFailure turns a resolveTarget error into a tool error. Internal
// errors, such as database failures, are logged instead of shown.
func (s *slackSession) targetFailure(ctx context.Context, err error) *mcp.CallToolResult {
	var te *toolError
	if errors.As(err, &te) {
		return errorResult(te.msg)
	}
	s.h.logger.Error(ctx, "resolve the Slack thread of a chat", slog.Error(err))
	return errorResult("Could not look up the Slack thread this chat started from. Try again later.")
}

// resolveThread is resolveTarget for tools that only act on a thread.
func (s *slackSession) resolveThread(ctx context.Context, channel, threadTS string) (slackTarget, error) {
	target, err := s.resolveTarget(ctx, channel, threadTS)
	if err != nil {
		return slackTarget{}, err
	}
	if target.threadTS == "" {
		return slackTarget{}, toolErr("thread_ts is required when channel is set.")
	}
	return target, nil
}

type slackAction struct {
	Text string `json:"text" jsonschema:"Button label, 75 characters at most."`
	URL  string `json:"url" jsonschema:"The https URL the button opens."`
}

type slackSendMessageArgs struct {
	Message        string        `json:"message" jsonschema:"Message text in Slack mrkdwn, 3000 characters at most."`
	Actions        []slackAction `json:"actions,omitempty" jsonschema:"Optional link buttons shown below the message, for example links to pull requests or issues."`
	Channel        string        `json:"channel,omitempty" jsonschema:"Channel ID to post in. Leave empty to reply in the Slack thread this chat started from. Only set it when the user asks you to post elsewhere."`
	ThreadTS       string        `json:"thread_ts,omitempty" jsonschema:"Timestamp of the thread to reply in. Leave empty to reply in the Slack thread this chat started from, or to post a new top-level message when channel is set."`
	AllowBroadcast bool          `json:"allow_broadcast,omitempty" jsonschema:"Set to true only when the user explicitly asked you to notify everyone with @channel, @here, or @everyone. Otherwise those mentions are sent as plain text."`
}

type slackReportStatusArgs struct {
	Status          string   `json:"status" jsonschema:"Short status such as 'Searching files...'. Pass an empty string to clear the indicator."`
	LoadingMessages []string `json:"loading_messages,omitempty" jsonschema:"Optional short messages Slack rotates through while the status is set."`
	Channel         string   `json:"channel,omitempty" jsonschema:"Channel ID. Leave empty to use the Slack thread this chat started from."`
	ThreadTS        string   `json:"thread_ts,omitempty" jsonschema:"Thread timestamp. Leave empty to use the Slack thread this chat started from."`
}

type slackReactArgs struct {
	Reaction string `json:"reaction" jsonschema:"Emoji name without colons, such as thumbsup, eyes, or white_check_mark."`
	TS       string `json:"ts,omitempty" jsonschema:"Timestamp of the message to react to. Leave empty to react to the message that started the thread."`
	Remove   bool   `json:"remove,omitempty" jsonschema:"Set to true to remove the reaction instead of adding it."`
	Channel  string `json:"channel,omitempty" jsonschema:"Channel ID of the message. Leave empty to use the Slack thread this chat started from."`
}

type slackReadThreadArgs struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"Number of messages to return, 50 by default and 200 at most."`
	Cursor   string `json:"cursor,omitempty" jsonschema:"Pagination cursor from a previous call."`
	Channel  string `json:"channel,omitempty" jsonschema:"Channel ID. Leave empty to read the Slack thread this chat started from."`
	ThreadTS string `json:"thread_ts,omitempty" jsonschema:"Thread timestamp. Leave empty to read the Slack thread this chat started from."`
}

type slackUserInfoArgs struct {
	UserID string `json:"user_id" jsonschema:"The Slack user ID, such as U01234ABCDE."`
}

type slackSearchArgs struct {
	Query string `json:"query" jsonschema:"Slack search query. Supports modifiers such as 'in:#general from:@alex deploy' or 'has:link before:2024-01-01'."`
	Count int    `json:"count,omitempty" jsonschema:"Maximum number of matches, 10 by default and 100 at most."`
}

func boolPtr(b bool) *bool { return &b }

func (s *slackSession) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "slack_send_message",
		Description: "Send a message to Slack. By default it replies in the Slack thread this chat started from, which is the only place the Slack user sees your answer. " +
			"Send your final answer once; do not echo your reasoning. Sending clears the thread's status indicator.\n\n" + slackFormattingRules,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	}, s.sendMessage)

	mcp.AddTool(server, &mcp.Tool{
		Name: "slack_report_status",
		Description: "Update the status indicator at the bottom of the Slack thread, for example 'Looking up the issue...'. " +
			"Call it before long-running work so the Slack user sees progress. Pass an empty status to clear it. Needs the assistant:write scope.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(true)},
	}, s.reportStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "slack_react_to_message",
		Description: "Add or remove an emoji reaction on a Slack message. By default it reacts to the message that started this chat's Slack thread.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(true)},
	}, s.react)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "slack_read_thread_replies",
		Description: "Read the messages in a Slack thread. By default it reads the thread this chat started from. Use it when the user refers to earlier discussion.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(true)},
	}, s.readThread)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "slack_get_user_info",
		Description: "Look up a Slack user by ID, for example the sender of the incoming message or someone mentioned in it.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(true)},
	}, s.userInfo)

	mcp.AddTool(server, &mcp.Tool{
		Name: "slack_search_messages",
		Description: "Search Slack messages visible to the token's user. Use it to gather context or find linked issues and pull requests. " +
			"Slack only allows search with a user token (xoxp-), not a bot token.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(true)},
	}, s.search)
}

func (s *slackSession) sendMessage(ctx context.Context, _ *mcp.CallToolRequest, args slackSendMessageArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Message) == "" {
		return errorResult("message must not be empty."), nil, nil
	}
	target, err := s.resolveTarget(ctx, args.Channel, args.ThreadTS)
	if err != nil {
		return s.targetFailure(ctx, err), nil, nil
	}

	text, truncated, originalLength := formatSlackMessage(args.Message)
	if !args.AllowBroadcast {
		text = neutralizeSlackBroadcasts(text)
	}
	blocks := []any{map[string]any{
		"type": "section",
		"text": map[string]any{"type": "mrkdwn", "text": text},
	}}
	if len(args.Actions) > 0 {
		if len(args.Actions) > 25 {
			return errorResult("actions supports 25 buttons at most."), nil, nil
		}
		elements := make([]any, 0, len(args.Actions))
		for _, action := range args.Actions {
			u, err := url.Parse(action.URL)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				return errorResult("each action url must be an http or https URL."), nil, nil
			}
			label := []rune(strings.TrimSpace(action.Text))
			if len(label) == 0 {
				return errorResult("each action needs a text label."), nil, nil
			}
			if len(label) > 75 {
				label = label[:75]
			}
			elements = append(elements, map[string]any{
				"type": "button",
				"text": map[string]any{"type": "plain_text", "text": string(label)},
				"url":  u.String(),
			})
		}
		blocks = append(blocks, map[string]any{"type": "actions", "elements": elements})
	}
	if s.info.verified && s.h.opts.AccessURL != nil {
		chatURL := s.h.opts.AccessURL.JoinPath("agents", s.info.identity.ChatID.String())
		blocks = append(blocks, map[string]any{
			"type": "context",
			"elements": []any{map[string]any{
				"type": "mrkdwn",
				"text": "<" + chatURL.String() + "|View chat>",
			}},
		})
	}

	params := url.Values{
		"channel": {target.channel},
		"text":    {text},
		"blocks":  {jsonParam(blocks)},
	}
	if target.threadTS != "" {
		params.Set("thread_ts", target.threadTS)
	}
	var resp struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := s.api.call(ctx, "chat.postMessage", params, &resp); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	if target.threadTS != "" {
		// Best effort: the reply is already visible, and a missing
		// assistant:write scope must not turn a sent message into an error.
		_ = s.api.call(ctx, "assistant.threads.setStatus", url.Values{
			"channel_id": {target.channel},
			"thread_ts":  {target.threadTS},
			"status":     {""},
		}, nil)
	}

	out := map[string]any{"ok": true, "channel": resp.Channel, "ts": resp.TS}
	if target.threadTS != "" {
		out["thread_ts"] = target.threadTS
	}
	if truncated {
		out["truncated"] = true
		out["original_length"] = originalLength
		out["max_length"] = maxSlackMessageRunes
	}
	return jsonResult(out), nil, nil
}

func (s *slackSession) reportStatus(ctx context.Context, _ *mcp.CallToolRequest, args slackReportStatusArgs) (*mcp.CallToolResult, any, error) {
	target, err := s.resolveThread(ctx, args.Channel, args.ThreadTS)
	if err != nil {
		return s.targetFailure(ctx, err), nil, nil
	}
	params := url.Values{
		"channel_id": {target.channel},
		"thread_ts":  {target.threadTS},
		"status":     {args.Status},
	}
	if len(args.LoadingMessages) > 0 {
		params.Set("loading_messages", jsonParam(args.LoadingMessages))
	}
	if err := s.api.call(ctx, "assistant.threads.setStatus", params, nil); err != nil {
		return errorResult(err.Error() + " Continue without status updates."), nil, nil
	}
	return jsonResult(map[string]any{"ok": true}), nil, nil
}

func (s *slackSession) react(ctx context.Context, _ *mcp.CallToolRequest, args slackReactArgs) (*mcp.CallToolResult, any, error) {
	name := strings.Trim(strings.TrimSpace(args.Reaction), ":")
	if name == "" {
		return errorResult("reaction must be an emoji name such as thumbsup."), nil, nil
	}
	channel := strings.TrimSpace(args.Channel)
	ts := strings.TrimSpace(args.TS)
	var target slackTarget
	var err error
	if s.pinThread {
		// Reactions take no thread argument, so resolve the pinned thread
		// itself and then check the message belongs to it.
		target, err = s.resolveTarget(ctx, "", "")
		if err == nil && channel != "" && channel != target.channel {
			err = toolErr("This Slack server only acts on the Slack thread this chat started from. Leave channel empty.")
		}
	} else {
		target, err = s.resolveTarget(ctx, channel, "")
	}
	if err != nil {
		return s.targetFailure(ctx, err), nil, nil
	}
	if ts == "" {
		ts = target.threadTS
	}
	if ts == "" {
		return errorResult("ts is required when channel is set."), nil, nil
	}
	if s.pinThread && ts != target.threadTS {
		inThread, err := s.messageInThread(ctx, target, ts)
		if err != nil {
			return errorResult(err.Error()), nil, nil
		}
		if !inThread {
			return errorResult("This Slack server only reacts to messages in the Slack thread this chat started from, and " + ts + " is not one of them."), nil, nil
		}
	}
	method, action := "reactions.add", "added"
	if args.Remove {
		method, action = "reactions.remove", "removed"
	}
	if err := s.api.call(ctx, method, url.Values{
		"channel":   {target.channel},
		"timestamp": {ts},
		"name":      {name},
	}, nil); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return jsonResult(map[string]any{"ok": true, "action": action, "reaction": name, "ts": ts}), nil, nil
}

// messageInThread reports whether the message ts is a reply in the
// target thread, asking Slack for exactly that message.
func (s *slackSession) messageInThread(ctx context.Context, target slackTarget, ts string) (bool, error) {
	var resp struct {
		Messages []struct {
			TS       string `json:"ts"`
			ThreadTS string `json:"thread_ts"`
		} `json:"messages"`
	}
	if err := s.api.call(ctx, "conversations.replies", url.Values{
		"channel":   {target.channel},
		"ts":        {target.threadTS},
		"oldest":    {ts},
		"latest":    {ts},
		"inclusive": {"true"},
		"limit":     {"2"},
	}, &resp); err != nil {
		return false, err
	}
	for _, m := range resp.Messages {
		if m.TS == ts && m.ThreadTS == target.threadTS {
			return true, nil
		}
	}
	return false, nil
}

func (s *slackSession) readThread(ctx context.Context, _ *mcp.CallToolRequest, args slackReadThreadArgs) (*mcp.CallToolResult, any, error) {
	target, err := s.resolveThread(ctx, args.Channel, args.ThreadTS)
	if err != nil {
		return s.targetFailure(ctx, err), nil, nil
	}
	limit := clamp(args.Limit, 50, 200)
	params := url.Values{
		"channel": {target.channel},
		"ts":      {target.threadTS},
		"limit":   {strconv.Itoa(limit)},
	}
	if args.Cursor != "" {
		params.Set("cursor", args.Cursor)
	}
	var resp struct {
		Messages         []json.RawMessage `json:"messages"`
		HasMore          bool              `json:"has_more"`
		ResponseMetadata struct {
			NextCursor string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	if err := s.api.call(ctx, "conversations.replies", params, &resp); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	out := map[string]any{"messages": resp.Messages, "has_more": resp.HasMore}
	if resp.Messages == nil {
		out["messages"] = []json.RawMessage{}
	}
	if resp.ResponseMetadata.NextCursor != "" {
		out["next_cursor"] = resp.ResponseMetadata.NextCursor
	}
	return jsonResult(out), nil, nil
}

func (s *slackSession) userInfo(ctx context.Context, _ *mcp.CallToolRequest, args slackUserInfoArgs) (*mcp.CallToolResult, any, error) {
	userID := strings.TrimSpace(args.UserID)
	if userID == "" {
		return errorResult("user_id is required."), nil, nil
	}
	var resp struct {
		User json.RawMessage `json:"user"`
	}
	if err := s.api.call(ctx, "users.info", url.Values{"user": {userID}}, &resp); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return jsonResult(resp.User), nil, nil
}

func (s *slackSession) search(ctx context.Context, _ *mcp.CallToolRequest, args slackSearchArgs) (*mcp.CallToolResult, any, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return errorResult("query is required."), nil, nil
	}
	var resp struct {
		Messages json.RawMessage `json:"messages"`
	}
	if err := s.api.call(ctx, "search.messages", url.Values{
		"query": {query},
		"count": {strconv.Itoa(clamp(args.Count, 10, 100))},
	}, &resp); err != nil {
		return errorResult(err.Error()), nil, nil
	}
	return jsonResult(resp.Messages), nil, nil
}

// toolError is a message for the model, returned verbatim as a tool error.
type toolError struct{ msg string }

func (e *toolError) Error() string { return e.msg }

func toolErr(msg string) error { return &toolError{msg: msg} }

// clamp returns def for a non-positive v and caps v at limit.
func clamp(v, def, limit int) int {
	if v <= 0 {
		return def
	}
	return min(v, limit)
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return errorResult("encode result: " + err.Error())
	}
	return textResult(string(b))
}
