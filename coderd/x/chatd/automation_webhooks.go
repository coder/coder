package chatd

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
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

// webhookUnsubscribeTimeout bounds the unsubscribe of a delete or disable.
const webhookUnsubscribeTimeout = 15 * time.Second

// webhookEnvelope is a control envelope; a body without a type is an event.
type webhookEnvelope struct {
	Type      string  `json:"type"`
	Challenge string  `json:"challenge"`
	Cursor    *string `json:"cursor"`
	Error     *struct {
		Code int64 `json:"code"`
	} `json:"error"`
}

// ReceiveMCPEventWebhook answers one Standard Webhooks delivery to an
// automation's callback URL with the HTTP status and, for a verification, the
// challenge to echo. 410 drops a delivery that no subscription takes, 401 one
// that does not verify, and 503 asks for a retry.
func (p *Server) ReceiveMCPEventWebhook(ctx context.Context, automationID uuid.UUID, header http.Header, body []byte) (status int, challenge string) {
	//nolint:gocritic // A delivery has no session; its signature authenticates it.
	ctx = dbauthz.AsChatd(ctx)
	sub, secret, err := p.webhookTarget(ctx, automationID)
	switch {
	case errors.Is(err, errSubscriptionInactive):
		return http.StatusGone, ""
	case errors.Is(err, errOwnerAutomationsOff):
		return http.StatusServiceUnavailable, ""
	case err != nil:
		p.logger.Warn(ctx, "load chat automation webhook target", slog.F("automation_id", automationID), slog.Error(err))
		return http.StatusServiceUnavailable, ""
	}
	now := p.clock.Now("chatd", "automation-webhooks")
	if chatwebhook.Verify(header, body, secret, now) != nil {
		return http.StatusUnauthorized, ""
	}
	var envelope webhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return http.StatusBadRequest, ""
	}
	// A subscribe sends its challenge before its result stores the id, and a
	// new callback URL gets a new id, so the secret alone verifies it.
	if envelope.Type == "verification" {
		return http.StatusOK, envelope.Challenge
	}
	if sub.WebhookSubscriptionID.Valid && header.Get("X-MCP-Subscription-Id") != sub.WebhookSubscriptionID.String {
		return http.StatusUnauthorized, ""
	}
	if sub.State == database.ChatAutomationSubscriptionStatePending {
		return http.StatusServiceUnavailable, ""
	}
	// The grant has ended. A renewal may be in flight, so the server retries;
	// one that cannot renew, as after the owner disconnected, stays refused.
	if sub.RefreshBefore.Valid && now.After(sub.RefreshBefore.Time) {
		return http.StatusServiceUnavailable, ""
	}
	switch envelope.Type {
	case "gap":
		// A retry keeps its webhook-id, which Verify checked; its hash
		// bounds the key.
		key := sha256.Sum256([]byte(header.Get("webhook-id")))
		err = p.appendInbox(ctx, inboxAppend{AutomationID: automationID, Cursor: envelope.Cursor, Truncated: true, GapKey: "gap:" + hex.EncodeToString(key[:]), MaxPending: maxPendingInboxRows})
	case "terminated":
		reason := runReasonMCPTerminated
		if envelope.Error != nil && eventErrorActions[envelope.Error.Code].terminate != "" {
			reason = eventErrorActions[envelope.Error.Code].terminate
		}
		err = p.endSubscription(ctx, automationID, sql.NullTime{}, reason)
	case "":
		var event mcpclient.EventOccurrence
		if err := json.Unmarshal(body, &event); err != nil || event.EventID == "" || event.Name != sub.EventName {
			return http.StatusBadRequest, ""
		}
		err = p.appendInbox(ctx, inboxAppend{AutomationID: automationID, Events: []mcpclient.EventOccurrence{event}, Cursor: event.Cursor, MaxPending: maxPendingInboxRows})
	default:
		return http.StatusBadRequest, ""
	}
	switch {
	case errors.Is(err, errSubscriptionInactive):
		return http.StatusGone, ""
	case errors.Is(err, errInboxFull):
		return http.StatusServiceUnavailable, ""
	case err != nil:
		p.logger.Warn(ctx, "append chat automation webhook delivery", slog.F("automation_id", automationID), slog.Error(err))
		return http.StatusServiceUnavailable, ""
	}
	return http.StatusOK, ""
}

// errOwnerAutomationsOff asks for a retry: the experiment check fails closed.
var errOwnerAutomationsOff = xerrors.New("chat automations are off for the owner")

