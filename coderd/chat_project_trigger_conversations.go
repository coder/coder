package coderd

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// A trigger with continue_conversations keeps each external conversation,
// such as a Slack thread or a GitHub issue, in one chat. The first event of
// a conversation starts a chat and records it in
// chat_project_trigger_conversations; later events are sent to that chat as
// new user messages. Event IDs are claimed in chat_project_trigger_events so
// retried and duplicate deliveries are handled once.

const (
	// maxChatProjectTriggerConversationKeyChars bounds conversation IDs a
	// webhook caller chooses.
	maxChatProjectTriggerConversationKeyChars = 256
	// chatProjectTriggerEventRetention is how long handled event IDs are
	// remembered. Slack retries within minutes.
	chatProjectTriggerEventRetention = time.Hour
	// slackSignatureMaxSkew is how far a signed Slack request's timestamp
	// may be from the server's clock.
	slackSignatureMaxSkew = 5 * time.Minute
)

// chatProjectTriggerRejectError fails a trigger call with a status and
// response that are shown to the caller and recorded as the delivery.
type chatProjectTriggerRejectError struct {
	status int
	resp   codersdk.Response
}

func (e *chatProjectTriggerRejectError) Error() string { return e.resp.Message }

// chatProjectTriggerOutcome is what an accepted trigger event did.
type chatProjectTriggerOutcome struct {
	chat database.Chat
	// continued is set when the event was sent to an existing chat.
	continued bool
	// ignored, when set, explains why the event did nothing after all,
	// such as a duplicate delivery.
	ignored string
	// duplicate is set when the event was already handled.
	duplicate bool
}

// chatProjectTriggerRun starts or continues the chat for one accepted
// trigger event.
type chatProjectTriggerRun struct {
	api     *API
	trigger database.ChatProjectTrigger
	project database.ChatProject
	// agentCtx acts as the agent's user; sysCtx as the system.
	agentCtx context.Context
	sysCtx   context.Context
	event    chatProjectTriggerEvent
	// startMessage is the first message of a new chat; nextMessage is
	// sent to a chat the event continues.
	startMessage string
	nextMessage  string
}

const duplicateChatProjectTriggerEvent = "This event was already handled. Slack retries deliveries, and can send one message as several events."

// start creates a new chat for the event.
func (run *chatProjectTriggerRun) start() (chatProjectTriggerOutcome, error) {
	store := run.api.Database
	claimed, err := claimChatProjectTriggerEvents(run.sysCtx, store, run.trigger.ID, run.event.eventKeys)
	if err != nil {
		return chatProjectTriggerOutcome{}, err
	}
	if !claimed {
		return chatProjectTriggerOutcome{ignored: duplicateChatProjectTriggerEvent, duplicate: true}, nil
	}
	chat, err := run.createChat()
	if err != nil {
		// Let a retry of this event try again.
		if releaseErr := store.DeleteChatProjectTriggerEvents(run.sysCtx, database.DeleteChatProjectTriggerEventsParams{
			TriggerID: run.trigger.ID,
			EventKeys: run.event.eventKeys,
		}); releaseErr != nil {
			run.api.Logger.Warn(run.agentCtx, "release trigger event claims", slog.F("trigger_id", run.trigger.ID), slog.Error(releaseErr))
		}
		return chatProjectTriggerOutcome{}, err
	}
	return chatProjectTriggerOutcome{chat: chat}, nil
}

