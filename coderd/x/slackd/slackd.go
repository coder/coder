// Package slackd connects coderd to a Slack app over Socket Mode and
// submits Slack app mentions to chats. It is the built-in counterpart
// of github.com/coder/coder-agents-slackbot. Incoming Slack events are
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
//     history or queue.
package slackd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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
	// MetadataKeySlackEventID is the content-part metadata key that
	// stores the unique Slack event id used for deduplication.
	MetadataKeySlackEventID = chatd.MetadataKeySlackEventID

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
Coder's built-in Slack integration. Each user message contains Slack
metadata (channel, timestamps, sender) followed by the message content.

You can reply to the Slack thread with the slack_* tools when they are
available. You must reply in-thread with slack_send_message when the sender
reached you from Slack - otherwise they won't see your reply.

Slack messages use mrkdwn, not standard markdown:
- *text* = bold, _text_ = italics, ~text~ = strikethrough
- <http://example.com|link text> = links
- user mentions must be <@USER_ID> (e.g. <@U01UBAM2C4D>), never @username
- never use headings (#) or double asterisks (**text**)
- keep replies concise; messages over 3000 characters are truncated`

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

// UserInfoAPI is the subset of the Slack Web API used by slackd.
type UserInfoAPI interface {
	AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error)
	GetUserInfoContext(ctx context.Context, user string) (*slack.User, error)
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

	BotToken string
	AppToken string

	// SocketClient and UserInfoAPI override the real Slack clients in
	// tests. When nil they are built from BotToken and AppToken.
	SocketClient SocketClient
	UserInfoAPI  UserInfoAPI
}

// Server runs the Slack Socket Mode listener. Use New followed by
// Start; Close stops the listener and waits for in-flight event
// handlers.
type Server struct {
	logger      slog.Logger
	db          database.Store
	chat        ChatSubmitter
	ownerID     uuid.UUID
	providerID  string
	socket      SocketClient
	userInfoAPI UserInfoAPI

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
	userInfoAPI := opts.UserInfoAPI
	if socket == nil || userInfoAPI == nil {
		if opts.BotToken == "" || opts.AppToken == "" {
			return nil, xerrors.New("slackd: bot token and app token are required")
		}
		api := slack.New(opts.BotToken, slack.OptionAppLevelToken(opts.AppToken))
		if userInfoAPI == nil {
			userInfoAPI = api
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
		socket:       socket,
		userInfoAPI:  userInfoAPI,
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
		mention, ok := apiEvent.InnerEvent.Data.(*slackevents.AppMentionEvent)
		if !ok {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := s.handleMention(ctx, callback.EventID, mention); err != nil && ctx.Err() == nil {
				s.logger.Error(ctx, "handle slack mention",
					slog.F("event_id", callback.EventID),
					slog.F("channel", mention.Channel),
					slog.Error(err),
				)
			}
		}()
	}
}

// handleMention submits one Slack app mention to the sender's chat for
// the thread, creating the chat when the (thread, owner) pair is new.
// Other owners' chats bound to the same thread are interrupted first so
// at most one chat per thread is actively generating. The
// lookup-interrupt-submit sequence runs under a thread-scoped advisory
// lock (withThreadLock) so concurrent senders on different replicas
// cannot leave two chats generating.
func (s *Server) handleMention(ctx context.Context, eventID string, ev *slackevents.AppMentionEvent) error {
	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}
	threadKey := ev.Channel + ":" + threadTS

	botUID, err := s.resolveBotUserID(ctx)
	if err != nil {
		s.logger.Warn(ctx, "resolve slack bot user id", slog.Error(err))
	}
	text := ev.Text
	if botUID != "" {
		text = strings.ReplaceAll(text, fmt.Sprintf("<@%s>", botUID), "")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "Hello!"
	}

	message := s.buildMessage(ctx, ev, botUID, text, threadTS)
	labels := map[string]string{
		LabelSlackd:      "true",
		LabelSlackThread: threadKey,
	}
	content := []codersdk.ChatMessagePart{{
		Type:     codersdk.ChatMessagePartTypeText,
		Text:     message,
		Metadata: map[string]string{MetadataKeySlackEventID: eventID},
	}}

	// Every message routes to the sender's own chat for the thread,
	// so the owner is resolved per message, not only for new threads.
	// Resolution keeps its fail-closed semantics: an error drops the
	// event (Slack redelivery plus event-id dedup make this safe).
	owner, kind, err := s.resolveOwner(ctx, ev.User)
	if err != nil {
		return xerrors.Errorf("resolve chat owner for slack user %q: %w", ev.User, err)
	}
	s.logger.Debug(ctx, "resolved chat owner for slack message",
		slog.F("thread", threadKey),
		slog.F("slack_user_id", ev.User),
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
			created, err := s.chat.CreateChat(ctx, chatd.CreateOptions{
				OrganizationID:     orgID,
				OwnerID:            owner.ID,
				APIKeyID:           apiKeyID,
				ModelConfigID:      modelConfig.ID,
				Title:              "Slack thread " + threadKey,
				SystemPrompt:       systemPrompt,
				InitialUserContent: content,
				Labels:             database.StringMap(labels),
				// Dedup is scoped per owner: each (owner, thread) pair
				// maps to exactly one chat even when replicas race.
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
	if s.providerID == "" || slackUserID == "" {
		owner, err := s.fallbackOwner(ctx)
		return owner, ownerResolutionFallback, err
	}
	users, err := s.db.GetUsersByExternalAuthProviderUserID(ctx, database.GetUsersByExternalAuthProviderUserIDParams{
		ProviderID:     s.providerID,
		ExternalUserID: slackUserID,
	})
	if err != nil {
		return database.User{}, "", xerrors.Errorf("look up linked users: %w", err)
	}
	switch len(users) {
	case 0:
		s.logger.Debug(ctx, "slack sender has no linked coder user, using fallback chat owner",
			slog.F("slack_user_id", slackUserID))
		owner, err := s.fallbackOwner(ctx)
		return owner, ownerResolutionFallback, err
	case 1:
		owner := users[0]
		if err := validateOwner(owner); err != nil {
			s.logger.Warn(ctx, "slack sender is linked to an unusable coder user",
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
		s.logger.Warn(ctx, "slack sender is linked to multiple coder users, failing closed",
			slog.F("slack_user_id", slackUserID),
			slog.F("linked_users", len(users)))
		return database.User{}, "", xerrors.Errorf("slack user %q is linked to %d coder users; refusing to pick one", slackUserID, len(users))
	}
}

// fallbackOwner loads and validates the configured chat owner. The
// fallback owner passes through the same usability checks as a linked
// owner.
func (s *Server) fallbackOwner(ctx context.Context) (database.User, error) {
	owner, err := s.db.GetUserByID(ctx, s.ownerID)
	if err != nil {
		return database.User{}, xerrors.Errorf("get configured chat owner %s: %w", s.ownerID, err)
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
	keys, err := s.db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
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
	inserted, err := s.db.InsertAPIKey(ctx, params)
	if err != nil {
		return "", xerrors.Errorf("insert api key: %w", err)
	}
	return inserted.ID, nil
}

// resolveBotUserID caches the bot's own Slack user id, used to strip
// the bot mention from message text.
func (s *Server) resolveBotUserID(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.botUID != "" {
		return s.botUID, nil
	}
	resp, err := s.userInfoAPI.AuthTestContext(ctx)
	if err != nil {
		return "", err
	}
	s.botUID = resp.UserID
	return s.botUID, nil
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
	user, err := s.userInfoAPI.GetUserInfoContext(ctx, id)
	if err != nil {
		s.logger.Warn(ctx, "slack user lookup failed", slog.F("user", id), slog.Error(err))
		return nil
	}
	s.userCache.Store(id, user)
	return user
}

// buildMessage renders the user message submitted to the chat: Slack
// metadata followed by the message content and resolved mentions.
func (s *Server) buildMessage(ctx context.Context, ev *slackevents.AppMentionEvent, botUID, text, threadTS string) string {
	sender := s.lookupUser(ctx, ev.User)
	senderName, senderRealName := ev.User, ""
	if sender != nil {
		senderName = sender.Name
		senderRealName = sender.RealName
		if senderRealName == "" {
			senderRealName = sender.Profile.DisplayName
		}
	}
	threadLine := threadTS
	if ev.ThreadTimeStamp == "" {
		threadLine = "N/A (new thread)"
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Slack message metadata:\n\n"+
		"Timestamp Raw: %s\nThread Timestamp: %s\nChannel ID: %s\n"+
		"From User: %s (<@%s>) (%s)\n\n"+
		"Slack Message Content:\n%s\n",
		ev.TimeStamp, threadLine, ev.Channel, senderName, ev.User, senderRealName, text)

	mentions := extractMentions(ev.Text)
	if len(mentions) > 0 {
		_, _ = sb.WriteString("\nMentions found in the message:\n")
		for _, id := range mentions {
			if id == botUID {
				_, _ = fmt.Fprintf(&sb, "Bot (this is the Slack app the message was sent to): %s\n", id)
				continue
			}
			if user := s.lookupUser(ctx, id); user != nil {
				_, _ = fmt.Fprintf(&sb, "User: %s => %s (%s)\n", id, user.Name, user.RealName)
			} else {
				_, _ = fmt.Fprintf(&sb, "User: %s\n", id)
			}
		}
	}
	return sb.String()
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
