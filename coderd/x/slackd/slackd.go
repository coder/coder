// Package slackd connects coderd to a Slack app over Socket Mode and
// submits Slack app mentions and direct messages to chats. It is the built-in
// counterpart of github.com/coder/coder-agents-slackbot. Incoming Slack events are
// reduced to message submission; replies to Slack happen through the
// Slack tools that chatd enables for chats carrying the slackd labels.
//
// Chats are per (Slack thread, owner): every message resolves the
// sender to a Coder user (the linked user through the configured
// external auth provider, or the configured fallback owner) and routes
// to that owner's chat for the thread, creating it on demand. A thread
// can therefore bind multiple chats, one per owner, but slackd keeps at
// most one of them actively generating: before a message is submitted
// to one owner's chat, every other chat bound to the same thread is
// interrupted (best-effort per sibling). The whole switch runs under a
// thread-scoped Postgres advisory lock so concurrent senders on
// different replicas cannot leave two chats generating.
//
// Each supported message submits a catch-up: the Slack thread is fetched and
// every message the chat has not yet seen is included in the user
// message, one <slack-message> block per Slack message. Ingestion is
// tracked through content-part metadata: every block part is stamped
// with slack_message_ts, and the newest stamped ts (the watermark)
// bounds later fetches. Replies the chat posted itself via the
// slack_send_message tool are excluded through slack_posted_message_ts
// metadata that chatd stamps on the tool-result part; they never
// advance the watermark.
//
// Every coderd replica runs its own Socket Mode connection, so the
// same Slack event can be delivered to multiple replicas. slackd
// deduplicates in two layers:
//
//   - Chat creation for a (thread, owner) pair is serialized through
//     chatd's DedupLabels support, which takes a Postgres advisory
//     transaction lock derived from the owner and the thread labels
//     and returns the existing chat when another replica won the race.
//   - Message submission stamps the Slack event id into the message
//     content metadata and asks chatd to reject the submission when a
//     message with the same event id already exists in the chat's
//     history or queue. The whole catch-up batch rides on one event
//     id, so a redelivered event cannot double-ingest thread messages.
package slackd

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
)

const (
	// LabelSlackd marks chats managed by slackd.
	LabelSlackd = chatd.LabelSlackd
	// LabelSlackThread stores the "<channel>:<thread_ts>" key that
	// binds a chat to a Slack thread.
	LabelSlackThread = chatd.LabelSlackThread
	// LabelSlackShared marks chats created for unlinked Slack senders
	// (shared mode). propose_mcp_server refuses in shared mode.
	LabelSlackShared = chatd.LabelSlackShared
	// MetadataKeySlackEventID is the content-part metadata key that
	// stores the unique Slack event id used for deduplication.
	MetadataKeySlackEventID = chatd.MetadataKeySlackEventID
	// MetadataKeySlackMessageTS is the content-part metadata key that
	// stores the Slack message timestamp of an ingested thread message.
	MetadataKeySlackMessageTS = chatd.MetadataKeySlackMessageTS
	// MetadataKeySlackPostedMessageTS is the content-part metadata key
	// that stores the Slack timestamp of a reply the chat posted via
	// the slack_send_message tool.
	MetadataKeySlackPostedMessageTS = chatd.MetadataKeySlackPostedMessageTS
	// MetadataKeySlackSenderID is the content-part metadata key that
	// stores the Slack sender id so chatd can resolve the requesting
	// Coder user at MCP proposal creation.
	MetadataKeySlackSenderID = chatd.MetadataKeySlackSenderID

	// Reconnection backoff bounds for the Socket Mode run loop.
	reconnectBackoffFloor = time.Second
	reconnectBackoffCeil  = time.Minute

	// apiKeyLifetime is the lifetime of the delegated API key slackd
	// mints for the chat owner; apiKeyRotateBefore is how long before
	// expiry a replacement is minted.
	apiKeyLifetime     = 30 * 24 * time.Hour
	apiKeyRotateBefore = 24 * time.Hour

	// Polling bounds while waiting for the thread-scoped advisory
	// lock held by another replica switching the thread's active
	// chat.
	threadLockRetryFloor = 10 * time.Millisecond
	threadLockRetryCeil  = time.Second
)

const systemPrompt = `You process messages forwarded from Slack by slackd,
Coder's built-in Slack integration. Each user message starts with Slack
thread metadata (channel, thread timestamp) followed by one or more
<slack-message></slack-message> blocks. Each block is one Slack message
from the thread, in chronological order, carrying nested
<timestamp-raw>, <timestamp>, <from-user>, and <content> tags. User mentions in the
content are rendered inline as @name, the way Slack displays them; use
slack_get_user_info or the <from-user> tags when you need a user's id.
A single user message may catch you up on several Slack messages at
once: everything said in the thread since the last message you received
is included.

You can reply to the Slack thread with the slack_* tools when they are
available. You must reply in-thread with slack_send_message when the sender
reached you from Slack - otherwise they won't see your reply.

Slack messages use mrkdwn, not standard markdown:
- *text* = bold, _text_ = italics, ~text~ = strikethrough
- <http://example.com|link text> = links
- user mentions must be <@USER_ID> (e.g. <@U01UBAM2C4D>), never @username
- never use headings (#) or double asterisks (**text**)
- keep replies concise; messages over 3000 characters are truncated

# MCP Servers

You can access external services via MCP servers. You may use existing
servers or create new ones via the "propose_mcp_server" tool.

"propose_mcp_server" proposes a **new** server. It only posts a confirmation
card to the thread; nothing is created until the requesting user accepts it on
a Coder page.

Before proposing a server:
- Always find a reliable source for its configuration, preferably the connected
  service's official MCP documentation.
- Keep in mind that you are proposing personal MCP servers, not deployment-wide ones.
  If an external service requires an admin to set up the MCP server, the user will
  likely have trouble setting it up. Some services may have multiple different ways
  to authenticate their MCP server: always prefer the one that the user can do themselves.
  For example, GitHub allows the MCP server to be configured with a personal access token
  or with a GitHub app - the personal access token is the easier, preferred option.
- Use that source to determine the endpoint, authentication, and transport.
- Never guess configuration from memory or an unverified source.
- Prefer "streamable_http" over "sse"; use "sse" only when a reliable source
  shows that streamable HTTP is unavailable.
- Prefer OAuth2 dynamic client registration when the server supports it. For
  static OAuth2 clients, use user_input wrappers for metadata the user must
  provide, including both the client ID and client secret when necessary.
- Every auth field accepts either a value or user_input wrapper. Mark user
  inputs sensitive when they contain secrets. Never ask users to paste
  credentials into Slack; they enter them on the Coder review page.
- Do not assume the user knows how the external service works or is configured.
  Never write vague directives like "ensure the Foo API is enabled"; instead
  name the exact page and action, e.g. "Ensure the Foo API is enabled: visit
  [this page](https://...) and confirm Foo is turned on."
- For manual OAuth2 (when not using dynamic client registration), the review
  page shows the OAuth2 redirect URI. Tell the user to copy it from the review
  page when registering the OAuth application.

Call the tool, then **end your turn** and wait for the "[system]" message
reporting the outcome.

Proposed servers are personal to the requesting user, not the whole deployment:
no other users can access them.

Be proactive about proposing new servers. If the user expresses interest in
using an external service and you do not already have an MCP server for it,
call propose_mcp_server immediately. Do not ask whether they would like you to
propose a server, whether they want help setting it up, or for permission to
proceed. Just propose it. The user can reject the proposal, so there is no
downside to proposing one right away.

# Shared and individual mode modes

You may be operating in either shared or individual mode.
- Shared mode: you're responding to a Slack user who is not linked to a Coder account.
- Individual mode: you're responding to a Slack user who is linked to a Coder account.

In shared mode, you only have deployment-wide resources: workspaces you create
are visible to everyone, and external services use global credentials.

In individual mode, you have access to the resources of the user you're responding to.

The modes are backed by different chat sessions: if user A started chatting with
you in individual mode, and then user B chimed in shared mode and you're responding to them,
you will not see the resources of user A, such as any MCP servers they configured.

Users may be confused by this: if it looks like you had access to some external services
when responding to user A, and then you're responding to user B and you don't have access
to those services, explain that it's likely user A has those services configured, but user B
doesn't.

{{ SystemPromptSuffix }}`

