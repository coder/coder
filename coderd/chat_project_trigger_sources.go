package coderd

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// maxChatProjectTriggerSourcePayloadBytes bounds GitHub and Slack
	// request bodies. They are summarized rather than copied into the chat,
	// so they may be larger than plain webhook payloads.
	maxChatProjectTriggerSourcePayloadBytes = 1 << 20
	maxChatProjectTriggerEvents             = 32
	// maxChatProjectTriggerTextChars bounds free text, such as issue
	// bodies, copied from an event into the chat.
	maxChatProjectTriggerTextChars    = 4000
	maxChatProjectTriggerCommits      = 20
	maxChatProjectTriggerSenders      = 32
	maxChatProjectTriggerPatternChars = 512
)

var (
	chatProjectTriggerGitHubEventPattern = regexp.MustCompile(`^[a-z_]+(\.[a-z_]+)?$`)
	chatProjectTriggerRepositoryPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	chatProjectTriggerChannelPattern     = regexp.MustCompile(`^[A-Z0-9]+$`)
	chatProjectTriggerSlackUserPattern   = regexp.MustCompile(`^[UW][A-Z0-9]+$`)
	chatProjectTriggerGitHubLoginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\[bot\])?$`)
	chatProjectTriggerSlackEvents        = []string{
		codersdk.ChatProjectTriggerSlackEventAppMention,
		codersdk.ChatProjectTriggerSlackEventMessage,
		codersdk.ChatProjectTriggerSlackEventSlashCommand,
	}
)

// chatProjectTriggerFilter is the event filter of a trigger.
type chatProjectTriggerFilter struct {
	events     []string
	repository string
	channel    string
	text       codersdk.ChatProjectTriggerTextFilter
}

// normalizeChatProjectTriggerFilter validates the event filter for a
// source. It returns the field and message of the first invalid value.
func normalizeChatProjectTriggerFilter(source codersdk.ChatProjectTriggerSource, input chatProjectTriggerFilter) (filter chatProjectTriggerFilter, field string, message string) {
	filter = chatProjectTriggerFilter{
		events:     uniqueTrimmed(input.events),
		repository: strings.TrimSpace(input.repository),
		channel:    strings.TrimSpace(input.channel),
		text: codersdk.ChatProjectTriggerTextFilter{
			TextPattern:         strings.TrimSpace(input.text.TextPattern),
			TextPatternIsRegex:  input.text.TextPatternIsRegex,
			IgnoreThreadReplies: input.text.IgnoreThreadReplies,
			Senders:             uniqueTrimmed(input.text.Senders),
		},
	}
	if filter.text.TextPattern == "" {
		filter.text.TextPatternIsRegex = false
	}
	if utf8.RuneCountInString(filter.text.TextPattern) > maxChatProjectTriggerPatternChars {
		return filter, "text_pattern", fmt.Sprintf("Text filter must be at most %d characters.", maxChatProjectTriggerPatternChars)
	}
	if filter.text.TextPatternIsRegex {
		if _, err := regexp.Compile(filter.text.TextPattern); err != nil {
			return filter, "text_pattern", fmt.Sprintf("Text filter is not a valid regular expression: %s.", err)
		}
	}
	if filter.text.IgnoreThreadReplies && source != codersdk.ChatProjectTriggerSourceSlack {
		return filter, "ignore_thread_replies", "Only Slack triggers can keep thread replies from starting chats."
	}
	if len(filter.text.Senders) > maxChatProjectTriggerSenders {
		return filter, "senders", fmt.Sprintf("A trigger can be limited to at most %d people.", maxChatProjectTriggerSenders)
	}
	if len(filter.events) > maxChatProjectTriggerEvents {
		return filter, "events", fmt.Sprintf("A trigger can have at most %d events.", maxChatProjectTriggerEvents)
	}
	if filter.repository != "" && source != codersdk.ChatProjectTriggerSourceGitHub {
		return filter, "repository", "Only GitHub triggers can be limited to a repository."
	}
	if filter.channel != "" && source != codersdk.ChatProjectTriggerSourceSlack {
		return filter, "channel", "Only Slack triggers can be limited to a channel."
	}

	switch source {
	case codersdk.ChatProjectTriggerSourceWebhook:
		if len(filter.events) > 0 {
			return filter, "events", "Webhook triggers start a chat for every call and have no events."
		}
		if len(filter.text.Senders) > 0 {
			return filter, "senders", "Webhook triggers cannot be limited to people."
		}
	case codersdk.ChatProjectTriggerSourceGitHub:
		for _, event := range filter.events {
			if !chatProjectTriggerGitHubEventPattern.MatchString(event) {
				return filter, "events", fmt.Sprintf("%q is not a GitHub event. Use an event name such as push or pull_request.opened.", event)
			}
		}
		if filter.repository != "" && !chatProjectTriggerRepositoryPattern.MatchString(filter.repository) {
			return filter, "repository", "Repository must be in the form owner/name."
		}
		for _, sender := range filter.text.Senders {
			if !chatProjectTriggerGitHubLoginPattern.MatchString(sender) {
				return filter, "senders", fmt.Sprintf("%q is not a GitHub username.", sender)
			}
		}
	case codersdk.ChatProjectTriggerSourceSlack:
		for _, event := range filter.events {
			if !slices.Contains(chatProjectTriggerSlackEvents, event) {
				return filter, "events", fmt.Sprintf("%q is not a Slack trigger event. Use one of: %s.", event, strings.Join(chatProjectTriggerSlackEvents, ", "))
			}
		}
		if filter.channel != "" && !chatProjectTriggerChannelPattern.MatchString(filter.channel) {
			return filter, "channel", "Channel must be a Slack channel ID, such as C0123456789."
		}
		for _, sender := range filter.text.Senders {
			if !chatProjectTriggerSlackUserPattern.MatchString(sender) {
				return filter, "senders", fmt.Sprintf("%q is not a Slack user ID. Use an ID such as U0123456789.", sender)
			}
		}
	case codersdk.ChatProjectTriggerSourceSchedule:
		if len(filter.events) > 0 {
			return filter, "events", "Schedule triggers start a chat on their schedule and have no events."
		}
		if len(filter.text.Senders) > 0 {
			return filter, "senders", "Schedule triggers cannot be limited to people."
		}
		if filter.text.TextPattern != "" {
			return filter, "text_pattern", "Schedule triggers have no text to filter."
		}
	default:
		return filter, "source", "Source must be webhook, github, slack, or schedule."
	}
	return filter, "", ""
}

func uniqueTrimmed(values []string) []string {
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// chatProjectTriggerSenderIgnores returns why the trigger's sender filter
// skips an event, or "" when the sender may start or continue a chat.
func chatProjectTriggerSenderIgnores(trigger database.ChatProjectTrigger, event chatProjectTriggerEvent) string {
	if len(trigger.Senders) > 0 && !slices.ContainsFunc(trigger.Senders, func(sender string) bool {
		return strings.EqualFold(sender, event.sender)
	}) {
		return "The trigger does not listen to this sender."
	}
	return ""
}

// chatProjectTriggerTextFilterIgnores returns why the trigger's text filter
// skips an event, or "" when the event may start a chat.
func chatProjectTriggerTextFilterIgnores(trigger database.ChatProjectTrigger, event chatProjectTriggerEvent) string {
	if trigger.TextPattern == "" {
		return ""
	}
	if trigger.TextPatternIsRegex {
		// The pattern was validated when it was saved.
		re, err := regexp.Compile(trigger.TextPattern)
		if err != nil || !re.MatchString(event.text) {
			return "The event text does not match the trigger's filter."
		}
		return ""
	}
	if !strings.Contains(strings.ToLower(event.text), strings.ToLower(trigger.TextPattern)) {
		return "The event text does not match the trigger's filter."
	}
	return ""
}

// chatProjectTriggerEvent is what a webhook request asks the trigger to do.
type chatProjectTriggerEvent struct {
	// reply, when set, answers the request without starting a chat, such as
	// for GitHub pings and Slack URL verification.
	reply func(rw http.ResponseWriter)
	// replyDescription records what reply answered.
	replyDescription string
	// name is the event name, such as issue_comment.created.
	name string
	// ignored, when set, explains why the event does not start a chat.
	ignored string
	// duplicate marks a repeat of an event already received, such as a
	// Slack retry. Duplicates are expected noise and are not recorded as
	// deliveries.
	duplicate bool
	// title is the chat title. Empty uses the trigger name.
	title string
	// summary describes the event to the agent.
	summary string
	// details is event data shown to the agent as untrusted JSON.
	details map[string]any
	// slashCommand is set for Slack slash commands, which expect a message
	// in the response.
	slashCommand bool
	// text and sender are what the trigger's text filter matches against.
	text   string
	sender string
	// threadReply is set for Slack messages that reply in a thread.
	threadReply bool
	// conversationKey identifies the conversation the event belongs to:
	// team/channel/thread_ts for Slack, owner/repo#number for GitHub
	// issues and pull requests, or the X-Coder-Conversation-Id header for
	// plain webhooks. Empty when the event has no conversation.
	conversationKey string
	// eventKeys identify the delivery, so retries and other events for the
	// same message are handled once.
	eventKeys []string
	// mentionsBot is set for Slack messages that mention the app.
	mentionsBot bool
	// followUpOnly is set for Slack thread replies the trigger only
	// handles because they may continue a chat, such as message events
	// when the trigger listens for app mentions.
	followUpOnly bool
}

func chatProjectTriggerEventMatches(events []string, event, action string) bool {
	if len(events) == 0 {
		return true
	}
	return slices.Contains(events, event) || (action != "" && slices.Contains(events, event+"."+action))
}

// parseGitHubTriggerEvent interprets a GitHub webhook delivery.
func parseGitHubTriggerEvent(trigger database.ChatProjectTrigger, r *http.Request, body []byte) (chatProjectTriggerEvent, error) {
	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		return chatProjectTriggerEvent{}, xerrors.New("missing X-GitHub-Event header: configure this URL as a GitHub webhook")
	}
	if event == "ping" {
		return chatProjectTriggerEvent{name: event, replyDescription: "Answered the GitHub ping.", reply: func(rw http.ResponseWriter) {
			writeChatProjectTriggerReply(rw, map[string]string{"message": "pong"})
		}}, nil
	}

	// GitHub sends either JSON or a form with a payload field.
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return chatProjectTriggerEvent{}, xerrors.Errorf("parse form payload: %w", err)
		}
		body = []byte(values.Get("payload"))
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return chatProjectTriggerEvent{}, xerrors.Errorf("payload is not a JSON object: %w", err)
	}

	action := jsonString(payload, "action")
	eventName := event
	if action != "" {
		eventName += "." + action
	}
	if !chatProjectTriggerEventMatches(trigger.Events, event, action) {
		return chatProjectTriggerEvent{name: eventName, ignored: fmt.Sprintf("The trigger does not listen for %s.", eventName)}, nil
	}
	repository := jsonString(payload, "repository", "full_name")
	if repository != "" && !githubRepositoryPattern.MatchString(repository) {
		return chatProjectTriggerEvent{name: eventName, ignored: "The event's repository is not a valid owner/name."}, nil
	}
	// GitHub redelivers events with the same delivery ID, so each is
	// handled once.
	var eventKeys []string
	if delivery := r.Header.Get("X-GitHub-Delivery"); delivery != "" {
		if !githubDeliveryPattern.MatchString(delivery) {
			return chatProjectTriggerEvent{name: eventName, ignored: "The X-GitHub-Delivery header is not a valid delivery ID."}, nil
		}
		eventKeys = append(eventKeys, "delivery:"+delivery)
	}
	if trigger.Repository != "" && !strings.EqualFold(trigger.Repository, repository) {
		return chatProjectTriggerEvent{name: eventName, ignored: fmt.Sprintf("The event came from %s, but the trigger only listens to %s.", repository, trigger.Repository)}, nil
	}

	details := map[string]any{
		"event":      event,
		"action":     action,
		"repository": repository,
		"sender":     jsonString(payload, "sender", "login"),
	}
	where := ""
	if repository != "" {
		where = " in " + repository
	}
	summary := fmt.Sprintf("GitHub sent a %s event%s.", eventName, where)
	title := ""
	text := ""

	switch event {
	case "pull_request", "pull_request_review", "pull_request_review_comment":
		pr := jsonObject(payload, "pull_request")
		prDetails := githubIssueDetails(pr)
		prDetails["base"] = jsonString(pr, "base", "ref")
		prDetails["head"] = jsonString(pr, "head", "ref")
		prDetails["merged"] = jsonValue(pr, "merged")
		details["pull_request"] = prDetails
		if review := jsonObject(payload, "review"); review != nil {
			details["review"] = map[string]any{
				"state": jsonString(review, "state"),
				"body":  truncateTriggerText(jsonString(review, "body")),
				"url":   jsonString(review, "html_url"),
			}
		}
		if comment := jsonObject(payload, "comment"); comment != nil {
			details["comment"] = githubCommentDetails(comment)
		}
		title = githubIssueTitle(repository, pr)
		summary = fmt.Sprintf("GitHub sent a %s event for pull request %s.", eventName, githubIssueRef(repository, pr))
		text = githubEventText(payload, pr)
	case "issues", "issue_comment":
		issue := jsonObject(payload, "issue")
		details["issue"] = githubIssueDetails(issue)
		if label := jsonObject(payload, "label"); label != nil {
			details["label"] = jsonString(label, "name")
		}
		if comment := jsonObject(payload, "comment"); comment != nil {
			details["comment"] = githubCommentDetails(comment)
		}
		kind := "issue"
		if jsonObject(issue, "pull_request") != nil {
			kind = "pull request"
		}
		title = githubIssueTitle(repository, issue)
		summary = fmt.Sprintf("GitHub sent a %s event for %s %s.", eventName, kind, githubIssueRef(repository, issue))
		text = githubEventText(payload, issue)
	case "push":
		ref := jsonString(payload, "ref")
		commits := make([]map[string]any, 0)
		if list, ok := payload["commits"].([]any); ok {
			for _, item := range list {
				if len(commits) == maxChatProjectTriggerCommits {
					break
				}
				commit, _ := item.(map[string]any)
				text += jsonString(commit, "message") + "\n"
				commits = append(commits, map[string]any{
					"id":      jsonString(commit, "id"),
					"message": truncateTriggerText(jsonString(commit, "message")),
					"author":  jsonString(commit, "author", "name"),
					"url":     jsonString(commit, "url"),
				})
			}
		}
		details["push"] = map[string]any{
			"ref":     ref,
			"before":  jsonString(payload, "before"),
			"after":   jsonString(payload, "after"),
			"compare": jsonString(payload, "compare"),
			"commits": commits,
		}
		title = fmt.Sprintf("Push to %s%s", strings.TrimPrefix(ref, "refs/heads/"), where)
		summary = fmt.Sprintf("GitHub sent a push event for %s%s.", ref, where)
	case "release":
		release := jsonObject(payload, "release")
		details["release"] = map[string]any{
			"tag":  jsonString(release, "tag_name"),
			"name": jsonString(release, "name"),
			"url":  jsonString(release, "html_url"),
			"body": truncateTriggerText(jsonString(release, "body")),
		}
		title = fmt.Sprintf("Release %s%s", jsonString(release, "tag_name"), where)
		text = jsonString(release, "name") + "\n" + jsonString(release, "body")
	case "workflow_run":
		run := jsonObject(payload, "workflow_run")
		details["workflow_run"] = map[string]any{
			"name":       jsonString(run, "name"),
			"status":     jsonString(run, "status"),
			"conclusion": jsonString(run, "conclusion"),
			"branch":     jsonString(run, "head_branch"),
			"commit":     jsonString(run, "head_sha"),
			"url":        jsonString(run, "html_url"),
		}
		title = fmt.Sprintf("Workflow %s %s", jsonString(run, "name"), jsonString(run, "conclusion"))
		text = jsonString(run, "name")
	default:
		// Unknown events keep their payload so the agent can still act on
		// them, bounded like plain webhook payloads.
		if len(body) <= maxChatProjectTriggerPayloadBytes {
			details["payload"] = payload
		}
		text = string(body)
	}
	return chatProjectTriggerEvent{
		name:            eventName,
		title:           strings.TrimSpace(title),
		summary:         summary,
		details:         details,
		text:            text,
		sender:          jsonString(payload, "sender", "login"),
		conversationKey: githubConversationKey(event, repository, payload),
		eventKeys:       eventKeys,
	}, nil
}

// githubConversationKey returns owner/repo#number, lowercased, for events on
// an issue or pull request, or "" for other events. Comments, reviews, and
// changes to one issue or pull request share a key.
func githubConversationKey(event, repository string, payload map[string]any) string {
	var number any
	switch event {
	case "issues", "issue_comment":
		number = jsonValue(payload, "issue", "number")
	case "pull_request", "pull_request_review", "pull_request_review_comment":
		number = jsonValue(payload, "pull_request", "number")
	default:
		return ""
	}
	n, ok := number.(float64)
	if !ok || repository == "" {
		return ""
	}
	return strings.ToLower(repository) + "#" + strconv.FormatFloat(n, 'f', -1, 64)
}

// parseSlackTriggerEvent interprets a Slack Events API request or slash
// command.
func parseSlackTriggerEvent(trigger database.ChatProjectTrigger, r *http.Request, body []byte) (chatProjectTriggerEvent, error) {
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return chatProjectTriggerEvent{}, xerrors.Errorf("parse slash command: %w", err)
		}
		if values.Get("command") == "" {
			return chatProjectTriggerEvent{}, xerrors.New("form payload is not a Slack slash command")
		}
		if !chatProjectTriggerEventMatches(trigger.Events, codersdk.ChatProjectTriggerSlackEventSlashCommand, "") {
			return chatProjectTriggerEvent{name: codersdk.ChatProjectTriggerSlackEventSlashCommand, ignored: "The trigger does not listen for slash commands.", slashCommand: true}, nil
		}
		if !validSlackID(values.Get("team_id"), true) || !validSlackID(values.Get("channel_id"), true) ||
			(values.Get("trigger_id") != "" && !slackTriggerIDPattern.MatchString(values.Get("trigger_id"))) {
			return chatProjectTriggerEvent{name: codersdk.ChatProjectTriggerSlackEventSlashCommand, ignored: malformedSlackIDs, slashCommand: true}, nil
		}
		if trigger.Channel != "" && trigger.Channel != values.Get("channel_id") {
			return chatProjectTriggerEvent{name: codersdk.ChatProjectTriggerSlackEventSlashCommand, ignored: "The trigger does not listen to this channel.", slashCommand: true}, nil
		}
		// Each run of a command has its own trigger_id, so a repeated
		// request for one run is handled once.
		var eventKeys []string
		if triggerID := values.Get("trigger_id"); triggerID != "" {
			eventKeys = append(eventKeys, "slash:"+triggerID)
		}
		text := truncateTriggerText(values.Get("text"))
		title := strings.TrimSpace(values.Get("command") + " " + firstLine(text))
		return chatProjectTriggerEvent{
			name:         codersdk.ChatProjectTriggerSlackEventSlashCommand,
			title:        title,
			summary:      fmt.Sprintf("Someone ran the %s Slack slash command.", values.Get("command")),
			slashCommand: true,
			eventKeys:    eventKeys,
			text:         values.Get("text"),
			sender:       values.Get("user_id"),
			details: map[string]any{
				"event":        codersdk.ChatProjectTriggerSlackEventSlashCommand,
				"command":      values.Get("command"),
				"text":         text,
				"user_id":      values.Get("user_id"),
				"user_name":    values.Get("user_name"),
				"channel_id":   values.Get("channel_id"),
				"channel_name": values.Get("channel_name"),
				"team_domain":  values.Get("team_domain"),
			},
		}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return chatProjectTriggerEvent{}, xerrors.Errorf("payload is not a Slack event: %w", err)
	}
	switch jsonString(payload, "type") {
	case "url_verification":
		challenge := jsonString(payload, "challenge")
		return chatProjectTriggerEvent{name: "url_verification", replyDescription: "Verified the Slack request URL.", reply: func(rw http.ResponseWriter) {
			writeChatProjectTriggerReply(rw, map[string]string{"challenge": challenge})
		}}, nil
	case "event_callback":
	default:
		return chatProjectTriggerEvent{}, xerrors.Errorf("unsupported Slack request type %q", jsonString(payload, "type"))
	}
	// Slack retries deliveries it thinks timed out. Retries carry the same
	// event ID as the original, which the handler uses to handle each event
	// once. A retry without an ID cannot be matched, so it is ignored.
	eventID := jsonString(payload, "event_id")
	if eventID != "" && !slackEventIDPattern.MatchString(eventID) {
		return chatProjectTriggerEvent{ignored: malformedSlackIDs}, nil
	}
	if r.Header.Get("X-Slack-Retry-Num") != "" && eventID == "" {
		return chatProjectTriggerEvent{ignored: "Slack retried an event without an event ID, so it cannot be told apart from the original.", duplicate: true}, nil
	}
	event := jsonObject(payload, "event")
	eventType := jsonString(event, "type")
	user := jsonString(event, "user")
	botUserID := slackBotUserID(payload)
	// Bot messages, edits, and joins have a subtype. Ignoring them, and
	// messages from the app's own user, keeps an agent that posts to Slack
	// from triggering itself.
	if jsonString(event, "bot_id") != "" || jsonString(event, "subtype") != "" || (botUserID != "" && user == botUserID) {
		return chatProjectTriggerEvent{name: eventType, ignored: "Messages from bots, including this app, and message changes do not start or continue chats."}, nil
	}
	ts := jsonString(event, "ts")
	threadTS := jsonString(event, "thread_ts")
	if !validSlackID(jsonString(payload, "team_id"), true) || !validSlackID(jsonString(event, "channel"), true) ||
		!validSlackTS(ts) || !validSlackTS(threadTS) {
		return chatProjectTriggerEvent{name: eventType, ignored: malformedSlackIDs}, nil
	}
	threadReply := threadTS != "" && threadTS != ts
	followUpOnly := false
	if !chatProjectTriggerEventMatches(trigger.Events, eventType, "") {
		// Replies in a thread arrive as message events. With conversations
		// on, they can continue the thread's chat even when the trigger
		// only starts chats for mentions.
		if !trigger.ContinueConversations || !threadReply || eventType != codersdk.ChatProjectTriggerSlackEventMessage {
			return chatProjectTriggerEvent{name: eventType, ignored: fmt.Sprintf("The trigger does not listen for %s.", eventType)}, nil
		}
		followUpOnly = true
	}
	channel := jsonString(event, "channel")
	if trigger.Channel != "" && trigger.Channel != channel {
		return chatProjectTriggerEvent{name: eventType, ignored: fmt.Sprintf("The message was in %s, but the trigger only listens to %s.", channel, trigger.Channel)}, nil
	}
	rawText := jsonString(event, "text")
	text := truncateTriggerText(rawText)
	summary := "Someone posted a Slack message."
	if eventType == codersdk.ChatProjectTriggerSlackEventAppMention {
		summary = "Someone mentioned the app in Slack."
	}
	teamID := jsonString(payload, "team_id")
	var eventKeys []string
	if eventID != "" {
		eventKeys = append(eventKeys, "event:"+eventID)
	}
	if ts != "" {
		// A mention arrives as both a message and an app_mention event
		// with different IDs but the same message.
		eventKeys = append(eventKeys, "message:"+channel+":"+ts)
	}
	return chatProjectTriggerEvent{
		name:        eventType,
		title:       firstLine(text),
		summary:     summary,
		text:        rawText,
		sender:      user,
		threadReply: threadReply,
		mentionsBot: eventType == codersdk.ChatProjectTriggerSlackEventAppMention ||
			(botUserID != "" && strings.Contains(rawText, "<@"+botUserID+">")),
		followUpOnly:    followUpOnly,
		conversationKey: slackConversationKey(teamID, channel, ts, threadTS),
		eventKeys:       eventKeys,
		details: map[string]any{
			"event":     eventType,
			"text":      text,
			"user_id":   user,
			"channel":   channel,
			"ts":        ts,
			"thread_ts": threadTS,
			"team_id":   teamID,
		},
	}, nil
}

// Conversation and event keys are built from IDs in the payload, so they are
// checked to be the shape GitHub and Slack use before they are stored.
var (
	// githubRepositoryPattern matches owner/name; GitHub caps owners at 39
	// characters and repository names at 100.
	githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)
	githubDeliveryPattern   = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	slackIDPattern          = regexp.MustCompile(`^[A-Z0-9]{1,32}$`)
	slackTSPattern          = regexp.MustCompile(`^[0-9]{1,12}\.[0-9]{1,9}$`)
	slackEventIDPattern     = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
	slackTriggerIDPattern   = regexp.MustCompile(`^[A-Za-z0-9.]{1,128}$`)
)

const malformedSlackIDs = "The Slack request has a malformed team, channel, event, or message ID."

// validSlackID reports whether id looks like a Slack team or channel ID.
// An empty id is valid only when optional.
func validSlackID(id string, optional bool) bool {
	if id == "" {
		return optional
	}
	return slackIDPattern.MatchString(id)
}

// validSlackTS reports whether ts is empty or a Slack message timestamp.
func validSlackTS(ts string) bool {
	return ts == "" || slackTSPattern.MatchString(ts)
}

// slackConversationKey returns team/channel/thread_ts, where a message that
// is not in a thread uses its own ts, the ts its replies' thread_ts will
// have.
func slackConversationKey(teamID, channel, ts, threadTS string) string {
	thread := threadTS
	if thread == "" {
		thread = ts
	}
	if channel == "" || thread == "" {
		return ""
	}
	return teamID + "/" + channel + "/" + thread
}

// slackBotUserID returns the user ID of the app the event was delivered to,
// so the app's own messages can be told apart.
func slackBotUserID(payload map[string]any) string {
	list, _ := payload["authorizations"].([]any)
	for _, item := range list {
		auth, _ := item.(map[string]any)
		if id := jsonString(auth, "user_id"); id != "" {
			return id
		}
	}
	return ""
}

func writeChatProjectTriggerReply(rw http.ResponseWriter, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(rw).Encode(body)
}

func githubIssueDetails(issue map[string]any) map[string]any {
	labels := make([]string, 0)
	if list, ok := issue["labels"].([]any); ok {
		for _, item := range list {
			if label, ok := item.(map[string]any); ok {
				labels = append(labels, jsonString(label, "name"))
			}
		}
	}
	return map[string]any{
		"number": jsonValue(issue, "number"),
		"title":  jsonString(issue, "title"),
		"state":  jsonString(issue, "state"),
		"author": jsonString(issue, "user", "login"),
		"url":    jsonString(issue, "html_url"),
		"body":   truncateTriggerText(jsonString(issue, "body")),
		"labels": labels,
	}
}

func githubCommentDetails(comment map[string]any) map[string]any {
	return map[string]any{
		"author": jsonString(comment, "user", "login"),
		"body":   truncateTriggerText(jsonString(comment, "body")),
		"url":    jsonString(comment, "html_url"),
	}
}

// githubEventText is the text a GitHub issue or pull request event is
// filtered on: the comment or review when there is one, otherwise the title
// and body.
func githubEventText(payload, issue map[string]any) string {
	if comment := jsonObject(payload, "comment"); comment != nil {
		return jsonString(comment, "body")
	}
	if review := jsonObject(payload, "review"); review != nil {
		return jsonString(review, "body")
	}
	return jsonString(issue, "title") + "\n" + jsonString(issue, "body")
}

func githubIssueRef(repository string, issue map[string]any) string {
	return fmt.Sprintf("%s#%v", repository, jsonValue(issue, "number"))
}

func githubIssueTitle(repository string, issue map[string]any) string {
	if issue == nil {
		return ""
	}
	return fmt.Sprintf("%s %s", githubIssueRef(repository, issue), jsonString(issue, "title"))
}

func jsonValue(m map[string]any, path ...string) any {
	var v any = m
	for _, key := range path {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = obj[key]
	}
	return v
}

func jsonString(m map[string]any, path ...string) string {
	s, _ := jsonValue(m, path...).(string)
	return s
}

func jsonObject(m map[string]any, path ...string) map[string]any {
	obj, _ := jsonValue(m, path...).(map[string]any)
	return obj
}

func truncateTriggerText(s string) string {
	if utf8.RuneCountInString(s) <= maxChatProjectTriggerTextChars {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxChatProjectTriggerTextChars]) + "\n[truncated]"
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
