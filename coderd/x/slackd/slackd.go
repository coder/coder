// Package slackd connects coderd to a Slack app over Socket Mode and
// submits Slack app mentions to chats. It is the built-in counterpart
// of github.com/coder/coder-agents-slackbot. Incoming Slack events are
// reduced to message submission; replies to Slack happen through the
// Slack tools that chatd enables for chats carrying the slackd labels.
//
// Every coderd replica runs its own Socket Mode connection, so the
// same Slack event can be delivered to multiple replicas. slackd
// deduplicates in two layers:
//
//   - Chat creation for a new Slack thread is serialized through
//     chatd's DedupLabels support, which takes a Postgres advisory
//     transaction lock derived from the thread labels and returns the
//     existing chat when another replica won the race.
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
	// Slack messages.
	ChatOwnerUserID uuid.UUID

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
	socket      SocketClient
	userInfoAPI UserInfoAPI

	closeCtx    context.Context
	closeCancel context.CancelFunc
	wg          sync.WaitGroup

	// Reconnection backoff bounds; fixed except in tests.
	backoffFloor time.Duration
	backoffCeil  time.Duration

	userCache sync.Map // slack user id -> *slack.User

	mu              sync.Mutex
	botUID          string
	orgID           uuid.UUID
	apiKeyID        string
	apiKeyExpiresAt time.Time
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

// handleMention submits one Slack app mention to the chat bound to its
// thread, creating the chat when the thread is new.
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

	chatID, err := s.findChat(ctx, labels)
	if err != nil {
		return xerrors.Errorf("find chat for thread %q: %w", threadKey, err)
	}
	apiKeyID, err := s.ensureAPIKeyID(ctx)
	if err != nil {
		return xerrors.Errorf("ensure api key: %w", err)
	}
	if chatID == uuid.Nil {
		orgID, err := s.resolveOrganizationID(ctx)
		if err != nil {
			return xerrors.Errorf("resolve organization: %w", err)
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
		chat, err := s.chat.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     orgID,
			OwnerID:            s.ownerID,
			APIKeyID:           apiKeyID,
			ModelConfigID:      modelConfig.ID,
			Title:              "Slack thread " + threadKey,
			SystemPrompt:       systemPrompt,
			InitialUserContent: content,
			Labels:             database.StringMap(labels),
			DedupLabels:        labels,
		})
		switch {
		case err == nil:
			s.logger.Info(ctx, "created chat for slack thread",
				slog.F("chat_id", chat.ID), slog.F("thread", threadKey))
			return nil
		case xerrors.Is(err, chatd.ErrChatAlreadyExists):
			// Another replica created the chat first; fall through to
			// message submission, which dedups by event id.
			chatID = chat.ID
		default:
			return xerrors.Errorf("create chat: %w", err)
		}
	}

	_, err = s.chat.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:           chatID,
		CreatedBy:        s.ownerID,
		APIKeyID:         apiKeyID,
		Content:          content,
		BusyBehavior:     chatd.SendMessageBusyBehaviorInterrupt,
		DedupMetadataKey: MetadataKeySlackEventID,
	})
	if xerrors.Is(err, chatd.ErrDuplicateMessage) {
		s.logger.Debug(ctx, "duplicate slack event, skipping",
			slog.F("event_id", eventID), slog.F("chat_id", chatID))
		return nil
	}
	if err != nil {
		return xerrors.Errorf("send message: %w", err)
	}
	s.logger.Info(ctx, "submitted slack message to chat",
		slog.F("chat_id", chatID), slog.F("event_id", eventID))
	return nil
}

// findChat returns the oldest chat owned by the configured user with
// the given labels, or uuid.Nil when none exists.
func (s *Server) findChat(ctx context.Context, labels map[string]string) (uuid.UUID, error) {
	filter, err := json.Marshal(labels)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("marshal label filter: %w", err)
	}
	chats, err := s.db.GetChatsByOwnerAndLabels(ctx, database.GetChatsByOwnerAndLabelsParams{
		OwnerID:     s.ownerID,
		LabelFilter: filter,
	})
	if err != nil {
		return uuid.Nil, err
	}
	if len(chats) == 0 {
		return uuid.Nil, nil
	}
	return chats[0].ID, nil
}

// ensureAPIKeyID returns a cached delegated API key for the chat
// owner, minting a new one when the cached key is missing or close to
// expiry. User chat messages require an API key id: the AI Gateway
// attributes LLM usage of the generation turn to it.
func (s *Server) ensureAPIKeyID(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.apiKeyID != "" && time.Until(s.apiKeyExpiresAt) > apiKeyRotateBefore {
		return s.apiKeyID, nil
	}
	params, _, err := apikey.Generate(apikey.CreateParams{
		UserID:          s.ownerID,
		LoginType:       database.LoginTypeToken,
		LifetimeSeconds: int64(apiKeyLifetime / time.Second),
	})
	if err != nil {
		return "", xerrors.Errorf("generate api key: %w", err)
	}
	// Token names are unique per user and every replica mints its own
	// key, so include the random key id in the name.
	params.TokenName = "slackd-" + params.ID
	inserted, err := s.db.InsertAPIKey(ctx, params)
	if err != nil {
		return "", xerrors.Errorf("insert api key: %w", err)
	}
	s.apiKeyID = inserted.ID
	s.apiKeyExpiresAt = inserted.ExpiresAt
	return s.apiKeyID, nil
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

// resolveOrganizationID caches the organization used for created
// chats: the chat owner's default organization, or their only
// organization membership.
func (s *Server) resolveOrganizationID(ctx context.Context) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.orgID != uuid.Nil {
		return s.orgID, nil
	}
	orgs, err := s.db.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  s.ownerID,
		Deleted: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get organizations for chat owner: %w", err)
	}
	if len(orgs) == 0 {
		return uuid.Nil, xerrors.Errorf("chat owner %s belongs to no organization", s.ownerID)
	}
	for _, org := range orgs {
		if org.IsDefault {
			s.orgID = org.ID
			return s.orgID, nil
		}
	}
	if len(orgs) > 1 {
		return uuid.Nil, xerrors.Errorf("chat owner %s belongs to %d organizations and none is the default", s.ownerID, len(orgs))
	}
	s.orgID = orgs[0].ID
	return s.orgID, nil
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