const systemPromptSuffixShared = `# User identity

The Slack user you're responding to (user id <@{{ SlackUserID }}>) is not linked
to a Coder account, so you are in shared mode.
You only have deployment-wide resources: workspaces you create
are visible to everyone, and external services use global credentials.

Do not do personal or long-running work in shared mode (for example writing
code and pushing it to GitHub). Ask the user to link their Coder account to
Slack first so you can act on their behalf.

In shared mode you can only use MCP servers that admins already configured.
Do not call propose_mcp_server; it will fail. If the user needs a service you
cannot reach, ask them to link their account so you can propose servers for
them.

Whenever you ask them to link their account:
- share {{ AccessURL }}/settings/external-auth - that's the link they need to visit
- ask them to ping you once they are connected`

const systemPromptSuffixIndividual = `# User identity

You have access to the resources of the user you're talking to on Slack: user id <@{{ SlackUserID }}>.
Any integrations that you have access to are scoped to that user. You're acting on their behalf.
Be responsible with that power. If you're about to do something potentially destructive, or
something that may share their data with others, ask them for confirmation first.`

const memorySystemPrompt = `# Memory

You wake up fresh each session. These memories are your continuity:

Daily notes: /daily/YYYY-MM-DD.md - raw logs of what happened
Long-term: /memory.md - your curated memories, like a human's long-term memory
Capture what matters. Decisions, context, things to remember. Skip the secrets unless asked to keep them.

If you don't look at your memories, conversation with the user will be disjointed.

## memory.md - Your Long-Term Memory

Write significant events, thoughts, decisions, opinions, lessons learned
This is your curated memory - the distilled essence, not raw logs

## Write It Down - No "Mental Notes"!

Memory is limited - if you want to remember something, WRITE IT TO A MEMORY
"Mental notes" don't survive session restarts. Files do.
When someone says "remember this" -> update /daily/YYYY-MM-DD.md or relevant memory
When you make a mistake -> document it so future-you doesn't repeat it
Text > Brain

## Use your memory

Before answering any factual question, recommendation, or request involving prior work, users, organizations, projects, integrations, or recent activity:
1. Search relevant user memories first using "search_memories".
2. Use the memory result as context, then verify externally only when freshness or accuracy requires it.
3. Do not browse the web or begin a new investigation until the memory search is complete.
4. Skip memory search only for simple computation, rewriting/translation, casual conversation, or when the user explicitly requests a clean-slate answer.
If memory and external sources differ, identify the discrepancy and prefer the source appropriate to the question.

The default place you should keep your memories is in "/memory.md".`

// ChatSubmitter is the subset of *chatd.Server used by slackd.
type ChatSubmitter interface {
	CreateChat(ctx context.Context, opts chatd.CreateOptions) (database.Chat, error)
	SendMessage(ctx context.Context, opts chatd.SendMessageOptions) (chatd.SendMessageResult, error)
	InterruptChat(ctx context.Context, chat database.Chat) (database.Chat, error)
}

// SocketClient is the subset of *socketmode.Client used by slackd.
// RunContext maintains the Socket Mode connection and returns on
// failure; EventsChannel delivers connection and Events API events;
// Ack acknowledges Events API requests so Slack does not redeliver.
type SocketClient interface {
	RunContext(ctx context.Context) error
	EventsChannel() <-chan socketmode.Event
	Ack(req socketmode.Request, payload ...any) error
}

// WebAPI is the subset of the Slack Web API used by slackd.
type WebAPI interface {
	AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error)
	GetUserInfoContext(ctx context.Context, user string) (*slack.User, error)
	GetConversationRepliesContext(ctx context.Context, params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error)
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
	PostEphemeralContext(ctx context.Context, channelID, userID string, options ...slack.MsgOption) (string, error)
}

// socketClientAdapter adapts *socketmode.Client to SocketClient.
type socketClientAdapter struct {
	*socketmode.Client
}

func (a socketClientAdapter) EventsChannel() <-chan socketmode.Event {
	return a.Events
}

// Options configures a slackd Server.
type Options struct {
	Logger   slog.Logger
	Database database.Store
	Chat     ChatSubmitter
	// ChatOwnerUserID is the Coder user that owns chats created from
	// Slack messages when the sender has no external auth link (or
	// when ExternalAuthProviderID is empty).
	ChatOwnerUserID uuid.UUID
	// ExternalAuthProviderID, when non-empty, is the ID of the Slack
	// external auth provider whose links map Slack sender ids to Coder
	// users. New Slack thread chats are then owned by the linked Coder
	// user; unlinked senders fall back to ChatOwnerUserID.
	ExternalAuthProviderID string
	// AccessURL is the deployment access URL, used in the shared-mode
	// system prompt so unlinked senders can be pointed at
	// /settings/external-auth to connect their Coder account.
	AccessURL *url.URL

	BotToken string
	AppToken string

	// SocketClient and WebAPI override the real Slack clients in
	// tests. When nil they are built from BotToken and AppToken.
	SocketClient SocketClient
	WebAPI       WebAPI

	// Proposals handles MCP server proposal state transitions. When
	// set, slackd routes Cancel button clicks (received over Socket
	// Mode) to it. The HTTP accept/reject endpoints are mounted by
	// coderd on the same instance.
	Proposals *ProposalsAPI
}

