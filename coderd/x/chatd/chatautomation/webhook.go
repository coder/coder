package chatautomation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/xurl"
	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

const (
	// webhookTTL is the lifetime that a subscribe asks for; the grant decides.
	webhookTTL = time.Hour
	// webhookMaxAge bounds the replay of a subscribe with a stored cursor.
	webhookMaxAge = 24 * time.Hour
	// webhookPendingLease leaves a pending subscription to its subscribe.
	webhookPendingLease = 2 * time.Minute
	// codeCallbackEndpointError means that the callback URL failed.
	codeCallbackEndpointError = -32027
)

// deliveryOrder lists the delivery modes that subscribe picks from, best
// first. Push would sit between webhook and poll; it is not built.
var deliveryOrder = []struct {
	mode     mcpclient.DeliveryMode
	eligible func(accessURL *url.URL) bool
}{
	{mcpclient.DeliveryWebhook, xurl.IsPublicHTTPS},
	{mcpclient.DeliveryPoll, func(*url.URL) bool { return true }},
}

// deliveryModes returns the modes of deliveryOrder that an event type offers
// and that this deployment can receive.
func deliveryModes(offered []mcpclient.DeliveryMode, accessURL *url.URL) []mcpclient.DeliveryMode {
	var modes []mcpclient.DeliveryMode
	for _, d := range deliveryOrder {
		if slices.Contains(offered, d.mode) && d.eligible(accessURL) {
			modes = append(modes, d.mode)
		}
	}
	return modes
}

// Unsubscriber ends a webhook subscription on its MCP server, best effort.
type Unsubscriber func(ctx context.Context, owner uuid.UUID, sub database.ChatAutomationMCPSubscription)

// WebhookURL returns an automation's callback URL. The path names the
// automation, so each subscription has its own server-side key and secret.
func WebhookURL(accessURL *url.URL, automationID uuid.UUID) string {
	if accessURL == nil {
		accessURL = &url.URL{}
	}
	return accessURL.JoinPath("api/experimental/chats/automations", automationID.String(), "mcp-events").String()
}

// WebhookSubscribeParams returns the events/subscribe params that create or
// refresh sub, from its stored cursor.
func WebhookSubscribeParams(accessURL *url.URL, sub database.ChatAutomationMCPSubscription, secret string) (mcpclient.SubscribeParams, error) {
	var args map[string]any
	if err := json.Unmarshal(sub.Arguments, &args); err != nil {
		return mcpclient.SubscribeParams{}, xerrors.Errorf("decode arguments: %w", err)
	}
	params := mcpclient.SubscribeParams{
		Name:      sub.EventName,
		Arguments: args,
		Delivery:  mcpclient.WebhookDelivery{URL: WebhookURL(accessURL, sub.AutomationID), Secret: secret},
		TTLMs:     ptr.Ref(webhookTTL.Milliseconds()),
	}
	if sub.Cursor.Valid {
		params.Cursor = &sub.Cursor.String
		params.MaxAgeMs = ptr.Ref(webhookMaxAge.Milliseconds())
	}
	return params, nil
}

// WebhookGrant returns the update that stores a subscribe result, made while
// the subscription's renew_at was lease. The subscription renews when a
// quarter of the grant is left.
func WebhookGrant(automationID uuid.UUID, res mcpclient.SubscribeResult, lease, now time.Time) database.UpdateChatAutomationWebhookGrantParams {
	// A null grant means no expiry, which only a ttlMs of null may ask for,
	// and no grant outlives the TTL that Coder asked for.
	refreshBefore := now.Add(webhookTTL)
	if res.RefreshBefore != nil && res.RefreshBefore.Before(refreshBefore) {
		refreshBefore = *res.RefreshBefore
	}
	renewAt := refreshBefore.Add(-max(refreshBefore.Sub(now), 0) / 4)
	grant := database.UpdateChatAutomationWebhookGrantParams{
		AutomationID:          automationID,
		WebhookSubscriptionID: res.ID,
		RefreshBefore:         refreshBefore,
		RenewAt:               renewAt,
		Lease:                 lease,
		Cursor:                sql.NullString{},
		LastError:             sql.NullString{},
		Now:                   now,
	}
	if res.Cursor != nil {
		grant.Cursor = sql.NullString{String: *res.Cursor, Valid: true}
	}
	if res.DeliveryStatus != nil && res.DeliveryStatus.LastError != nil {
		grant.LastError = sql.NullString{String: fmt.Sprintf("webhook delivery error: %.64s", *res.DeliveryStatus.LastError), Valid: true}
	}
	return grant
}