// webhookTarget returns the webhook subscription of automationID and its
// secret, or errSubscriptionInactive when no enabled automation and config
// take deliveries there.
func (p *Server) webhookTarget(ctx context.Context, automationID uuid.UUID) (database.ChatAutomationMCPSubscription, string, error) {
	var none database.ChatAutomationMCPSubscription
	automation, err := p.db.GetChatAutomationByID(ctx, automationID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !automation.Enabled) {
		return none, "", errSubscriptionInactive
	}
	if err != nil {
		return none, "", err
	}
	if p.automationsEnabled == nil || !p.automationsEnabled(ctx, automation.OwnerID) {
		return none, "", errOwnerAutomationsOff
	}
	subs, err := p.db.GetChatAutomationMCPSubscriptionsByAutomationIDs(ctx, []uuid.UUID{automationID})
	if err != nil {
		return none, "", err
	}
	if len(subs) != 1 || subs[0].Delivery != database.ChatAutomationEventDeliveryWebhook ||
		subs[0].State == database.ChatAutomationSubscriptionStateTerminated {
		return none, "", errSubscriptionInactive
	}
	// An admin disabled or deleted (NULL here) the server's config.
	cfg, err := p.db.GetMCPServerConfigByID(ctx, subs[0].MCPServerConfigID.UUID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !cfg.Enabled) {
		return none, "", errSubscriptionInactive
	}
	if err != nil {
		return none, "", err
	}
	secret, err := p.db.GetChatAutomationWebhookSecret(ctx, automationID)
	if errors.Is(err, sql.ErrNoRows) {
		return none, "", errSubscriptionInactive
	}
	return subs[0], secret.Secret, err
}

// tickWebhookRenewals subscribes again each webhook subscription whose
// renewal is due, and each pending one that its subscribe left behind.
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
	var wg sync.WaitGroup
	slots := make(chan struct{}, eventPollConcurrency)
	for _, sub := range claimed {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Go(func() {
			defer func() { <-slots }()
			p.renewWebhook(ctx, sub)
		})
	}
	wg.Wait()
}

// renewWebhook subscribes sub with its stored cursor and secret and stores
// the grant. For an owner whose runs are refused, the lease retries.
func (p *Server) renewWebhook(ctx context.Context, sub database.ChatAutomationMCPSubscription) {
	if !sub.MCPServerConfigID.Valid {
		p.terminateSubscription(ctx, sub.AutomationID, sql.NullTime{}, runReasonMCPServerUnavailable)
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
		// The gap goes first: appendInbox clears the error that the grant
		// records. A pending subscription has no cursor to fall behind.
		if res.Truncated {
			err := p.appendInbox(ctx, inboxAppend{AutomationID: sub.AutomationID, Cursor: res.Cursor, Truncated: true, MaxPending: maxPendingInboxRows})
			if err != nil && !errors.Is(err, errSubscriptionInactive) && !errors.Is(err, errInboxFull) {
				return err
			}
		}
		if _, err := p.db.UpdateChatAutomationWebhookGrant(ctx, chatautomation.WebhookGrant(sub.AutomationID, res, sub.RenewAt.Time, p.clock.Now("chatd", "automation-webhooks"))); err != nil {
			return xerrors.Errorf("store webhook grant: %w", err)
		}
		return nil
	})
	switch {
	case err == nil || errors.Is(err, errSubscriptionInactive):
		return
	case errors.Is(err, mcpclient.ErrEventsUnsupported):
		p.terminateSubscription(ctx, sub.AutomationID, sql.NullTime{}, runReasonMCPEventUnsupported)
		return
	}
	p.handleEventError(ctx, sub.AutomationID, sql.NullTime{}, err, conn, func(now time.Time, backoff time.Duration, message string) error {
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
	ctx, cancel := context.WithTimeout(dbauthz.AsChatd(context.WithoutCancel(ctx)), 2*webhookUnsubscribeTimeout)
	defer cancel()
	callCtx, cancelCall := context.WithTimeout(ctx, webhookUnsubscribeTimeout)
	defer cancelCall()
	_, err := p.withEvents(callCtx, sub, owner, func(session *mcpclient.EventsSession) error {
		var args map[string]any
		if err := json.Unmarshal(sub.Arguments, &args); err != nil {
			return xerrors.Errorf("decode arguments: %w", err)
		}
		return session.Unsubscribe(callCtx, sub.EventName, args, chatautomation.WebhookURL(p.accessURL, sub.AutomationID))
	})
	if err != nil {
		p.logger.Warn(ctx, "unsubscribe chat automation webhook", slog.F("automation_id", sub.AutomationID), slog.Error(err))
	}
	// A re-enable during the call may have subscribed again, which this
	// unsubscribe then ended, so the loop subscribes once more. This runs
	// even after the call used up its time.
	if automation, err := p.db.GetChatAutomationByID(ctx, sub.AutomationID); err == nil && automation.Enabled {
		now := p.clock.Now("chatd", "automation-webhooks")
		if err := p.db.SetChatAutomationWebhookRenewAt(ctx, database.SetChatAutomationWebhookRenewAtParams{AutomationID: sub.AutomationID, RenewAt: now, LastError: sql.NullString{}, Now: now}); err != nil {
			p.logger.Warn(ctx, "schedule chat automation webhook subscribe", slog.F("automation_id", sub.AutomationID), slog.Error(err))
		}
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
