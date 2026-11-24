package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatautomation"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

const (
	eventClaimBatch = 100
	// eventLease keeps other replicas from a claimed poll or drain. Work of
	// a replica that dies waits this long.
	eventLease = 2 * time.Minute
	// eventPollFloor applies whatever nextPollMs the server sends.
	eventPollFloor   = 30 * time.Second
	eventPollJitter  = 5 * time.Second
	eventPollCeiling = time.Hour
	eventMaxAge      = 24 * time.Hour
	// maxEventsPerMessage bounds one poll and one message.
	maxEventsPerMessage = 50
	// maxPendingInboxRows stops polling a subscription whose chat does not
	// take its events; the server keeps them meanwhile.
	maxPendingInboxRows  = 1000
	eventErrorBackoff    = time.Minute
	eventAuthBackoff     = 10 * time.Minute
	eventRefusedBackoff  = 10 * time.Minute
	eventPollConcurrency = 8
	maxLastErrorBytes    = 1024
)

// eventErrorAction is how a poll reacts to an error of the events extension:
// terminate the subscription with a reason, or try again after a backoff.
type eventErrorAction struct {
	terminate runReason
	backoff   time.Duration
}

// eventErrorActions maps the error codes of the events extension. Any other
// error retries after eventErrorBackoff. mcpclient already lists the event
// types again and retries once on HeaderMismatch.
var eventErrorActions = map[int64]eventErrorAction{
	-32024: {terminate: runReasonMCPForbidden},
	-32023: {terminate: runReasonMCPEventNotFound},
	-32026: {terminate: runReasonMCPEventUnsupported},
	-32602: {terminate: runReasonMCPInvalidArguments},
	-32025: {backoff: 5 * time.Minute},
	-32020: {backoff: time.Minute},
}

// inboxAppend is what one delivery adds to a subscription's inbox.
type inboxAppend struct {
	AutomationID uuid.UUID
	Events       []mcpclient.EventOccurrence
	// Cursor is stored when it is not nil.
	Cursor *string
	// Truncated adds a gap row: the server dropped events before these.
	Truncated bool
	// NextPollAt is stored when it is valid.
	NextPollAt sql.NullTime
}

// errSubscriptionInactive means that the subscription was deleted or
// terminated, so the delivery is dropped.
var errSubscriptionInactive = xerrors.New("the MCP event subscription is not active")

// appendInbox is the only writer of the event inbox. Every delivery mode
// calls it. It holds the automation lock and no chat row, so it cannot form a
// cycle with a drain, which locks the chat first.
func (p *Server) appendInbox(ctx context.Context, in inboxAppend) error {
	// The query reads the occurrences in their wire shape.
	events, err := json.Marshal(in.Events)
	if err != nil {
		return xerrors.Errorf("encode events: %w", err)
	}
	return p.db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatautomation.LockID(in.AutomationID)); err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		updated, err := tx.UpdateChatAutomationMCPCursor(ctx, database.UpdateChatAutomationMCPCursorParams{
			AutomationID: in.AutomationID,
			Cursor:       sql.NullString{String: ptr.NilToEmpty(in.Cursor), Valid: in.Cursor != nil},
			NextPollAt:   in.NextPollAt,
			Now:          p.clock.Now("chatd", "automation-events"),
		})
		if err != nil {
			return xerrors.Errorf("update subscription: %w", err)
		}
		if updated == 0 {
			return errSubscriptionInactive
		}
		if in.Truncated {
			if err := tx.InsertChatAutomationInboxNotice(ctx, database.InsertChatAutomationInboxNoticeParams{
				AutomationID: in.AutomationID,
				Kind:         database.ChatAutomationInboxKindGap,
				Detail:       "The MCP server dropped events before the ones that follow, so some events are missing.",
			}); err != nil {
				return xerrors.Errorf("insert gap: %w", err)
			}
		}
		if _, err := tx.InsertChatAutomationInboxEvents(ctx, database.InsertChatAutomationInboxEventsParams{
			AutomationID: in.AutomationID,
			Events:       events,
		}); err != nil {
			return xerrors.Errorf("insert events: %w", err)
		}
		return nil
	}, nil)
}