// Server runs the Slack Socket Mode listener. Use New followed by
// Start; Close stops the listener and waits for in-flight event
// handlers.
type Server struct {
	logger     slog.Logger
	db         database.Store
	chat       ChatSubmitter
	ownerID    uuid.UUID
	providerID string
	accessURL  *url.URL
	socket     SocketClient
	webAPI     WebAPI
	proposals  *ProposalsAPI

	closeCtx    context.Context
	closeCancel context.CancelFunc
	wg          sync.WaitGroup

	// Reconnection backoff bounds; fixed except in tests.
	backoffFloor time.Duration
	backoffCeil  time.Duration

	userCache sync.Map // slack user id -> *slack.User

	// mu protects botUID, the deployment bot identity returned by
	// Slack. Coder owner, organization, and delegated API key data are
	// intentionally not cached: they are resolved from the database
	// per Slack thread so ownership can vary between threads.
	mu     sync.Mutex
	botUID string
}

// New validates the options and returns an unstarted Server.
func New(opts Options) (*Server, error) {
	if opts.Database == nil {
		return nil, xerrors.New("slackd: database is required")
	}
	if opts.Chat == nil {
		return nil, xerrors.New("slackd: chat submitter is required")
	}
	if opts.ChatOwnerUserID == uuid.Nil {
		return nil, xerrors.New("slackd: chat owner user id is required")
	}
	socket := opts.SocketClient
	webAPI := opts.WebAPI
	if socket == nil || webAPI == nil {
		if opts.BotToken == "" || opts.AppToken == "" {
			return nil, xerrors.New("slackd: bot token and app token are required")
		}
		api := slack.New(opts.BotToken, slack.OptionAppLevelToken(opts.AppToken))
		if webAPI == nil {
			webAPI = api
		}
		if socket == nil {
			socket = socketClientAdapter{socketmode.New(api)}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		logger:       opts.Logger,
		db:           opts.Database,
		chat:         opts.Chat,
		ownerID:      opts.ChatOwnerUserID,
		providerID:   opts.ExternalAuthProviderID,
		accessURL:    opts.AccessURL,
		socket:       socket,
		webAPI:       webAPI,
		proposals:    opts.Proposals,
		closeCtx:     ctx,
		closeCancel:  cancel,
		backoffFloor: reconnectBackoffFloor,
		backoffCeil:  reconnectBackoffCeil,
	}, nil
}

// Start launches the event consumer and the Socket Mode connection
// loop. ctx carries the authorization identity (dbauthz.AsSlackd) for
// all database and chatd access; the loops stop when ctx is canceled
// or the server is closed.
func (s *Server) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.closeCtx, cancel)

	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		defer stop()
		defer cancel()
		s.runLoop(ctx)
	}()
	go func() {
		defer s.wg.Done()
		s.consumeEvents(ctx)
	}()
}

// Close stops the server and waits for in-flight work.
func (s *Server) Close() {
	s.closeCancel()
	s.wg.Wait()
}

// runLoop maintains the Socket Mode connection, reconnecting with
// exponential backoff. The Socket Mode client performs its own
// reconnects internally; this loop covers the cases where RunContext
// gives up and returns (e.g. invalid auth responses or repeated
// failures).
func (s *Server) runLoop(ctx context.Context) {
	r := retry.New(s.backoffFloor, s.backoffCeil)
	for {
		start := time.Now()
		err := s.socket.RunContext(ctx)
		if ctx.Err() != nil {
			return
		}
		s.logger.Warn(ctx, "slack socket mode connection ended, reconnecting", slog.Error(err))
		// A connection that survived for a while was healthy; only
		// quick failures should escalate the backoff.
		if time.Since(start) > s.backoffCeil {
			r.Reset()
		}
		if !r.Wait(ctx) {
			return
		}
	}
}

// consumeEvents dispatches Socket Mode events until ctx is canceled.
func (s *Server) consumeEvents(ctx context.Context) {
	events := s.socket.EventsChannel()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			s.handleEvent(ctx, evt)
		}
	}
}

func (s *Server) handleEvent(ctx context.Context, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnecting:
		s.logger.Info(ctx, "connecting to slack socket mode")
	case socketmode.EventTypeConnected:
		s.logger.Info(ctx, "slack socket mode connected")
	case socketmode.EventTypeConnectionError:
		s.logger.Warn(ctx, "slack socket mode connection error", slog.F("data", fmt.Sprintf("%v", evt.Data)))
	case socketmode.EventTypeInteractive:
		// Ack immediately; interactive payloads are handled
		// asynchronously and Slack retries unacked requests.
		if evt.Request != nil {
			if err := s.socket.Ack(*evt.Request); err != nil {
				s.logger.Warn(ctx, "acknowledge slack interactive event", slog.Error(err))
			}
		}
		callback, ok := evt.Data.(slack.InteractionCallback)
		if !ok || callback.Type != slack.InteractionTypeBlockActions {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleBlockActions(ctx, callback)
		}()
	case socketmode.EventTypeEventsAPI:
		// Ack immediately: Slack redelivers unacked events, and
		// redelivery is handled by event-id dedup anyway.
		if evt.Request != nil {
			if err := s.socket.Ack(*evt.Request); err != nil {
				s.logger.Warn(ctx, "acknowledge slack events api event", slog.Error(err))
			}
		}
		apiEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		callback, ok := apiEvent.Data.(*slackevents.EventsAPICallbackEvent)
		if !ok || callback.EventID == "" {
			s.logger.Warn(ctx, "events api event without event id, skipping")
			return
		}
		message, ok := s.normalizeIncomingMessage(ctx, apiEvent.InnerEvent.Data)
		if !ok {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.handleMessage(ctx, callback.EventID, message); err != nil && ctx.Err() == nil {
				s.logger.Error(ctx, "handle slack message",
					slog.F("event_id", callback.EventID),
					slog.F("channel", message.channel),
					slog.Error(err),
				)
			}
		}()
	}
}

// incomingMessage is the common subset of Slack app mentions and direct
// messages used by the chat submission pipeline.
type incomingMessage struct {
	user            string
	text            string
	timestamp       string
	threadTimestamp string
	channel         string
}

// normalizeIncomingMessage normalizes supported Events API payloads. Ordinary human
// messages in one-to-one direct message channels are accepted without an app
// mention. Other message events are ignored, including messages authored by
// the app itself, to prevent reply loops.
func (s *Server) normalizeIncomingMessage(ctx context.Context, event any) (incomingMessage, bool) {
	switch ev := event.(type) {
	case *slackevents.AppMentionEvent:
		return incomingMessageFromMention(ev), true
	case *slackevents.MessageEvent:
		if ev.ChannelType != "im" || ev.SubType != "" || ev.BotID != "" ||
			ev.User == "" || ev.Channel == "" || ev.TimeStamp == "" {
			return incomingMessage{}, false
		}
		botUID, err := s.resolveBotUserID(ctx)
		if err != nil {
			s.logger.Warn(ctx, "resolve slack bot user id for direct message", slog.Error(err))
			return incomingMessage{}, false
		}
		if ev.User == botUID {
			return incomingMessage{}, false
		}
		return incomingMessage{
			user:            ev.User,
			text:            ev.Text,
			timestamp:       ev.TimeStamp,
			threadTimestamp: ev.ThreadTimeStamp,
			channel:         ev.Channel,
		}, true
	default:
		return incomingMessage{}, false
	}
}

