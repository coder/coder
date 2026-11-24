package chatautomation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/xurl"
	"github.com/coder/coder/v2/coderd/x/chatd/chatwebhook"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

const (
	// WebhookTTL is the lifetime that a webhook subscribe suggests. The
	// server's grant decides.
	WebhookTTL = time.Hour
	// webhookMaxAge bounds the replay of a subscribe with a stored cursor.
	webhookMaxAge = 24 * time.Hour
	// webhookPendingLease is how long the automation loop leaves a pending
	// subscription to the subscribe that created it.
	webhookPendingLease = 2 * time.Minute
	// minWebhookRenewal keeps a short grant from renewing every tick.
	minWebhookRenewal = 30 * time.Second
	// codeCallbackEndpointError means that the server could not verify or
	// reach the callback URL.
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

// WebhookURL returns the callback URL of an automation's webhook deliveries.
// The path names the automation, so that each subscription has its own
// server-side key and secret.
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
		TTLMs:     ptr.Ref(WebhookTTL.Milliseconds()),
	}
	if sub.Cursor.Valid {
		params.Cursor = &sub.Cursor.String
		params.MaxAgeMs = ptr.Ref(webhookMaxAge.Milliseconds())
	}
	return params, nil
}

// WebhookGrant returns the update that stores a subscribe result. The
// subscription renews when a quarter of the grant is left.
func WebhookGrant(automationID uuid.UUID, res mcpclient.SubscribeResult, now time.Time) database.UpdateChatAutomationWebhookGrantParams {
	// A null grant means no expiry, which only a ttlMs of null may ask for.
	refreshBefore := now.Add(WebhookTTL)
	if res.RefreshBefore != nil {
		refreshBefore = *res.RefreshBefore
	}
	renewAt := refreshBefore.Add(-refreshBefore.Sub(now) / 4)
	if floor := now.Add(minWebhookRenewal); renewAt.Before(floor) {
		renewAt = floor
	}
	grant := database.UpdateChatAutomationWebhookGrantParams{
		AutomationID:          automationID,
		WebhookSubscriptionID: res.ID,
		RefreshBefore:         refreshBefore,
		RenewAt:               renewAt,
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

// subscribeWebhook creates the automation with a pending webhook
// subscription and its secret, and then subscribes, so that the receiver can
// answer the server's verification during the call. A failed subscribe
// deletes the automation.
func subscribeWebhook(ctx context.Context, options ToolOptions, session *mcpclient.EventsSession, cfg database.MCPServerConfig, args toolArgs, arguments json.RawMessage) (database.ChatAutomation, database.ChatAutomationMCPSubscription, error) {
	secret := chatwebhook.GenerateSecret()
	var sub database.ChatAutomationMCPSubscription
	created, err := createEventAutomation(ctx, options.DB, options.Chat, args.Name, args.Prompt, func(tx database.Store, created database.ChatAutomation) error {
		var err error
		sub, err = tx.InsertChatAutomationMCPSubscription(ctx, database.InsertChatAutomationMCPSubscriptionParams{
			AutomationID:      created.ID,
			MCPServerConfigID: cfg.ID,
			EventName:         args.Event,
			Arguments:         arguments,
			Delivery:          database.ChatAutomationEventDeliveryWebhook,
			State:             database.ChatAutomationSubscriptionStatePending,
			RenewAt:           sql.NullTime{Time: options.Clock.Now().Add(webhookPendingLease), Valid: true},
		})
		if err != nil {
			return xerrors.Errorf("insert subscription: %w", err)
		}
		return tx.InsertChatAutomationWebhookSecret(ctx, database.InsertChatAutomationWebhookSecretParams{AutomationID: created.ID, Secret: secret})
	})
	if err != nil {
		return database.ChatAutomation{}, sub, err
	}
	params, err := WebhookSubscribeParams(options.AccessURL, sub, secret)
	if err == nil {
		var res mcpclient.SubscribeResult
		res, err = session.Subscribe(ctx, params)
		if err == nil {
			_, err = options.DB.UpdateChatAutomationWebhookGrant(ctx, WebhookGrant(created.ID, res, options.Clock.Now()))
		}
	}
	if err != nil {
		// The server keeps nothing that a failed subscribe asked for, and the
		// receiver refuses deliveries to a deleted automation.
		if _, derr := Delete(ctx, options.DB, created.OrganizationID, created.ID, nil); derr != nil {
			options.Logger.Warn(ctx, "delete chat automation after a failed webhook subscribe", slog.F("automation_id", created.ID), slog.Error(derr))
		}
		return database.ChatAutomation{}, sub, err
	}
	sub.State = database.ChatAutomationSubscriptionStateActive
	return created, sub, nil
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