// subscribeWebhook creates the automation with a pending subscription and
// its secret before it subscribes, so that the receiver can answer the
// verification during the call. A failed subscribe deletes the automation,
// unless someone disabled or deleted it meanwhile.
func subscribeWebhook(ctx context.Context, options ToolOptions, session *mcpclient.EventsSession, cfg database.MCPServerConfig, args toolArgs, arguments json.RawMessage) (database.ChatAutomation, database.ChatAutomationMCPSubscription, error) {
	secret, err := chatwebhook.GenerateSecret()
	if err != nil {
		return database.ChatAutomation{}, database.ChatAutomationMCPSubscription{}, err
	}
	var sub database.ChatAutomationMCPSubscription
	created, err := createEventAutomation(ctx, options.DB, options.Chat, args.Name, args.Prompt, func(tx database.Store, created database.ChatAutomation) error {
		sub, err = tx.InsertChatAutomationMCPSubscription(ctx, database.InsertChatAutomationMCPSubscriptionParams{
			AutomationID:      created.ID,
			MCPServerConfigID: cfg.ID,
			EventName:         args.Event,
			Arguments:         arguments,
			Delivery:          database.ChatAutomationEventDeliveryWebhook,
			State:             database.ChatAutomationSubscriptionStatePending,
			Cursor:            sql.NullString{},
			NextPollAt:        sql.NullTime{},
			RenewAt:           sql.NullTime{Time: options.Clock.Now().Add(webhookPendingLease), Valid: true},
		})
		if err != nil {
			return xerrors.Errorf("insert subscription: %w", err)
		}
		return tx.InsertChatAutomationWebhookSecret(ctx, database.InsertChatAutomationWebhookSecretParams{AutomationID: created.ID, Secret: secret, SecretKeyID: sql.NullString{}})
	})
	if err != nil {
		return database.ChatAutomation{}, sub, err
	}
	params, err := WebhookSubscribeParams(options.AccessURL, sub, secret)
	if err == nil {
		var res mcpclient.SubscribeResult
		res, err = session.Subscribe(ctx, params)
		if err == nil {
			_, err = options.DB.UpdateChatAutomationWebhookGrant(ctx, WebhookGrant(created.ID, res, sub.RenewAt.Time, options.Clock.Now()))
		}
	}
	if err != nil {
		if derr := discardPending(ctx, options.DB, created.ID); derr != nil {
			return database.ChatAutomation{}, sub, derr
		}
		return database.ChatAutomation{}, sub, err
	}
	sub.State = database.ChatAutomationSubscriptionStateActive
	return created, sub, nil
}

// discardPending deletes the automation of a failed webhook subscribe, whose
// deliveries the receiver then refuses. A disable or delete that committed
// meanwhile stands, and errSubscribeInterrupted stops a poll fallback.
func discardPending(ctx context.Context, db database.Store, id uuid.UUID) error {
	err := db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, LockID(id)); err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		automation, err := tx.GetChatAutomationByID(ctx, id)
		if httpapi.Is404Error(err) || (err == nil && !automation.Enabled) {
			return errSubscribeInterrupted
		}
		if err != nil {
			return err
		}
		return tx.DeleteChatAutomationByID(ctx, id)
	}, nil)
	if errors.Is(err, errSubscribeInterrupted) {
		return errSubscribeInterrupted
	}
	return err
}

// webhookSubscription returns automation's webhook subscription when it has
// one that a server may still deliver to.
func webhookSubscription(ctx context.Context, tx database.Store, automation database.ChatAutomation) (database.ChatAutomationMCPSubscription, bool, error) {
	if automation.Kind != database.ChatAutomationKindMcpEvent {
		return database.ChatAutomationMCPSubscription{}, false, nil
	}
	//nolint:gocritic // The caller read the automation as the actor.
	subs, err := tx.GetChatAutomationMCPSubscriptionsByAutomationIDs(dbauthz.AsSystemRestricted(ctx), []uuid.UUID{automation.ID})
	if err != nil {
		return database.ChatAutomationMCPSubscription{}, false, xerrors.Errorf("get subscription: %w", err)
	}
	found := len(subs) == 1 && subs[0].Delivery == database.ChatAutomationEventDeliveryWebhook &&
		subs[0].State != database.ChatAutomationSubscriptionStateTerminated
	if !found {
		return database.ChatAutomationMCPSubscription{}, false, nil
	}
	return subs[0], true, nil
}