func incomingMessageFromMention(ev *slackevents.AppMentionEvent) incomingMessage {
	return incomingMessage{
		user:            ev.User,
		text:            ev.Text,
		timestamp:       ev.TimeStamp,
		threadTimestamp: ev.ThreadTimeStamp,
		channel:         ev.Channel,
	}
}

// handleMention submits an app mention through the shared Slack message path.
func (s *Server) handleMention(ctx context.Context, eventID string, ev *slackevents.AppMentionEvent) error {
	return s.handleMessage(ctx, eventID, incomingMessageFromMention(ev))
}

// handleMessage submits one supported Slack message to the sender's chat for
// the thread, creating the chat when the (thread, owner) pair is new.
// Other owners' chats bound to the same thread are interrupted first so
// at most one chat per thread is actively generating. The
// lookup-interrupt-submit sequence runs under a thread-scoped advisory
// lock (withThreadLock) so concurrent senders on different replicas
// cannot leave two chats generating.
func (s *Server) handleMessage(ctx context.Context, eventID string, ev incomingMessage) error {
	threadTS := ev.threadTimestamp
	if threadTS == "" {
		threadTS = ev.timestamp
	}
	threadKey := ev.channel + ":" + threadTS

	// Thread identity labels used for lookup, sibling interrupt, and
	// creation dedup. Mode labels (e.g. LabelSlackShared) are stamped
	// only on CreateChat so they do not affect matching.
	labels := map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: threadKey,
	}

	// Every message routes to the sender's own chat for the thread,
	// so the owner is resolved per message, not only for new threads.
	// Resolution keeps its fail-closed semantics: an error drops the
	// event (Slack redelivery plus event-id dedup make this safe).
	owner, kind, err := s.resolveOwner(ctx, ev.user)
	if err != nil {
		return xerrors.Errorf("resolve chat owner for slack user %q: %w", ev.user, err)
	}
	s.logger.Debug(ctx, "resolved chat owner for slack message",
		slog.F("thread", threadKey),
		slog.F("slack_user_id", ev.user),
		slog.F("owner_id", owner.ID),
		slog.F("resolution", string(kind)),
	)

	// The chat lookup, duplicate pre-check, sibling interruption, and
	// message submission form one critical section: without the thread
	// lock, two concurrent senders could each observe the other's chat
	// as idle (or nonexistent), both pass their interruption pass, and
	// both start generating.
	return s.withThreadLock(ctx, threadKey, func() error {
		chat, found, err := s.findChat(ctx, labels, owner.ID)
		if err != nil {
			return xerrors.Errorf("find chat for thread %q: %w", threadKey, err)
		}
		if found {
			// Redelivered events must not disturb the thread: without this
			// pre-check, a replay of an already-processed event would
			// interrupt the currently active sibling chat and then have its
			// SendMessage rejected as a duplicate, killing a legitimate
			// response without starting a replacement. The locked check
			// inside SendMessage stays authoritative.
			duplicate, err := s.isDuplicateEvent(ctx, chat.ID, eventID)
			if err != nil {
				return xerrors.Errorf("check duplicate slack event: %w", err)
			}
			if duplicate {
				s.logger.Debug(ctx, "duplicate slack event, skipping",
					slog.F("event_id", eventID), slog.F("chat_id", chat.ID))
				return nil
			}
		}

		// Everything said in the thread since the last ingested message
		// rides on this event, one <slack-message> block per unseen
		// Slack message. The unseen set is computed under the thread
		// lock so the watermark is consistent across replicas.
		unseen, err := s.unseenThreadMessages(ctx, ev, threadTS, uuid.NullUUID{UUID: chat.ID, Valid: found})
		if err != nil {
			return err
		}
		// Everything this event carries was already ingested through an
		// earlier event's catch-up (a late-delivered mention has a fresh
		// event id, so event-id dedup does not catch it). Submitting
		// would produce an empty catch-up and disturb the thread.
		if len(unseen) == 0 {
			s.logger.Debug(ctx, "no unseen slack messages for event, skipping",
				slog.F("event_id", eventID), slog.F("thread", threadKey))
			return nil
		}
		content := s.buildContent(ctx, ev, threadTS, eventID, unseen)

		// At most one chat per Slack thread generates at a time: other
		// owners' chats bound to this thread are interrupted before the
		// sender's chat receives the message (a newly created chat starts
		// generating immediately, so this also runs before CreateChat).
		s.interruptSiblingChats(ctx, labels, owner.ID)

		if !found {
			orgID, err := s.resolveOrganizationID(ctx, owner.ID)
			if err != nil {
				return xerrors.Errorf("resolve organization: %w", err)
			}
			apiKeyID, err := s.ensureAPIKeyID(ctx, owner.ID)
			if err != nil {
				return xerrors.Errorf("ensure api key: %w", err)
			}
			// Chats are created with the deployment default model, like
			// the HTTP create path when no model is specified.
			modelConfig, err := s.db.GetDefaultChatModelConfig(ctx)
			if err != nil {
				if xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.New("no default chat model config is configured")
				}
				return xerrors.Errorf("get default chat model config: %w", err)
			}
			createLabels := map[string]string{
				LabelSlackd:      labels[LabelSlackd],
				LabelSlackThread: labels[LabelSlackThread],
			}
			if kind == ownerResolutionFallback {
				createLabels[LabelSlackShared] = "true"
			}
			created, err := s.chat.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID:     orgID,
				OwnerID:            owner.ID,
				APIKeyID:           apiKeyID,
				ModelConfigID:      modelConfig.ID,
				Title:              "Slack thread " + threadKey,
				SystemPrompt:       s.buildSystemPrompt(ctx, kind, owner.ID, ev.user),
				InitialUserContent: content,
				Labels:             database.StringMap(createLabels),
				// Dedup is scoped per owner: each (owner, thread) pair
				// maps to exactly one chat even when replicas race.
				// Mode labels stay out of the dedup filter so shared and
				// individual resolutions for the same owner still collide.
				DedupLabels: labels,
			})
			switch {
			case err == nil:
				s.logger.Info(ctx, "created chat for slack thread",
					slog.F("chat_id", created.ID), slog.F("thread", threadKey),
					slog.F("owner_id", owner.ID))
				return nil
			case xerrors.Is(err, chatd.ErrChatAlreadyExists):
				// Another replica created this owner's chat for the
				// thread first. Fall through to message submission;
				// event-id dedup drops replays.
				chat = created
				s.logger.Info(ctx, "slack thread chat creation race resolved to existing chat",
					slog.F("chat_id", chat.ID), slog.F("thread", threadKey),
					slog.F("owner_id", chat.OwnerID))
			default:
				return xerrors.Errorf("create chat: %w", err)
			}
		} else {
			s.logger.Debug(ctx, "found existing chat for slack thread",
				slog.F("chat_id", chat.ID), slog.F("thread", threadKey),
				slog.F("owner_id", chat.OwnerID))
		}

		apiKeyID, err := s.ensureAPIKeyID(ctx, chat.OwnerID)
		if err != nil {
			return xerrors.Errorf("ensure api key: %w", err)
		}
		_, err = s.chat.SendMessage(ctx, chatd.SendMessageOptions{
			ChatID:           chat.ID,
			CreatedBy:        chat.OwnerID,
			APIKeyID:         apiKeyID,
			Content:          content,
			BusyBehavior:     chatd.SendMessageBusyBehaviorInterrupt,
			DedupMetadataKey: MetadataKeySlackEventID,
		})
		if xerrors.Is(err, chatd.ErrDuplicateMessage) {
			s.logger.Debug(ctx, "duplicate slack event, skipping",
				slog.F("event_id", eventID), slog.F("chat_id", chat.ID))
			return nil
		}
		if err != nil {
			return xerrors.Errorf("send message: %w", err)
		}
		s.logger.Info(ctx, "submitted slack message to chat",
			slog.F("chat_id", chat.ID), slog.F("event_id", eventID))
		return nil
	})
}