// continueOrStart sends the event to the chat of its conversation, or
// starts one when the conversation has none, its chat was archived, or
// startIgnores allows it. The conversation is locked while this decides, so
// concurrent events in one conversation cannot start two chats.
func (run *chatProjectTriggerRun) continueOrStart(startIgnores func() string) (chatProjectTriggerOutcome, error) {
	var outcome chatProjectTriggerOutcome
	err := run.api.Database.InTx(func(tx database.Store) error {
		outcome = chatProjectTriggerOutcome{}
		lockKey := fmt.Sprintf("chat-trigger-conversation:%s:%s", run.trigger.ID, run.event.conversationKey)
		if err := tx.AcquireLock(run.sysCtx, database.GenLockID(lockKey)); err != nil {
			return xerrors.Errorf("lock conversation: %w", err)
		}
		conversation, err := tx.GetChatProjectTriggerConversation(run.sysCtx, database.GetChatProjectTriggerConversationParams{
			TriggerID:       run.trigger.ID,
			ConversationKey: run.event.conversationKey,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get conversation: %w", err)
		}
		if err == nil {
			chat, err := tx.GetChatByID(run.sysCtx, conversation.ChatID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return xerrors.Errorf("get conversation chat: %w", err)
			}
			if err == nil && !chat.Archived {
				outcome = chatProjectTriggerOutcome{chat: chat, continued: true}
			}
		}
		if !outcome.continued {
			if message := startIgnores(); message != "" {
				outcome.ignored = message
				return nil
			}
		}
		// Claims commit with the decision, so a delivery that fails here
		// can be retried.
		claimed, err := claimChatProjectTriggerEvents(run.sysCtx, tx, run.trigger.ID, run.event.eventKeys)
		if err != nil {
			return err
		}
		if !claimed {
			outcome = chatProjectTriggerOutcome{ignored: duplicateChatProjectTriggerEvent, duplicate: true}
			return nil
		}
		if outcome.continued {
			return nil
		}
		// Chat creation runs outside the transaction. The lock is held
		// until the mapping is written.
		chat, err := run.createChat()
		if err != nil {
			return err
		}
		if _, err := tx.UpsertChatProjectTriggerConversation(run.sysCtx, database.UpsertChatProjectTriggerConversationParams{
			TriggerID:       run.trigger.ID,
			ConversationKey: run.event.conversationKey,
			ChatID:          chat.ID,
		}); err != nil {
			return xerrors.Errorf("record conversation chat: %w", err)
		}
		outcome.chat = chat
		return nil
	}, nil)
	if err != nil || !outcome.continued || outcome.ignored != "" {
		return outcome, err
	}
	// Sending runs after the transaction so a long queue or interrupt does
	// not hold the conversation lock.
	_, err = run.api.chatDaemon.SendMessage(run.agentCtx, chatd.SendMessageOptions{
		ChatID:       outcome.chat.ID,
		CreatedBy:    run.project.OwnerID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText(run.nextMessage)},
		BusyBehavior: chatd.SendMessageBusyBehavior(run.trigger.BusyBehavior),
	})
	if err != nil {
		if errors.Is(err, chatd.ErrChatArchived) {
			return outcome, &chatProjectTriggerRejectError{
				status: http.StatusConflict,
				resp:   codersdk.Response{Message: "The conversation's chat was archived while the event was sent. Send the event again to start a new chat."},
			}
		}
		return outcome, xerrors.Errorf("send event to chat: %w", err)
	}
	return outcome, nil
}

// createChat starts a chat owned by the agent's user for the event and
// shares it with the agent's owner.
func (run *chatProjectTriggerRun) createChat() (database.Chat, error) {
	api, project := run.api, run.project
	agentUserID := project.AgentUserID.UUID
	title := run.trigger.Name
	if run.event.title != "" {
		title = truncateChatProjectTriggerTitle(run.event.title)
	}

	modelConfigID, reasoningEffort, status, resp := api.resolveCreateChatModelConfigID(run.agentCtx, agentUserID, codersdk.CreateChatRequest{
		OrganizationID: project.OrganizationID,
	}, project.ModelConfigID)
	if resp != nil {
		return database.Chat{}, &chatProjectTriggerRejectError{status: status, resp: *resp}
	}

	mcpServerIDs, unavailable, err := validateChatMCPServerIDs(run.agentCtx, api.Database, project.OrganizationID, project.MCPServerIDs)
	if err != nil {
		return database.Chat{}, err
	}
	mcpServerIDs = slices.DeleteFunc(mcpServerIDs, func(id uuid.UUID) bool {
		return slices.Contains(unavailable, id)
	})

	chat, err := api.chatDaemon.CreateChat(run.agentCtx, chatd.CreateOptions{
		OrganizationID:  project.OrganizationID,
		OwnerID:         agentUserID,
		CreatedBy:       project.OwnerID,
		ProjectID:       uuid.NullUUID{UUID: project.ID, Valid: true},
		Title:           title,
		TitleSource:     database.ChatTitleSourceFallback,
		ModelConfigID:   modelConfigID,
		ReasoningEffort: reasoningEffort,
		ClientType:      database.ChatClientTypeApi,
		InitialUserContent: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText(run.startMessage),
		},
		MCPServerIDs: mcpServerIDs,
		Labels: database.StringMap{
			codersdk.ChatProjectTriggerLabel: run.trigger.ID.String(),
		},
	})
	if err != nil {
		return database.Chat{}, err
	}
	if err := shareChatProjectAgentChat(run.agentCtx, api.Database, chat.ID, project.OwnerID); err != nil {
		api.Logger.Warn(run.agentCtx, "share triggered chat with agent owner", slog.F("chat_id", chat.ID), slog.Error(err))
	}
	return chat, nil
}

