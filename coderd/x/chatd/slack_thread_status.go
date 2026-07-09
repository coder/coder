package chatd

import (
	"context"
	"time"

	"github.com/slack-go/slack"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/quartz"
)

const (
	// slackThreadStatusThinking is shown on the Slack thread while the
	// chat's runner is alive.
	slackThreadStatusThinking = "is thinking..."
	// slackThreadStatusInterval is how often the status is re-set while
	// the runner is alive. The periodic re-set retries transient Slack
	// failures and restores the status after Slack auto-clears it when
	// a message is posted to the thread.
	slackThreadStatusInterval = time.Minute
)

// slackThreadStatus maintains the Slack assistant thread status of a
// slackd-bound chat for the lifetime of its runner. It is intentionally
// unaware of chat status: while the maintenance goroutine is alive it
// keeps the thread status set to "thinking", and it clears the status
// when its context is canceled. All Slack calls are best-effort; errors
// are logged and retried on the next tick.
type slackThreadStatus struct {
	api      chattool.SlackAPI
	channel  string
	threadTS string
	logger   slog.Logger
	clock    quartz.Clock
	done     chan struct{}
}

// newSlackThreadStatus returns a status maintainer for the chat, or nil
// when the chat is not bound to a Slack thread via the slackd labels,
// the label is malformed, or no Slack client is configured.
func newSlackThreadStatus(api chattool.SlackAPI, chat database.Chat, logger slog.Logger, clock quartz.Clock) *slackThreadStatus {
	if api == nil {
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
		api:      api,
		channel:  channel,
		threadTS: threadTS,
		logger:   logger,
		clock:    clock,
		done:     make(chan struct{}),
	}
}

// start spawns the maintenance goroutine. It sets the status
// immediately, re-sets it every slackThreadStatusInterval, and clears
// it when ctx is canceled. Use wait to block until the final clear has
// completed.
func (s *slackThreadStatus) start(ctx context.Context) {
	go func() {
		defer close(s.done)
		s.set(ctx, slackThreadStatusThinking)
		ticker := s.clock.NewTicker(slackThreadStatusInterval, "chatd", "slack-thread-status")
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.set(ctx, slackThreadStatusThinking)
			case <-ctx.Done():
				clearCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownCleanupTimeout)
				s.set(clearCtx, "")
				cancel()
				return
			}
		}
	}()
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