// ownerResolution describes how the owner of a new Slack thread chat
// was selected.
type ownerResolution string

const (
	// ownerResolutionLinked means the sender's Slack account is linked
	// to the returned Coder user through the configured external auth
	// provider.
	ownerResolutionLinked ownerResolution = "linked"
	// ownerResolutionFallback means the configured ChatOwnerUserID was
	// used, either because user mapping is disabled or because the
	// sender has no external auth link.
	ownerResolutionFallback ownerResolution = "fallback"
)

// resolveOwner maps the Slack sender to the Coder user that will own a
// new Slack thread chat. A sender linked through the configured
// external auth provider owns the chat; an unlinked sender falls back
// to the configured chat owner. Ambiguous mappings (multiple linked
// Coder accounts) and unusable linked users fail closed instead of
// falling back.
func (s *Server) resolveOwner(ctx context.Context, slackUserID string) (database.User, ownerResolution, error) {
	return resolveSlackSender(ctx, s.logger, s.db, s.providerID, s.ownerID, slackUserID)
}

// NewSlackUserResolver returns a resolver mapping Slack user ids to
// Coder user ids using the same resolution as slackd's chat-owner
// routing (external auth link, or the configured fallback owner). The
// MCP proposal Cancel flow uses it to authorize clicks against the
// proposal's requester.
func NewSlackUserResolver(logger slog.Logger, db database.Store, providerID string, fallbackOwnerID uuid.UUID) func(ctx context.Context, slackUserID string) (uuid.UUID, error) {
	return func(ctx context.Context, slackUserID string) (uuid.UUID, error) {
		// The resolver is shared with chatd tools, whose context carries the
		// narrower chatd identity. Slack user mapping requires slackd's user
		// read permission regardless of which caller invokes the resolver.
		ctx = dbauthz.AsSlackd(ctx) //nolint:gocritic // Slack identity resolution is internal slackd work.
		user, _, err := resolveSlackSender(ctx, logger, db, providerID, fallbackOwnerID, slackUserID)
		if err != nil {
			return uuid.Nil, err
		}
		return user.ID, nil
	}
}

// resolveSlackSender implements Server.resolveOwner as a package
// function so the MCP proposal Cancel flow can reuse it.
func resolveSlackSender(ctx context.Context, logger slog.Logger, db database.Store, providerID string, fallbackOwnerID uuid.UUID, slackUserID string) (database.User, ownerResolution, error) {
	if providerID == "" || slackUserID == "" {
		owner, err := fallbackOwner(ctx, db, fallbackOwnerID)
		return owner, ownerResolutionFallback, err
	}
	users, err := db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
		ProviderID:     providerID,
		ExternalUserID: slackUserID,
	})
	if err != nil {
		return database.User{}, "", xerrors.Errorf("look up linked users: %w", err)
	}
	switch len(users) {
	case 0:
		logger.Debug(ctx, "slack sender has no linked coder user, using fallback chat owner",
			slog.F("slack_user_id", slackUserID))
		owner, err := fallbackOwner(ctx, db, fallbackOwnerID)
		return owner, ownerResolutionFallback, err
	case 1:
		owner := users[0]
		if err := validateOwner(owner); err != nil {
			logger.Warn(ctx, "slack sender is linked to an unusable coder user",
				slog.F("slack_user_id", slackUserID),
				slog.F("owner_id", owner.ID),
				slog.Error(err))
			return database.User{}, "", xerrors.Errorf("linked coder user %s: %w", owner.ID, err)
		}
		return owner, ownerResolutionLinked, nil
	default:
		// Never pick among multiple linked accounts, even when only
		// one of them is active: selection would depend on row order
		// or status filtering rather than the user's intent.
		logger.Warn(ctx, "slack sender is linked to multiple coder users, failing closed",
			slog.F("slack_user_id", slackUserID),
			slog.F("linked_users", len(users)))
		return database.User{}, "", xerrors.Errorf("slack user %q is linked to %d coder users; refusing to pick one", slackUserID, len(users))
	}
}

// fallbackOwner loads and validates the configured chat owner. The
// fallback owner passes through the same usability checks as a linked
// owner.
func fallbackOwner(ctx context.Context, db database.Store, ownerID uuid.UUID) (database.User, error) {
	owner, err := db.GetUserByID(ctx, ownerID)
	if err != nil {
		return database.User{}, xerrors.Errorf("get configured chat owner %s: %w", ownerID, err)
	}
	if err := validateOwner(owner); err != nil {
		return database.User{}, xerrors.Errorf("configured chat owner %s: %w", owner.ID, err)
	}
	return owner, nil
}

// validateOwner rejects users that must not own Slack thread chats:
// deleted, non-active (suspended or dormant), and system users.
func validateOwner(user database.User) error {
	switch {
	case user.Deleted:
		return xerrors.New("user is deleted")
	case user.IsSystem:
		return xerrors.New("user is a system user")
	case user.Status != database.UserStatusActive:
		return xerrors.Errorf("user status is %q, not active", user.Status)
	}
	return nil
}

// findChat returns the oldest non-archived chat carrying the given
// labels that is owned by ownerID. The label listing is cross-owner;
// the owner filter is applied in Go so each sender routes to their own
// chat for the thread.
func (s *Server) findChat(ctx context.Context, labels map[string]string, ownerID uuid.UUID) (database.Chat, bool, error) {
	filter, err := json.Marshal(labels)
	if err != nil {
		return database.Chat{}, false, xerrors.Errorf("marshal label filter: %w", err)
	}
	chats, err := s.db.GetChatsByLabels(ctx, filter)
	if err != nil {
		return database.Chat{}, false, err
	}
	for _, chat := range chats {
		if chat.OwnerID == ownerID {
			return chat, true, nil
		}
	}
	return database.Chat{}, false, nil
}