// claimChatProjectTriggerEvents records event keys as handled. It returns
// false when any of them was already claimed, which marks the event as a
// duplicate. Events without keys are always claimed.
func claimChatProjectTriggerEvents(ctx context.Context, store database.Store, triggerID uuid.UUID, keys []string) (bool, error) {
	if len(keys) == 0 {
		return true, nil
	}
	if err := store.DeleteOldChatProjectTriggerEvents(ctx, database.DeleteOldChatProjectTriggerEventsParams{
		TriggerID: triggerID,
		Before:    dbtime.Now().Add(-chatProjectTriggerEventRetention),
	}); err != nil {
		return false, xerrors.Errorf("prune trigger events: %w", err)
	}
	claimed, err := store.InsertChatProjectTriggerEvents(ctx, database.InsertChatProjectTriggerEventsParams{
		TriggerID: triggerID,
		EventKeys: keys,
	})
	if err != nil {
		return false, xerrors.Errorf("claim trigger events: %w", err)
	}
	if len(claimed) == len(keys) {
		return true, nil
	}
	// Release the keys this call did claim, so a later delivery of the
	// other event that shares them is not lost.
	if len(claimed) > 0 {
		if err := store.DeleteChatProjectTriggerEvents(ctx, database.DeleteChatProjectTriggerEventsParams{
			TriggerID: triggerID,
			EventKeys: claimed,
		}); err != nil {
			return false, xerrors.Errorf("release trigger events: %w", err)
		}
	}
	return false, nil
}

// webhookConversationKey returns the conversation a plain webhook call
// belongs to, from the X-Coder-Conversation-Id header.
func webhookConversationKey(header http.Header) (string, error) {
	key := strings.TrimSpace(header.Get(codersdk.ChatProjectTriggerConversationHeader))
	if utf8.RuneCountInString(key) > maxChatProjectTriggerConversationKeyChars {
		return "", xerrors.Errorf("%s must be at most %d characters", codersdk.ChatProjectTriggerConversationHeader, maxChatProjectTriggerConversationKeyChars)
	}
	return key, nil
}

// verifyGitHubSignature checks X-Hub-Signature-256, the HMAC-SHA256 of the
// body keyed with the webhook's secret.
// See https://docs.github.com/webhooks/using-webhooks/validating-webhook-deliveries.
func verifyGitHubSignature(secret string, header http.Header, body []byte) error {
	signature := header.Get("X-Hub-Signature-256")
	if signature == "" {
		return xerrors.New("missing X-Hub-Signature-256 header: set the same secret on the GitHub webhook")
	}
	got, ok := strings.CutPrefix(signature, "sha256=")
	if !ok {
		return xerrors.New("X-Hub-Signature-256 must start with sha256=")
	}
	gotMAC, err := hex.DecodeString(got)
	if err != nil {
		return xerrors.New("X-Hub-Signature-256 is not hex encoded")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !hmac.Equal(gotMAC, mac.Sum(nil)) {
		return xerrors.New("signature mismatch")
	}
	return nil
}

// verifySlackSignature checks a request against Slack's signing secret.
// See https://api.slack.com/authentication/verifying-requests-from-slack.
func verifySlackSignature(secret string, header http.Header, body []byte, now time.Time) error {
	timestamp := header.Get("X-Slack-Request-Timestamp")
	signature := header.Get("X-Slack-Signature")
	if timestamp == "" || signature == "" {
		return xerrors.New("the request has no X-Slack-Signature or X-Slack-Request-Timestamp header")
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return xerrors.New("X-Slack-Request-Timestamp is not a Unix timestamp")
	}
	skew := now.Sub(time.Unix(seconds, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > slackSignatureMaxSkew {
		return xerrors.New("X-Slack-Request-Timestamp is more than 5 minutes from the server's clock")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":"))
	_, _ = mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return xerrors.New("X-Slack-Signature does not match")
	}
	return nil
}

// chatProjectTriggerFollowUpMessage is sent to a chat a plain webhook call
// continues. Like the first message, it labels the payload as data.
func chatProjectTriggerFollowUpMessage(trigger database.ChatProjectTrigger, contentType string, payload []byte) string {
	body := chatProjectTriggerMessage(database.ChatProjectTrigger{Name: trigger.Name}, contentType, payload)
	return strings.Replace(body,
		fmt.Sprintf("This chat was started by the %q webhook trigger.", trigger.Name),
		fmt.Sprintf("The %q webhook trigger was called again in this conversation.", trigger.Name), 1)
}

// chatProjectTriggerEventFollowUpMessage is sent to a chat a GitHub or
// Slack event continues. The trigger's prompt is not repeated.
func chatProjectTriggerEventFollowUpMessage(trigger database.ChatProjectTrigger, event chatProjectTriggerEvent) string {
	body := chatProjectTriggerEventMessage(database.ChatProjectTrigger{Name: trigger.Name}, event)
	return strings.Replace(body,
		fmt.Sprintf("This chat was started by the %q trigger.", trigger.Name),
		fmt.Sprintf("A new event arrived in this conversation through the %q trigger.", trigger.Name), 1)
}