// terminateSubscription ends a subscription for good, tells the chat in its
// inbox and records the reason as a refused run. Terminating an inactive
// subscription changes nothing.
func (p *Server) terminateSubscription(ctx context.Context, automationID uuid.UUID, reason runReason) {
	err := p.db.InTx(func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, chatautomation.LockID(automationID)); err != nil {
			return xerrors.Errorf("lock chat automation: %w", err)
		}
		terminated, err := tx.TerminateChatAutomationMCPSubscription(ctx, database.TerminateChatAutomationMCPSubscriptionParams{
			AutomationID: automationID,
			Reason:       string(reason),
			Now:          p.clock.Now("chatd", "automation-events"),
		})
		if err != nil {
			return xerrors.Errorf("terminate subscription: %w", err)
		}
		if terminated == 0 {
			return errSubscriptionInactive
		}
		return tx.InsertChatAutomationInboxNotice(ctx, database.InsertChatAutomationInboxNoticeParams{
			AutomationID: automationID,
			Kind:         database.ChatAutomationInboxKindTerminated,
			Detail:       "The subscription ended and receives no more events: " + string(reason),
		})
	}, nil)
	if err != nil {
		if !errors.Is(err, errSubscriptionInactive) && ctx.Err() == nil {
			p.logger.Warn(ctx, "terminate chat automation subscription", slog.F("automation_id", automationID), slog.Error(err))
		}
		return
	}
	p.recordEventRefusal(ctx, automationID, reason)
}

// recordEventRefusal records reason as the last run unless it already is.
func (p *Server) recordEventRefusal(ctx context.Context, automationID uuid.UUID, reason runReason) {
	automation, err := p.db.GetChatAutomationByID(ctx, automationID)
	if err != nil || automation.LastRunReason.String == string(reason) {
		return
	}
	p.recordAutomationRun(ctx, automation, AutomationRunResult{
		Outcome: runReasonOutcome[reason],
		Reason:  string(reason),
		ChatID:  automation.TargetChatID,
	})
}

// tickEventPolls polls every subscription that is due, with one session per
// owner and MCP server.
func (p *Server) tickEventPolls(ctx context.Context, now time.Time) {
	//nolint:gocritic // The poller reads and writes the subscriptions of every owner.
	ctx = dbauthz.AsChatd(ctx)
	claimed, err := p.db.ClaimDueChatAutomationMCPPolls(ctx, database.ClaimDueChatAutomationMCPPollsParams{
		Now:        now,
		MaxPending: maxPendingInboxRows,
		LimitCount: eventClaimBatch,
		LeaseUntil: now.Add(eventLease),
	})
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Warn(ctx, "claim due chat automation polls", slog.Error(err))
		}
		return
	}
	type group struct{ owner, config uuid.UUID }
	groups := make(map[group][]database.ClaimDueChatAutomationMCPPollsRow)
	for _, sub := range claimed {
		if !sub.MCPServerConfigID.Valid {
			p.terminateSubscription(ctx, sub.AutomationID, runReasonMCPServerUnavailable)
			continue
		}
		key := group{owner: sub.OwnerID, config: sub.MCPServerConfigID.UUID}
		groups[key] = append(groups[key], sub)
	}
	var eg errgroup.Group
	eg.SetLimit(eventPollConcurrency)
	for key, subs := range groups {
		eg.Go(func() error {
			p.pollEventGroup(ctx, key.owner, key.config, subs, now)
			return nil
		})
	}
	_ = eg.Wait()
}

// pollEventGroup polls the subscriptions of one owner on one MCP server.
func (p *Server) pollEventGroup(ctx context.Context, owner, configID uuid.UUID, subs []database.ClaimDueChatAutomationMCPPollsRow, now time.Time) {
	cfg, err := p.db.GetMCPServerConfigByID(ctx, configID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		cfg.Enabled = false
	}
	var conn *eventsConn
	if err == nil {
		conn, err = p.openEvents(ctx, cfg, owner)
	}
	if err != nil {
		for _, sub := range subs {
			p.handlePollError(ctx, sub.AutomationID, err, conn, now)
		}
		return
	}
	defer conn.close()
	for _, sub := range subs {
		if !conn.session.Supported() {
			p.terminateSubscription(ctx, sub.AutomationID, runReasonMCPEventUnsupported)
			continue
		}
		p.pollSubscription(ctx, conn, sub, now)
	}
}