// isDuplicateEvent reports whether the chat already carries a message
// stamped with the given Slack event id in its history or queue, using
// the same content-metadata containment filter as chatd's SendMessage
// dedup.
func (s *Server) isDuplicateEvent(ctx context.Context, chatID uuid.UUID, eventID string) (bool, error) {
	filter, err := json.Marshal([]map[string]map[string]string{
		{"metadata": {MetadataKeySlackEventID: eventID}},
	})
	if err != nil {
		return false, xerrors.Errorf("marshal dedup content filter: %w", err)
	}
	return s.db.ChatMessageExistsWithContentMetadata(ctx, database.ChatMessageExistsWithContentMetadataParams{
		ChatID:        chatID,
		ContentFilter: filter,
	})
}

// withThreadLock runs fn while holding a Postgres advisory transaction
// lock scoped to the Slack thread, serializing the
// lookup-interrupt-submit sequence across replicas so at most one chat
// per thread starts generating at a time. The lock lives in a
// dedicated transaction on its own connection; fn's database and chatd
// work runs outside that transaction and commits independently. The
// lock is polled with TryAcquireLock so a canceled context stops the
// wait.
//
// KNOWN ISSUE: this design can deadlock the database connection pool.
// The lock-holding transaction pins one pool connection for the whole
// critical section while fn needs further connections for its own
// queries and chatd transactions. With enough concurrent mentions
// (e.g. the default pool of 10 connections), lock holders and waiters
// can occupy every connection, leaving no free connection for any fn
// to make progress, so no holder ever commits and releases. InTx also
// begins its transaction with context.Background(), so cancellation
// does not reliably break a wait for the initial connection. Fixing
// this requires a design that does not reserve an application-pool
// connection while the critical section runs on others, such as a
// separate bounded lock pool or an atomic database-backed claim/queue.
func (s *Server) withThreadLock(ctx context.Context, threadKey string, fn func() error) error {
	lockID := database.GenLockID("slackd:thread:" + threadKey)
	return s.db.InTx(func(tx database.Store) error {
		r := retry.New(threadLockRetryFloor, threadLockRetryCeil)
		for {
			acquired, err := tx.TryAcquireLock(ctx, lockID)
			if err != nil {
				return xerrors.Errorf("acquire slack thread lock: %w", err)
			}
			if acquired {
				break
			}
			if !r.Wait(ctx) {
				return ctx.Err()
			}
		}
		return fn()
	}, nil)
}

// interruptSiblingChats interrupts every non-archived chat carrying
// the thread labels that is owned by someone other than ownerID, so at
// most one chat per Slack thread is actively generating. Interruption
// is best-effort: failures are logged and never block message
// delivery. Idle siblings (chatstate.ErrTransitionNotAllowed) and
// concurrently deleted ones (chatstate.ErrChatNotFound) are expected
// and ignored.
func (s *Server) interruptSiblingChats(ctx context.Context, labels map[string]string, ownerID uuid.UUID) {
	filter, err := json.Marshal(labels)
	if err != nil {
		s.logger.Warn(ctx, "marshal label filter for sibling chat interrupt", slog.Error(err))
		return
	}
	chats, err := s.db.GetChatsByLabels(ctx, filter)
	if err != nil {
		s.logger.Warn(ctx, "list sibling chats for interrupt", slog.Error(err))
		return
	}
	for _, sibling := range chats {
		if sibling.OwnerID == ownerID {
			continue
		}
		_, err := s.chat.InterruptChat(ctx, sibling)
		switch {
		case err == nil:
			s.logger.Info(ctx, "interrupted sibling slack thread chat",
				slog.F("chat_id", sibling.ID), slog.F("owner_id", sibling.OwnerID))
		case xerrors.Is(err, chatstate.ErrTransitionNotAllowed), xerrors.Is(err, chatstate.ErrChatNotFound):
			// The sibling is idle or already gone; nothing to interrupt.
		default:
			s.logger.Warn(ctx, "interrupt sibling slack thread chat",
				slog.F("chat_id", sibling.ID), slog.Error(err))
		}
	}
}

// ensureAPIKeyID returns the id of a delegated API key owned by
// ownerID, minting a new one when no reusable key exists. User chat
// messages require an API key id: the AI Gateway attributes LLM usage
// of the generation turn to it. The key is looked up in the database
// on every call; nothing owner-specific is cached in process memory.
func (s *Server) ensureAPIKeyID(ctx context.Context, ownerID uuid.UUID) (string, error) {
	return ensureAPIKeyID(ctx, s.db, ownerID)
}

// ensureAPIKeyID implements Server.ensureAPIKeyID as a package
// function so the MCP proposal handlers can reuse it.
func ensureAPIKeyID(ctx context.Context, db database.Store, ownerID uuid.UUID) (string, error) {
	keys, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
		LoginType:      database.LoginTypeToken,
		UserID:         ownerID,
		IncludeExpired: false,
	})
	if err != nil {
		return "", xerrors.Errorf("get api keys for chat owner: %w", err)
	}
	// Only slackd-minted keys with enough remaining lifetime are
	// reusable; unrelated user tokens are never touched.
	reusable := keys[:0:0]
	for _, key := range keys {
		if strings.HasPrefix(key.TokenName, "slackd-") && time.Until(key.ExpiresAt) > apiKeyRotateBefore {
			reusable = append(reusable, key)
		}
	}
	if len(reusable) > 0 {
		// Deterministic selection: latest expiry first, key id as the
		// tie breaker, so replicas converge on the same key.
		sort.Slice(reusable, func(i, j int) bool {
			if !reusable[i].ExpiresAt.Equal(reusable[j].ExpiresAt) {
				return reusable[i].ExpiresAt.After(reusable[j].ExpiresAt)
			}
			return reusable[i].ID < reusable[j].ID
		})
		return reusable[0].ID, nil
	}
	params, _, err := apikey.Generate(apikey.CreateParams{
		UserID:          ownerID,
		LoginType:       database.LoginTypeToken,
		LifetimeSeconds: int64(apiKeyLifetime / time.Second),
	})
	if err != nil {
		return "", xerrors.Errorf("generate api key: %w", err)
	}
	// Token names are unique per user and concurrent replicas may
	// mint keys simultaneously, so include the random key id in the
	// name. A later event deterministically picks among them.
	params.TokenName = "slackd-" + params.ID
	inserted, err := db.InsertAPIKey(ctx, params)
	if err != nil {
		return "", xerrors.Errorf("insert api key: %w", err)
	}
	return inserted.ID, nil
}

// resolveBotUserID caches the bot's own Slack user id, used to tell
// the model how it appears in Slack messages.
func (s *Server) resolveBotUserID(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.botUID != "" {
		return s.botUID, nil
	}
	resp, err := s.webAPI.AuthTestContext(ctx)
	if err != nil {
		return "", err
	}
	s.botUID = resp.UserID
	return s.botUID, nil
}

