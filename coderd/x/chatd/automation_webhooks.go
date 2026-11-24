package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// webhookUnsubscribeTimeout bounds the unsubscribe that a delete or disable
// makes before it answers.
const webhookUnsubscribeTimeout = 15 * time.Second

// webhookEnvelope holds the fields of the control envelopes that the events
// extension POSTs besides events. A body without a type is an event.
type webhookEnvelope struct {
	Type      string  `json:"type"`
	Challenge string  `json:"challenge"`
	Cursor    *string `json:"cursor"`
	Error     *struct {
		Code int64 `json:"code"`
	} `json:"error"`
}

// ReceiveMCPEventWebhook answers one Standard Webhooks delivery to the
// callback URL of an automation. It returns the HTTP status and, for a
// verification, the challenge to echo. 410 tells the server to drop a
// delivery that no subscription takes, 401 rejects one that does not verify,
// and 503 asks it to retry later.
func (p *Server) ReceiveMCPEventWebhook(ctx context.Context, automationID uuid.UUID, header http.Header, body []byte) (status int, challenge string) {
	//nolint:gocritic // A delivery has no session; its signature authenticates it.
	ctx = dbauthz.AsChatd(ctx)
	sub, secret, err := p.webhookTarget(ctx, automationID)
	switch {
	case errors.Is(err, errSubscriptionInactive):
		return http.StatusGone, ""
	case err != nil:
		p.logger.Warn(ctx, "load chat automation webhook target", slog.F("automation_id", automationID), slog.Error(err))
		return http.StatusServiceUnavailable, ""
	}
	if chatwebhook.Verify(header, body, secret, p.clock.Now("chatd", "automation-webhooks")) != nil {
		return http.StatusUnauthorized, ""
	}
	id := header.Get("X-MCP-Subscription-Id")
	if sub.WebhookSubscriptionID.Valid && id != sub.WebhookSubscriptionID.String {
		return http.StatusUnauthorized, ""
	}
	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return http.StatusBadRequest, ""
	}
	if envelope.Type == "verification" {
		// A first subscribe verifies before its result names the id.
		if !sub.WebhookSubscriptionID.Valid && id != "" {
			if err := p.db.SetChatAutomationWebhookSubscriptionID(ctx, database.SetChatAutomationWebhookSubscriptionIDParams{
				AutomationID:          automationID,
				WebhookSubscriptionID: id,
			}); err != nil {
				return http.StatusServiceUnavailable, ""
			}
		}
		return http.StatusOK, envelope.Challenge
	}
	if sub.State == database.ChatAutomationSubscriptionStatePending {
		return http.StatusServiceUnavailable, ""
	}
	switch envelope.Type {
	case "gap":
		err = p.appendInbox(ctx, inboxAppend{AutomationID: automationID, Cursor: envelope.Cursor, Truncated: true})
	case "terminated":
		reason := runReasonMCPTerminated
		if envelope.Error != nil && eventErrorActions[envelope.Error.Code].terminate != "" {
			reason = eventErrorActions[envelope.Error.Code].terminate
		}
		p.terminateSubscription(ctx, automationID, reason)
	case "":
		var event mcpclient.EventOccurrence
		if err := json.Unmarshal(body, &event); err != nil || event.EventID == "" || event.Name != sub.EventName {
			return http.StatusBadRequest, ""
		}
		pending, cerr := p.db.CountPendingChatAutomationInboxRows(ctx, database.CountPendingChatAutomationInboxRowsParams{
			AutomationID: automationID,
			MaxPending:   maxPendingInboxRows,
		})
		if cerr != nil || pending >= maxPendingInboxRows {
			return http.StatusServiceUnavailable, ""
		}
		err = p.appendInbox(ctx, inboxAppend{AutomationID: automationID, Events: []mcpclient.EventOccurrence{event}, Cursor: event.Cursor})
	default:
		return http.StatusBadRequest, ""
	}
	switch {
	case errors.Is(err, errSubscriptionInactive):
		return http.StatusGone, ""
	case err != nil:
		p.logger.Warn(ctx, "append chat automation webhook delivery", slog.F("automation_id", automationID), slog.Error(err))
		return http.StatusServiceUnavailable, ""
	}
	return http.StatusOK, ""
}

// webhookTarget returns the webhook subscription of automationID and its
// secret. errSubscriptionInactive means that no enabled automation of an
// owner with automations on has a live webhook subscription there.
func (p *Server) webhookTarget(ctx context.Context, automationID uuid.UUID) (database.ChatAutomationMCPSubscription, string, error) {
	automation, err := p.db.GetChatAutomationByID(ctx, automationID)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatAutomationMCPSubscription{}, "", errSubscriptionInactive
	}
	if err != nil {
		return database.ChatAutomationMCPSubscription{}, "", err
	}
	if !automation.Enabled || p.automationsEnabled == nil || !p.automationsEnabled(ctx, automation.OwnerID) {
		return database.ChatAutomationMCPSubscription{}, "", errSubscriptionInactive
	}
	subs, err := p.db.GetChatAutomationMCPSubscriptionsByAutomationIDs(ctx, []uuid.UUID{automationID})
	if err != nil {
		return database.ChatAutomationMCPSubscription{}, "", err
	}
	if len(subs) != 1 || subs[0].Delivery != database.ChatAutomationEventDeliveryWebhook ||
		subs[0].State == database.ChatAutomationSubscriptionStateTerminated {
		return database.ChatAutomationMCPSubscription{}, "", errSubscriptionInactive
	}
	secret, err := p.db.GetChatAutomationWebhookSecret(ctx, automationID)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatAutomationMCPSubscription{}, "", errSubscriptionInactive
	}
	return subs[0], secret.Secret, err
}