// pollSubscription polls one subscription and appends what it returns. It
// polls again at once while the server has more, up to the pending limit.
func (p *Server) pollSubscription(ctx context.Context, conn *eventsConn, sub database.ClaimDueChatAutomationMCPPollsRow, now time.Time) {
	var args map[string]any
	_ = json.Unmarshal(sub.Arguments, &args)
	var cursor *string
	if sub.Cursor.Valid {
		cursor = &sub.Cursor.String
	}
	for page := 1; ; page++ {
		res, err := conn.session.Poll(ctx, mcpclient.PollParams{
			Name:      sub.EventName,
			Arguments: args,
			Cursor:    cursor,
			MaxAgeMs:  ptr.Ref(eventMaxAge.Milliseconds()),
			MaxEvents: ptr.Ref(int64(maxEventsPerMessage)),
		})
		if err != nil {
			p.handlePollError(ctx, sub.AutomationID, err, conn, now)
			return
		}
		in := inboxAppend{AutomationID: sub.AutomationID, Events: res.Events, Cursor: res.Cursor, Truncated: res.Truncated}
		more := res.HasMore && page < maxPendingInboxRows/maxEventsPerMessage
		if !more {
			in.NextPollAt = sql.NullTime{Time: nextPollAt(now, res.NextPollMs), Valid: true}
		}
		if err := p.appendInbox(ctx, in); err != nil {
			if !errors.Is(err, errSubscriptionInactive) && ctx.Err() == nil {
				p.logger.Warn(ctx, "append chat automation events", slog.F("automation_id", sub.AutomationID), slog.Error(err))
			}
			return
		}
		if !more {
			return
		}
		if res.Cursor != nil {
			cursor = res.Cursor
		}
	}
}

// nextPollAt follows the server's nextPollMs between the floor and the
// ceiling, with jitter so that subscriptions do not poll in step.
func nextPollAt(now time.Time, nextPollMs int64) time.Time {
	wait := min(max(time.Duration(nextPollMs)*time.Millisecond, eventPollFloor), eventPollCeiling)
	//nolint:gosec // Jitter spreads polls; it needs no secrecy.
	return now.Add(wait + rand.N(eventPollJitter))
}

// handlePollError terminates the subscription or schedules a retry, as
// eventErrorActions says.
func (p *Server) handlePollError(ctx context.Context, automationID uuid.UUID, err error, conn *eventsConn, now time.Time) {
	if ctx.Err() != nil {
		return
	}
	action := eventErrorAction{backoff: eventErrorBackoff}
	var eventsErr *mcpclient.EventsError
	authFailed := errors.Is(err, chatautomation.ErrMCPAuthRequired) || (conn != nil && conn.unauthorized.Load())
	switch {
	case authFailed:
		action.backoff = eventAuthBackoff
	case errors.As(err, &eventsErr):
		if a, ok := eventErrorActions[eventsErr.Code]; ok {
			action = a
		}
	}
	if action.terminate != "" {
		p.terminateSubscription(ctx, automationID, action.terminate)
		return
	}
	message := err.Error()
	if len(message) > maxLastErrorBytes {
		message = message[:maxLastErrorBytes]
	}
	if _, uerr := p.db.UpdateChatAutomationMCPCursor(ctx, database.UpdateChatAutomationMCPCursorParams{
		AutomationID: automationID,
		NextPollAt:   sql.NullTime{Time: now.Add(action.backoff), Valid: true},
		LastError:    sql.NullString{String: message, Valid: true},
		Now:          now,
	}); uerr != nil {
		p.logger.Warn(ctx, "record chat automation poll error", slog.F("automation_id", automationID), slog.Error(uerr))
	}
	if authFailed {
		p.recordEventRefusal(ctx, automationID, runReasonMCPAuthRequired)
	}
}