// buildSystemPrompt selects the shared or individual identity suffix
// from the sender's owner resolution, interpolates deployment and
// Slack identity placeholders, and loads long-term memory for linked
// users. It then appends the bot's own Slack identity so the model can
// recognize inline @bot-name mentions as referring to itself. When
// the bot identity cannot be resolved the rest of the prompt is
// returned unchanged.
func (s *Server) buildSystemPrompt(ctx context.Context, kind ownerResolution, ownerID uuid.UUID, slackUserID string) string {
	suffix := systemPromptSuffixShared
	if kind == ownerResolutionLinked {
		suffix = systemPromptSuffixIndividual
	}
	accessURL := ""
	if s.accessURL != nil {
		accessURL = strings.TrimRight(s.accessURL.String(), "/")
	}
	suffix = strings.NewReplacer(
		"{{ AccessURL }}", accessURL,
		"{{ SlackUserID }}", slackUserID,
	).Replace(suffix)
	prompt := strings.ReplaceAll(systemPrompt, "{{ SystemPromptSuffix }}", suffix)
	if kind == ownerResolutionLinked {
		prompt += "\n\n" + memorySystemPrompt
		memory, err := s.db.GetAgentMemoryByUserIDAndPath(slackMemoryContext(ctx, ownerID), database.GetAgentMemoryByUserIDAndPathParams{
			UserID: ownerID,
			Path:   "/memory.md",
		})
		if err == nil {
			prompt += "\n\n## Current memory.md\n\n<memory.md>\n" + memory.Content + "\n</memory.md>"
		} else if !xerrors.Is(err, sql.ErrNoRows) {
			s.logger.Warn(ctx, "load slack user long-term memory", slog.Error(err), slog.F("owner_id", ownerID))
		}
	}

	botUID, err := s.resolveBotUserID(ctx)
	if err != nil {
		s.logger.Warn(ctx, "resolve slack bot user id for system prompt", slog.Error(err))
		return prompt
	}
	botName := ""
	if user := s.lookupUser(ctx, botUID); user != nil {
		botName = user.Name
	}
	if botName == "" {
		return prompt + fmt.Sprintf("\n\nYour Slack user id is <@%s>. "+
			"Mentions of it in message content refer to you.", botUID)
	}
	return prompt + fmt.Sprintf("\n\nYou appear in Slack as @%s (user id <@%s>). "+
		"When @%s shows up in a message's content, the sender is addressing you.",
		botName, botUID, botName)
}

func slackMemoryContext(ctx context.Context, userID uuid.UUID) context.Context {
	actor := rbac.Subject{
		Type:  rbac.SubjectTypeUser,
		ID:    userID.String(),
		Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()
	// Slack turns run asynchronously, so the linked user's request actor is
	// unavailable when slackd creates the chat. Memory authorization remains
	// restricted to the linked owner by the user-scoped resource check.
	//nolint:gocritic // The synthetic actor is required for asynchronous turns.
	return dbauthz.As(ctx, actor)
}

// resolveOrganizationID selects the organization for a chat created
// under ownerID: the owner's default organization, or their only
// organization membership. The result is not cached, so membership
// and default-organization changes are observed by later messages.
func (s *Server) resolveOrganizationID(ctx context.Context, ownerID uuid.UUID) (uuid.UUID, error) {
	orgs, err := s.db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  ownerID,
		Deleted: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get organizations for chat owner: %w", err)
	}
	if len(orgs) == 0 {
		return uuid.Nil, xerrors.Errorf("chat owner %s belongs to no organization", ownerID)
	}
	for _, org := range orgs {
		if org.IsDefault {
			return org.ID, nil
		}
	}
	if len(orgs) > 1 {
		return uuid.Nil, xerrors.Errorf("chat owner %s belongs to %d organizations and none is the default", ownerID, len(orgs))
	}
	return orgs[0].ID, nil
}

// lookupUser fetches and caches a Slack user profile. Failures are
// non-fatal; the message falls back to raw user ids.
func (s *Server) lookupUser(ctx context.Context, id string) *slack.User {
	if cached, ok := s.userCache.Load(id); ok {
		user, _ := cached.(*slack.User)
		return user
	}
	user, err := s.webAPI.GetUserInfoContext(ctx, id)
	if err != nil {
		s.logger.Warn(ctx, "slack user lookup failed", slog.F("user", id), slog.Error(err))
		return nil
	}
	s.userCache.Store(id, user)
	return user
}

// buildMessageBody renders the body of a <slack-message> block:
// nested <timestamp-raw>, <timestamp>, <from-user>, and <content> tags. User
// mentions in the content are rendered inline as @name, the way Slack
// displays them.
func (s *Server) buildMessageBody(ctx context.Context, msg slack.Message) string {
	sender := s.lookupUser(ctx, msg.User)
	senderName, senderRealName := msg.User, ""
	if sender != nil {
		senderName = sender.Name
		senderRealName = sender.RealName
		if senderRealName == "" {
			senderRealName = sender.Profile.DisplayName
		}
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "<timestamp-raw>%s</timestamp-raw>\n", msg.Timestamp)
	if timestamp, ok := formatSlackTimestamp(msg.Timestamp); ok {
		_, _ = fmt.Fprintf(&sb, "<timestamp>%s</timestamp>\n", timestamp)
	}
	_, _ = fmt.Fprintf(&sb,
		"<from-user>%s (<@%s>) (%s)</from-user>\n"+
			"<content>\n%s\n</content>\n",
		senderName, msg.User, senderRealName,
		strings.TrimRight(s.renderMentionsInline(ctx, msg.Text), "\n"))
	return sb.String()
}

func formatSlackTimestamp(raw string) (string, bool) {
	secondsRaw, fractionRaw, _ := strings.Cut(raw, ".")
	seconds, err := strconv.ParseInt(secondsRaw, 10, 64)
	if err != nil || seconds < 0 || len(fractionRaw) > 9 {
		return "", false
	}
	for _, r := range fractionRaw {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	fractionRaw += strings.Repeat("0", 9-len(fractionRaw))
	nanoseconds, err := strconv.ParseInt(fractionRaw, 10, 64)
	if err != nil {
		return "", false
	}
	return time.Unix(seconds, nanoseconds).UTC().Format(time.RFC3339Nano), true
}

// renderMentionsInline replaces <@ID> mention tokens with @name, the
// way Slack displays them. Tokens whose user lookup fails are left
// as-is so the id is not lost.
func (s *Server) renderMentionsInline(ctx context.Context, text string) string {
	return mentionPattern.ReplaceAllStringFunc(text, func(match string) string {
		id := mentionPattern.FindStringSubmatch(match)[1]
		if user := s.lookupUser(ctx, id); user != nil && user.Name != "" {
			return "@" + user.Name
		}
		return match
	})
}

// buildContent renders the user message submitted to the chat: a
// header part carrying the thread metadata and the triggering event id
// (part 0), followed by one <slack-message> block part per unseen
// Slack message, in timestamp order. Each block part is stamped with
// slack_message_ts so later events can compute the unseen set.
func (s *Server) buildContent(
	ctx context.Context,
	ev incomingMessage,
	threadTS, eventID string,
	unseen []slack.Message,
) []codersdk.ChatMessagePart {
	threadLine := threadTS
	if ev.threadTimestamp == "" {
		threadLine = "N/A (new thread)"
	}
	header := fmt.Sprintf("Slack thread metadata:\n\n"+
		"Channel ID: %s\nThread Timestamp: %s\n\n"+
		"The slack-message blocks below are the messages from this Slack "+
		"thread that have not been submitted to this chat yet, in "+
		"chronological order.\n",
		ev.channel, threadLine)

	parts := []codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: header,
		Metadata: map[string]string{
			MetadataKeySlackEventID:  eventID,
			MetadataKeySlackSenderID: ev.user,
		},
	}}
	for _, msg := range unseen {
		parts = append(parts, codersdk.ChatMessagePart{
			Type:     codersdk.ChatMessagePartTypeText,
			Text:     "<slack-message>\n" + s.buildMessageBody(ctx, msg) + "</slack-message>\n",
			Metadata: map[string]string{MetadataKeySlackMessageTS: msg.Timestamp},
		})
	}
	return parts
}