// tickWebhookRenewals subscribes again each webhook subscription whose
// renewal is due, before the server's grant ends, and subscribes a pending
// one that its subscribe left behind.
func (p *Server) tickWebhookRenewals(ctx context.Context, now time.Time) {
	//nolint:gocritic // The renewal reads the subscriptions and secrets of every owner.
	ctx = dbauthz.AsChatd(ctx)
	claimed, err := p.db.ClaimDueChatAutomationWebhookRenewals(ctx, database.ClaimDueChatAutomationWebhookRenewalsParams{
		Now:        now,
		LimitCount: eventClaimBatch,
		LeaseUntil: now.Add(eventLease),
	})
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Warn(ctx, "claim due chat automation webhook renewals", slog.Error(err))
		}
		return
	}
	// Subscriptions that do not start within the budget keep their lease.
	ctx, cancel := context.WithTimeout(ctx, eventPollBudget)
	defer cancel()
	for _, sub := range claimed {
		if ctx.Err() != nil {
			return
		}
		p.renewWebhook(ctx, sub, now)
	}
}

// renewWebhook subscribes sub with its stored cursor and secret and stores
// the grant. It renews nothing for an owner whose runs are refused; the
// lease retries.
func (p *Server) renewWebhook(ctx context.Context, sub database.ChatAutomationMCPSubscription, now time.Time) {
	if !sub.MCPServerConfigID.Valid {
		p.terminateSubscription(ctx, sub.AutomationID, runReasonMCPServerUnavailable)
		return
	}
	automation, err := p.db.GetChatAutomationByID(ctx, sub.AutomationID)
	if err != nil || !p.eventOwnerActive(ctx, automation.OwnerID) {
		return
	}
	secret, err := p.db.GetChatAutomationWebhookSecret(ctx, sub.AutomationID)
	if err != nil {
		p.logger.Warn(ctx, "get chat automation webhook secret", slog.F("automation_id", sub.AutomationID), slog.Error(err))
		return
	}
	conn, err := p.withEvents(ctx, sub, automation.OwnerID, func(session *mcpclient.EventsSession) error {
		params, err := chatautomation.WebhookSubscribeParams(p.accessURL, sub, secret.Secret)
		if err != nil {
			return err
		}
		res, err := session.Subscribe(ctx, params)
		if err != nil {
			return err
		}
		if _, err := p.db.UpdateChatAutomationWebhookGrant(ctx, chatautomation.WebhookGrant(sub.AutomationID, res, p.clock.Now("chatd", "automation-webhooks"))); err != nil {
			return xerrors.Errorf("store webhook grant: %w", err)
		}
		if !res.Truncated {
			return nil
		}
		return p.appendInbox(ctx, inboxAppend{AutomationID: sub.AutomationID, Truncated: true})
	})
	if err == nil || errors.Is(err, errSubscriptionInactive) {
		return
	}
	p.handleEventError(ctx, sub.AutomationID, err, conn, func(backoff time.Duration, message string) error {
		return p.db.SetChatAutomationWebhookRenewAt(ctx, database.SetChatAutomationWebhookRenewAtParams{
			AutomationID: sub.AutomationID,
			RenewAt:      now.Add(backoff),
			LastError:    sql.NullString{String: message, Valid: true},
			Now:          now,
		})
	})
}

// UnsubscribeWebhook ends a webhook subscription on its MCP server as owner,
// best effort: the server's grant ends the rest.
func (p *Server) UnsubscribeWebhook(ctx context.Context, owner uuid.UUID, sub database.ChatAutomationMCPSubscription) {
	//nolint:gocritic // Reaching the MCP server reads and refreshes the owner's token as chatd.
	ctx, cancel := context.WithTimeout(dbauthz.AsChatd(ctx), webhookUnsubscribeTimeout)
	defer cancel()
	_, err := p.withEvents(ctx, sub, owner, func(session *mcpclient.EventsSession) error {
		var args map[string]any
		if err := json.Unmarshal(sub.Arguments, &args); err != nil {
			return xerrors.Errorf("decode arguments: %w", err)
		}
		return session.Unsubscribe(ctx, sub.EventName, args, chatautomation.WebhookURL(p.accessURL, sub.AutomationID))
	})
	if err != nil {
		p.logger.Warn(ctx, "unsubscribe chat automation webhook", slog.F("automation_id", sub.AutomationID), slog.Error(err))
	}
}

// withEvents runs fn with an events session to the MCP server of sub as
// owner. The returned conn is set when the session opened.
func (p *Server) withEvents(ctx context.Context, sub database.ChatAutomationMCPSubscription, owner uuid.UUID, fn func(*mcpclient.EventsSession) error) (*eventsConn, error) {
	if !sub.MCPServerConfigID.Valid {
		return nil, xerrors.New("the MCP server config was deleted")
	}
	cfg, err := p.db.GetMCPServerConfigByID(ctx, sub.MCPServerConfigID.UUID)
	if err != nil {
		return nil, xerrors.Errorf("get MCP server config: %w", err)
	}
	conn, err := p.openEvents(ctx, cfg, owner)
	if err != nil {
		return conn, err
	}
	defer conn.close()
	if !conn.session.Supported() {
		return conn, mcpclient.ErrEventsUnsupported
	}
	return conn, fn(conn.session)
}
