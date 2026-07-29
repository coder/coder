package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/slack-go/slack"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/quartz"
)

const (
	// slackThreadStatusTyping is shown until a generated activity is
	// available and whenever activity generation fails.
	slackThreadStatusTyping = "is typing..."
	// slackThreadStatusInterval is how often the status is re-set while
	// the runner is alive. The periodic re-set retries transient Slack
	// failures and restores the status after Slack auto-clears it when
	// a message is posted to the thread.
	slackThreadStatusInterval = 30 * time.Second
	// slackThreadStatusInitialDelay keeps the typing fallback visible before
	// the first generated activity replaces it.
	slackThreadStatusInitialDelay = 15 * time.Second
)

type slackThreadStatusChatLoader func(context.Context, uuid.UUID) (database.Chat, error)
type slackThreadStatusGenerator func(context.Context, database.Chat) (string, error)

// slackThreadStatus maintains the Slack assistant thread status of a
// slackd-bound chat for the lifetime of its runner. It caches generated
// activity in memory until durable chat history changes and clears the status
// when its context is canceled. All Slack and generation calls are
// best-effort; errors are logged and retried on the next tick.
type slackThreadStatus struct {
	api      chattool.SlackAPI
	chat     database.Chat
	loadChat slackThreadStatusChatLoader
	generate slackThreadStatusGenerator
	channel  string
	threadTS string
	logger   slog.Logger
	clock    quartz.Clock
	done     chan struct{}

	cachedStatus      string
	historyVersion    int64
	hasHistoryVersion bool
}

// newSlackThreadStatus returns a status maintainer for the chat, or nil
// when the chat is not bound to a Slack thread via the slackd labels,
// the label is malformed, or a required dependency is not configured.
func newSlackThreadStatus(
	api chattool.SlackAPI,
	chat database.Chat,
	loadChat slackThreadStatusChatLoader,
	generate slackThreadStatusGenerator,
	logger slog.Logger,
	clock quartz.Clock,
) *slackThreadStatus {
	if api == nil || loadChat == nil || generate == nil {
		return nil
	}
	if chat.Labels[LabelSlackd] != "true" {
		return nil
	}
	threadLabel, ok := chat.Labels[LabelSlackThread]
	if !ok {
		return nil
	}
	channel, threadTS, ok := parseSlackThreadLabel(threadLabel)
	if !ok {
		logger.Warn(context.Background(), "chat has a malformed slack thread label, skipping slack thread status",
			slog.F("chat_id", chat.ID),
			slog.F("label", threadLabel),
		)
		return nil
	}
	return &slackThreadStatus{
		api:          api,
		chat:         chat,
		loadChat:     loadChat,
		generate:     generate,
		channel:      channel,
		threadTS:     threadTS,
		logger:       logger,
		clock:        clock,
		done:         make(chan struct{}),
		cachedStatus: slackThreadStatusTyping,
	}
}

// start spawns the maintenance goroutine. It sets the typing fallback
// immediately, waits slackThreadStatusInitialDelay before generating an
// activity, re-sets the cached status every slackThreadStatusInterval, and
// clears it when ctx is canceled. Use wait to block until the final clear has
// completed.
func (s *slackThreadStatus) start(ctx context.Context) {
	go func() {
		defer close(s.done)
		s.set(ctx, s.cachedStatus)

		initialDelay := s.clock.NewTimer(slackThreadStatusInitialDelay, "chatd", "slack-thread-status-initial-delay")
		defer initialDelay.Stop()
		select {
		case <-initialDelay.C:
			s.refresh(ctx, s.chat)
		case <-ctx.Done():
			clearCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownCleanupTimeout)
			s.set(clearCtx, "")
			cancel()
			return
		}

		ticker := s.clock.NewTicker(slackThreadStatusInterval, "chatd", "slack-thread-status")
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.tick(ctx)
			case <-ctx.Done():
				clearCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownCleanupTimeout)
				s.set(clearCtx, "")
				cancel()
				return
			}
		}
	}()
}

func (s *slackThreadStatus) tick(ctx context.Context) {
	chat, err := s.loadChat(ctx, s.chat.ID)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug(ctx, "load chat for Slack thread status", slogError(err))
		}
		s.set(ctx, s.cachedStatus)
		return
	}
	if s.hasHistoryVersion && chat.HistoryVersion == s.historyVersion {
		s.set(ctx, s.cachedStatus)
		return
	}

	s.set(ctx, s.cachedStatus)
	s.refresh(ctx, chat)
}

func (s *slackThreadStatus) refresh(ctx context.Context, chat database.Chat) {
	status, err := s.generate(ctx, chat)
	if err != nil {
		s.logger.Warn(ctx, "generate Slack thread status",
			slog.F("chat_id", chat.ID),
			slogError(err),
		)
		return
	}

	if status == "" {
		status = slackThreadStatusTyping
	}
	s.cachedStatus = status
	s.historyVersion = chat.HistoryVersion
	s.hasHistoryVersion = true
	if status != slackThreadStatusTyping {
		s.set(ctx, status)
	}
}

// wait blocks until the maintenance goroutine has exited, including the
// final status clear. It must only be called after start.
func (s *slackThreadStatus) wait() {
	<-s.done
}

// set updates the thread status; an empty status clears it. Failures
// are logged and left to the next tick.
func (s *slackThreadStatus) set(ctx context.Context, status string) {
	err := s.api.SetAssistantThreadsStatusContext(ctx, slack.AssistantThreadsSetStatusParameters{
		ChannelID: s.channel,
		ThreadTS:  s.threadTS,
		Status:    status,
	})
	if err != nil && ctx.Err() == nil {
		s.logger.Warn(ctx, "set slack thread status",
			slog.F("channel", s.channel),
			slog.F("thread_ts", s.threadTS),
			slog.F("status", status),
			slogError(err),
		)
	}
}