// unseenThreadMessages fetches the Slack thread and returns the
// messages the chat has not ingested yet, in timestamp order. Seen is
// defined by the watermark (the newest slack_message_ts already in the
// chat) plus the exclusion set of replies the chat posted itself
// (slack_posted_message_ts). A null chatID means the chat does not
// exist yet and the whole thread is unseen. The fetch is
// authoritative: a mention at or below the watermark was already
// ingested by an earlier event's catch-up, so a late-delivered event
// (which carries a fresh event id and passes event-id dedup) must not
// re-include it. Only on fetch failure is the mention synthesized from
// the event payload and returned alone, so a Slack API hiccup never
// drops the event.
func (s *Server) unseenThreadMessages(
	ctx context.Context,
	ev incomingMessage,
	threadTS string,
	chatID uuid.NullUUID,
) ([]slack.Message, error) {
	mention := slack.Message{Msg: slack.Msg{
		Timestamp: ev.timestamp,
		User:      ev.user,
		Text:      ev.text,
	}}
	replies, err := s.fetchThreadReplies(ctx, ev.channel, threadTS)
	if err != nil {
		s.logger.Warn(ctx, "fetch slack thread replies, falling back to event-only submission",
			slog.F("channel", ev.channel), slog.F("thread_ts", threadTS), slog.Error(err))
		return []slack.Message{mention}, nil
	}

	// A new chat has no history: the whole thread so far is unseen,
	// including messages from before the bot was first mentioned.
	var watermark string
	posted := map[string]struct{}{}
	if chatID.Valid {
		watermark, posted, err = s.loadSeenState(ctx, chatID.UUID)
		if err != nil {
			return nil, xerrors.Errorf("load slack ingestion state: %w", err)
		}
	}

	unseen := make([]slack.Message, 0, len(replies))
	for _, msg := range replies {
		if msg.Timestamp == "" || slackTSCompare(msg.Timestamp, watermark) <= 0 {
			continue
		}
		if _, ok := posted[msg.Timestamp]; ok {
			continue
		}
		unseen = append(unseen, msg)
	}
	sort.SliceStable(unseen, func(i, j int) bool {
		return slackTSCompare(unseen[i].Timestamp, unseen[j].Timestamp) < 0
	})
	return unseen, nil
}

// fetchThreadReplies pages through the Slack thread and returns every
// message in it, with no sender filtering: bot and app messages are
// included because sibling chats bound to the same thread post replies
// this chat has never seen.
func (s *Server) fetchThreadReplies(ctx context.Context, channel, threadTS string) ([]slack.Message, error) {
	var all []slack.Message
	cursor := ""
	for {
		msgs, hasMore, nextCursor, err := s.webAPI.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
			ChannelID: channel,
			Timestamp: threadTS,
			Cursor:    cursor,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, msgs...)
		if !hasMore {
			return all, nil
		}
		cursor = nextCursor
	}
}

// loadSeenState returns the chat's ingestion watermark (the newest
// slack_message_ts across its non-deleted and queued messages) and the
// exclusion set of reply timestamps the chat posted itself via
// slack_send_message. Posted timestamps never advance the watermark: a
// reply posted at ts T while a human message at T-1 is still unseen
// must not skip that message.
func (s *Server) loadSeenState(ctx context.Context, chatID uuid.UUID) (string, map[string]struct{}, error) {
	seen, err := s.db.GetChatContentMetadataValues(ctx, database.GetChatContentMetadataValuesParams{
		ChatID:      chatID,
		MetadataKey: MetadataKeySlackMessageTS,
	})
	if err != nil {
		return "", nil, xerrors.Errorf("get ingested slack message timestamps: %w", err)
	}
	watermark := ""
	for _, ts := range seen {
		if watermark == "" || slackTSCompare(ts, watermark) > 0 {
			watermark = ts
		}
	}
	postedValues, err := s.db.GetChatContentMetadataValues(ctx, database.GetChatContentMetadataValuesParams{
		ChatID:      chatID,
		MetadataKey: MetadataKeySlackPostedMessageTS,
	})
	if err != nil {
		return "", nil, xerrors.Errorf("get posted slack message timestamps: %w", err)
	}
	posted := make(map[string]struct{}, len(postedValues))
	for _, ts := range postedValues {
		posted[ts] = struct{}{}
	}
	return watermark, posted, nil
}

// slackTSCompare orders Slack message timestamps ("seconds.suffix")
// numerically: both segments are parsed as integers because
// lexicographic order breaks across digit-count boundaries. An empty
// or malformed timestamp sorts before every well-formed one.
func slackTSCompare(a, b string) int {
	aSec, aSuf, aOK := parseSlackTS(a)
	bSec, bSuf, bOK := parseSlackTS(b)
	if !aOK || !bOK {
		switch {
		case aOK == bOK:
			return strings.Compare(a, b)
		case aOK:
			return 1
		default:
			return -1
		}
	}
	if aSec != bSec {
		return cmp.Compare(aSec, bSec)
	}
	return cmp.Compare(aSuf, bSuf)
}

// parseSlackTS splits a Slack timestamp into its integer seconds and
// suffix segments. The suffix is optional ("123" parses as 123.0).
func parseSlackTS(ts string) (secs int64, suffix int64, ok bool) {
	if ts == "" {
		return 0, 0, false
	}
	secPart, sufPart, hasSuffix := strings.Cut(ts, ".")
	secs, err := strconv.ParseInt(secPart, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if !hasSuffix {
		return secs, 0, true
	}
	suffix, err = strconv.ParseInt(sufPart, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return secs, suffix, true
}

// mentionPattern matches Slack user mentions like <@U0123ABC>.
var mentionPattern = regexp.MustCompile(`<@([A-Z0-9]+)>`)

// extractMentions returns the unique Slack user ids mentioned in text,
// in order of first appearance.
func extractMentions(text string) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, match := range mentionPattern.FindAllStringSubmatch(text, -1) {
		id := match[1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