// tickEventDrains sends the pending inbox rows of each subscription whose
// chat can take them.
func (p *Server) tickEventDrains(ctx context.Context, now time.Time) {
	//nolint:gocritic // The drain reads the inboxes of every owner and sends as each owner.
	ctx = dbauthz.AsChatd(ctx)
	claimed, err := p.db.ClaimChatAutomationInboxDrains(ctx, database.ClaimChatAutomationInboxDrainsParams{
		Now:        now,
		LimitCount: eventClaimBatch,
		LeaseUntil: now.Add(eventLease),
	})
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Warn(ctx, "claim chat automation inbox drains", slog.Error(err))
		}
		return
	}
	for _, sub := range claimed {
		p.drainInbox(ctx, sub.AutomationID, now)
	}
}

// drainInbox sends up to maxEventsPerMessage pending rows in one message. A
// busy chat keeps them for the next tick.
func (p *Server) drainInbox(ctx context.Context, automationID uuid.UUID, now time.Time) {
	retry := now.Add(automationTickInterval)
	automation, err := p.db.GetChatAutomationByID(ctx, automationID)
	var rows []database.ChatAutomationEventInbox
	if err == nil {
		rows, err = p.db.GetPendingChatAutomationInboxRows(ctx, database.GetPendingChatAutomationInboxRowsParams{
			AutomationID: automationID,
			LimitCount:   maxEventsPerMessage,
		})
	}
	if err == nil && len(rows) > 0 {
		var result AutomationRunResult
		result, err = p.runAutomation(ctx, automation, automationRun{trigger: automationTriggerMCPEvent, inbox: rows})
		if result.Outcome == database.ChatAutomationRunOutcomeRefused {
			retry = now.Add(eventRefusedBackoff)
		}
	}
	if err != nil && !errors.Is(err, ErrAutomationChanged) && !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
		p.logger.Warn(ctx, "drain chat automation inbox", slog.F("automation_id", automationID), slog.Error(err))
	}
	if err := p.db.SetChatAutomationMCPDrainAfter(ctx, database.SetChatAutomationMCPDrainAfterParams{
		AutomationID: automationID,
		DrainAfter:   retry,
	}); err != nil && ctx.Err() == nil {
		p.logger.Warn(ctx, "set chat automation drain time", slog.F("automation_id", automationID), slog.Error(err))
	}
}

// eventsConn is an events session that remembers whether the server
// answered 401, which no error type carries.
type eventsConn struct {
	session      *mcpclient.EventsSession
	base         http.RoundTripper
	unauthorized atomic.Bool
}

func (c *eventsConn) close() {
	_ = c.session.Close()
}

func (c *eventsConn) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.base.RoundTrip(req)
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		c.unauthorized.Store(true)
	}
	return resp, err
}

// openEvents opens an events session to cfg as owner, with the owner's
// OAuth2 token refreshed when the server uses one. Polls and the agent tool
// send the same identity, so the server sees one principal. The returned
// conn is set when only the session failed.
func (p *Server) openEvents(ctx context.Context, cfg database.MCPServerConfig, owner uuid.UUID) (*eventsConn, error) {
	if !cfg.Enabled {
		return nil, xerrors.New("the MCP server is disabled")
	}
	var tok *database.MCPServerUserToken
	if cfg.AuthType == "oauth2" {
		row, err := p.db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{MCPServerConfigID: cfg.ID, UserID: owner})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, chatautomation.ErrMCPAuthRequired
		}
		if err != nil {
			return nil, xerrors.Errorf("get MCP server token: %w", err)
		}
		row, err = mcpclient.RefreshUserToken(ctx, p.logger, p.db, p.mcpHTTPClient, cfg, row)
		if err != nil {
			return nil, err
		}
		if row.AccessToken == "" || row.OauthRefreshFailureReason != "" {
			return nil, chatautomation.ErrMCPAuthRequired
		}
		tok = &row
	}
	hc := *p.mcpHTTPClient
	if hc.Transport == nil {
		hc = *mcpclient.NewHTTPClient(p.mcpHTTPClient)
	}
	conn := &eventsConn{base: hc.Transport}
	hc.Transport = conn
	session, err := mcpclient.ConnectEvents(ctx, p.logger, cfg, tok, map[string]string{chatprovider.HeaderCoderOwnerID: owner.String()}, &hc)
	if err != nil {
		return conn, err
	}
	conn.session = session
	return conn, nil
}
