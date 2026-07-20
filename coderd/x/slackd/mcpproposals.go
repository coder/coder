package slackd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// proposalTTL is how long a pending proposal stays actionable.
	// Older pending proposals are treated as expired and lazily
	// pruned.
	proposalTTL = 24 * time.Hour

	// Default auth-poll cadence after an accepted oauth2 proposal.
	defaultAuthPollInterval = 3 * time.Second
	defaultAuthPollTimeout  = 5 * time.Minute

	proposalStatusPending  = "pending"
	proposalStatusAccepted = "accepted"
	proposalStatusRejected = "rejected"

	// Card bar colors by proposal state.
	proposalColorSuccess = "#2EB67D"
	proposalColorGray    = "#9CA3AF"
	proposalColorPartial = "#ECB22E"

	// proposalGoneMessage is returned for missing, rejected, and
	// expired proposals. It intentionally does not distinguish the
	// cases.
	proposalGoneMessage = "This MCP server proposal has expired or was already handled."
	// proposalForbiddenMessage is returned when a user other than the
	// requester tries to act on a proposal. It intentionally does not
	// identify the required user.
	proposalForbiddenMessage = "Another user must authorize this MCP server proposal."
)

// errProposalHandled aborts a proposal transition because the row is no
// longer pending (or has expired).
var errProposalHandled = xerrors.New("proposal expired or already handled")

var errInvalidProposalSecrets = xerrors.New("invalid proposal secret values")

// ProposalChat is the subset of *chatd.Server used by the MCP proposal
// handlers.
type ProposalChat interface {
	SendMessage(ctx context.Context, opts chatd.SendMessageOptions) (chatd.SendMessageResult, error)
	AddChatMCPServerID(ctx context.Context, chatID, serverID uuid.UUID) (database.Chat, error)
}

// ProposalsAPIOptions configures a ProposalsAPI.
type ProposalsAPIOptions struct {
	Logger   slog.Logger
	Database database.Store
	// Chat submits proposal-outcome system messages and enables
	// accepted servers for the proposing chat. May be nil when chatd
	// is disabled; without chatd no proposals can exist, so handlers
	// only ever 404.
	Chat ProposalChat
	// WebAPI updates proposal cards in Slack and posts ephemeral
	// replies. May be nil when the Slack integration is not
	// configured; card updates are then skipped.
	WebAPI WebAPI
	// AccessURL builds OAuth2 connect URLs.
	AccessURL *url.URL
	// ResolveSlackUser maps the Slack user clicking a Cancel button to
	// a Coder user id, so the click can be authorized against the
	// proposal's requester. Nil when the Slack integration is not
	// configured; Cancel clicks are then denied.
	ResolveSlackUser func(ctx context.Context, slackUserID string) (uuid.UUID, error)
	// HTTPClient performs OAuth2 discovery and Dynamic Client
	// Registration for accepted oauth2 proposals. Defaults to a
	// 30-second-timeout client.
	HTTPClient *http.Client
	// BackgroundCtx carries the authorization identity
	// (dbauthz.AsSlackd) for notifications, card updates, and auth
	// polling that outlive the triggering HTTP request. Defaults to
	// context.Background(), which fails database authorization; pass
	// a real context in production.
	BackgroundCtx context.Context

	// AuthPollInterval/AuthPollTimeout bound the post-accept OAuth2
	// authentication poll. Fixed except in tests.
	AuthPollInterval time.Duration
	AuthPollTimeout  time.Duration
}

// ProposalsAPI implements the MCP server proposal endpoints (get,
// accept, reject) and the Slack Cancel-button flow. It is constructed
// even when the Slack integration is not configured so the routes
// exist; without Slack no proposals can be created and lookups 404.
type ProposalsAPI struct {
	logger           slog.Logger
	db               database.Store
	chat             ProposalChat
	webAPI           WebAPI
	accessURL        *url.URL
	httpClient       *http.Client
	resolveSlackUser func(ctx context.Context, slackUserID string) (uuid.UUID, error)

	authPollInterval time.Duration
	authPollTimeout  time.Duration

	bgCtx    context.Context
	bgCancel context.CancelFunc
	wg       sync.WaitGroup
}

// NewProposalsAPI validates the options and returns a ProposalsAPI.
// Call Close to stop background auth polling.
func NewProposalsAPI(opts ProposalsAPIOptions) (*ProposalsAPI, error) {
	if opts.Database == nil {
		return nil, xerrors.New("slackd: proposals api requires a database")
	}
	httpClient := mcpOAuth2HTTPClient(opts.HTTPClient)
	pollInterval := opts.AuthPollInterval
	if pollInterval <= 0 {
		pollInterval = defaultAuthPollInterval
	}
	pollTimeout := opts.AuthPollTimeout
	if pollTimeout <= 0 {
		pollTimeout = defaultAuthPollTimeout
	}
	bgCtx := opts.BackgroundCtx
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	bgCtx, bgCancel := context.WithCancel(bgCtx)
	return &ProposalsAPI{
		logger:           opts.Logger,
		db:               opts.Database,
		chat:             opts.Chat,
		webAPI:           opts.WebAPI,
		accessURL:        opts.AccessURL,
		httpClient:       httpClient,
		resolveSlackUser: opts.ResolveSlackUser,
		authPollInterval: pollInterval,
		authPollTimeout:  pollTimeout,
		bgCtx:            bgCtx,
		bgCancel:         bgCancel,
	}, nil
}

// Close stops background auth polling and waits for in-flight work.
func (p *ProposalsAPI) Close() {
	p.bgCancel()
	p.wg.Wait()
}

// expired reports whether a pending proposal is past its TTL.
func proposalExpired(proposal database.MCPServerProposal) bool {
	return time.Since(proposal.CreatedAt) > proposalTTL
}

// pruneExpired lazily deletes proposals past the TTL. Best-effort.
func (p *ProposalsAPI) pruneExpired(ctx context.Context) {
	//nolint:gocritic // Expired-proposal pruning spans all chats.
	if err := p.db.DeleteExpiredMCPServerProposals(dbauthz.AsSystemRestricted(ctx), time.Now().Add(-proposalTTL)); err != nil {
		p.logger.Warn(ctx, "prune expired mcp server proposals", slog.Error(err))
	}
}

// loadAuthorizedProposal loads the proposal and its chat, enforcing
// that the caller is the proposal's requester (the Coder user the
// proposing Slack sender resolved to, recorded at creation time).
// Wrong callers get a 403 that does not identify the required user;
// missing proposals get the friendly gone message.
//
//nolint:revive // Helper writes to ResponseWriter on failure.
func (p *ProposalsAPI) loadAuthorizedProposal(rw http.ResponseWriter, r *http.Request, userID uuid.UUID) (database.MCPServerProposal, database.Chat, bool) {
	ctx := r.Context()

	proposalID, err := uuid.Parse(chi.URLParam(r, "mcpProposal"))
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid MCP server proposal ID.",
			Detail:  err.Error(),
		})
		return database.MCPServerProposal{}, database.Chat{}, false
	}

	p.pruneExpired(ctx)

	// The requester check below authorizes access; regular users
	// cannot read arbitrary chats or proposals directly.
	//nolint:gocritic // Ownership is checked explicitly below.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	proposal, err := p.db.GetMCPServerProposalByID(sysCtx, proposalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{Message: proposalGoneMessage})
			return database.MCPServerProposal{}, database.Chat{}, false
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server proposal.",
			Detail:  err.Error(),
		})
		return database.MCPServerProposal{}, database.Chat{}, false
	}
	chat, err := p.db.GetChatByID(sysCtx, proposal.ChatID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get MCP server proposal.",
			Detail:  err.Error(),
		})
		return database.MCPServerProposal{}, database.Chat{}, false
	}
	if proposal.RequesterID != userID {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{Message: proposalForbiddenMessage})
		return database.MCPServerProposal{}, database.Chat{}, false
	}
	return proposal, chat, true
}

// GetProposal handles GET /api/v2/mcp-server-proposals/{mcpProposal}.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (p *ProposalsAPI) GetProposal(rw http.ResponseWriter, r *http.Request) {
	p.getProposal(rw, r, httpmw.APIKey(r).UserID)
}

//nolint:revive // HTTP handler writes to ResponseWriter.
func (p *ProposalsAPI) getProposal(rw http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	ctx := r.Context()
	proposal, _, ok := p.loadAuthorizedProposal(rw, r, userID)
	if !ok {
		return
	}
	if proposal.Status == proposalStatusRejected ||
		(proposal.Status == proposalStatusPending && proposalExpired(proposal)) {
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{Message: proposalGoneMessage})
		return
	}

	req, err := parseProposalRequest(proposal)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to decode MCP server proposal.",
			Detail:  err.Error(),
		})
		return
	}

	resp := convertMCPServerProposal(proposal, req)
	if proposal.Status == proposalStatusAccepted && proposal.MCPServerConfigID.Valid {
		//nolint:gocritic // The requester check already authorized access.
		sysCtx := dbauthz.AsSystemRestricted(ctx)
		config, err := p.db.GetMCPServerConfigByID(sysCtx, proposal.MCPServerConfigID.UUID)
		if err == nil {
			resp.MCPServerConfigID = config.ID
			resp.Authenticated = p.configAuthenticated(sysCtx, config, proposal.RequesterID)
			if config.AuthType == "oauth2" {
				resp.ConnectURL = chattool.MCPOAuth2ConnectURL(p.accessURL, config.ID)
			}
		} else {
			p.logger.Warn(ctx, "load accepted proposal config", slog.Error(err))
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// AcceptProposal handles POST
// /api/v2/mcp-server-proposals/{mcpProposal}/accept. It is idempotent:
// the accepting transition runs once under a FOR UPDATE row lock, so
// concurrent and repeated accepts return the same created config and
// notifications fire only once.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (p *ProposalsAPI) AcceptProposal(rw http.ResponseWriter, r *http.Request) {
	p.acceptProposal(rw, r, httpmw.APIKey(r).UserID)
}

//nolint:revive // HTTP handler writes to ResponseWriter.
func (p *ProposalsAPI) acceptProposal(rw http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	ctx := r.Context()
	secretValues, ok := readProposalSecretValues(rw, r)
	if !ok {
		return
	}
	proposal, chat, ok := p.loadAuthorizedProposal(rw, r, userID)
	if !ok {
		return
	}

	// Creating the personal config requires deployment-config writes
	// regular users do not have; the requester check above scopes
	// the operation.
	//nolint:gocritic // See above.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	var (
		config      database.MCPServerConfig
		firstAccept bool
	)
	err := p.db.InTx(func(tx database.Store) error {
		locked, err := tx.GetMCPServerProposalByIDForUpdate(sysCtx, proposal.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errProposalHandled
			}
			return xerrors.Errorf("lock proposal: %w", err)
		}
		if locked.Status == proposalStatusAccepted && locked.MCPServerConfigID.Valid {
			config, err = tx.GetMCPServerConfigByID(sysCtx, locked.MCPServerConfigID.UUID)
			if err != nil {
				return xerrors.Errorf("load accepted config: %w", err)
			}
			return nil
		}
		if locked.Status != proposalStatusPending || proposalExpired(locked) {
			return errProposalHandled
		}
		req, err := parseProposalRequest(locked)
		if err != nil {
			return xerrors.Errorf("decode proposal request: %w", err)
		}
		req, err = resolveProposalSecrets(req, secretValues)
		if err != nil {
			return err
		}
		config, err = p.createProposedMCPServerConfig(sysCtx, tx, locked.RequesterID, req)
		if err != nil {
			return xerrors.Errorf("create mcp server config: %w", err)
		}
		if _, err := tx.UpdateMCPServerProposalStatus(sysCtx, database.UpdateMCPServerProposalStatusParams{
			ID:                locked.ID,
			Status:            proposalStatusAccepted,
			MCPServerConfigID: uuid.NullUUID{UUID: config.ID, Valid: true},
			AcceptedAt:        sql.NullTime{Time: time.Now(), Valid: true},
		}); err != nil {
			return xerrors.Errorf("mark proposal accepted: %w", err)
		}
		firstAccept = true
		return nil
	}, nil)
	switch {
	case errors.Is(err, errProposalHandled):
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{Message: proposalGoneMessage})
		return
	case database.IsUniqueViolation(err):
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "An MCP server with this slug already exists.",
			Detail:  err.Error(),
		})
		return
	case errors.Is(err, errInvalidProposalSecrets):
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid MCP server proposal credentials.",
			Detail:  err.Error(),
		})
		return
	case err != nil:
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to accept MCP server proposal.",
			Detail:  err.Error(),
		})
		return
	}

	authenticated := p.configAuthenticated(sysCtx, config, proposal.RequesterID)

	// Slack updates and chat notifications happen after commit, on the
	// accepting transition only. Repeated POSTs skip this entirely.
	if firstAccept {
		p.finalizeAcceptance(proposal, chat, config, authenticated)
	}

	resp := codersdk.AcceptMCPServerProposalResponse{
		MCPServerConfigID: config.ID,
		Authenticated:     authenticated,
	}
	if config.AuthType == "oauth2" && !authenticated {
		resp.ConnectURL = chattool.MCPOAuth2ConnectURL(p.accessURL, config.ID)
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// readProposalSecretValues accepts an empty body for compatibility with
// proposals that do not require user-supplied secrets.
func readProposalSecretValues(rw http.ResponseWriter, r *http.Request) (codersdk.AcceptMCPServerProposalRequest, bool) {
	var values codersdk.AcceptMCPServerProposalRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&values); err != nil {
		if errors.Is(err, io.EOF) {
			return values, true
		}
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return codersdk.AcceptMCPServerProposalRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Request body must contain a single JSON object.",
		})
		return codersdk.AcceptMCPServerProposalRequest{}, false
	}
	return values, true
}

func resolveProposalSecrets(
	req chattool.MCPServerProposalRequest,
	values codersdk.AcceptMCPServerProposalRequest,
) (chattool.MCPServerProposalRequest, error) {
	resolve := func(field, placeholder, value string) (string, error) {
		nonBlank := strings.TrimSpace(value) != ""
		switch {
		case placeholder != "" && !nonBlank:
			return "", xerrors.Errorf("%w: %s is required", errInvalidProposalSecrets, field)
		case placeholder == "" && nonBlank:
			return "", xerrors.Errorf("%w: %s was not requested", errInvalidProposalSecrets, field)
		default:
			return value, nil
		}
	}

	oauth2ClientSecret, err := resolve("oauth2_client_secret", req.OAuth2ClientSecretPlaceholder, values.OAuth2ClientSecret)
	if err != nil {
		return req, err
	}
	if req.OAuth2ClientSecretPlaceholder != "" {
		req.OAuth2ClientSecret = oauth2ClientSecret
	}
	apiKeyValue, err := resolve("api_key_value", req.APIKeyValuePlaceholder, values.APIKeyValue)
	if err != nil {
		return req, err
	}
	if req.APIKeyValuePlaceholder != "" {
		req.APIKeyValue = apiKeyValue
	}

	for header := range values.CustomHeaders {
		if _, ok := req.CustomHeaderPlaceholders[header]; !ok {
			return req, xerrors.Errorf("%w: custom header %q was not requested", errInvalidProposalSecrets, header)
		}
	}
	if len(req.CustomHeaderPlaceholders) > 0 && req.CustomHeaders == nil {
		req.CustomHeaders = make(map[string]string, len(req.CustomHeaderPlaceholders))
	}
	for header, placeholder := range req.CustomHeaderPlaceholders {
		value, ok := values.CustomHeaders[header]
		if !ok || strings.TrimSpace(value) == "" {
			return req, xerrors.Errorf("%w: custom header %q is required", errInvalidProposalSecrets, header)
		}
		if placeholder == "" {
			return req, xerrors.Errorf("%w: custom header %q has an empty placeholder", errInvalidProposalSecrets, header)
		}
		req.CustomHeaders[header] = value
	}
	return req, nil
}

// RejectProposal handles POST
// /api/v2/mcp-server-proposals/{mcpProposal}/reject. Only pending
// proposals can be rejected.
//
//nolint:revive // HTTP handler writes to ResponseWriter.
func (p *ProposalsAPI) RejectProposal(rw http.ResponseWriter, r *http.Request) {
	p.rejectProposal(rw, r, httpmw.APIKey(r).UserID)
}

//nolint:revive // HTTP handler writes to ResponseWriter.
func (p *ProposalsAPI) rejectProposal(rw http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	ctx := r.Context()
	proposal, chat, ok := p.loadAuthorizedProposal(rw, r, userID)
	if !ok {
		return
	}
	//nolint:gocritic // The requester check already authorized access.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	err := p.rejectPendingProposal(sysCtx, proposal.ID)
	switch {
	case errors.Is(err, errProposalHandled):
		httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{Message: proposalGoneMessage})
		return
	case err != nil:
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to reject MCP server proposal.",
			Detail:  err.Error(),
		})
		return
	}
	p.notifyRejected(proposal, chat)
	rw.WriteHeader(http.StatusNoContent)
}

// rejectPendingProposal flips a pending, unexpired proposal to
// rejected under the row lock. It returns errProposalHandled when the
// proposal is not pending anymore (or expired).
func (p *ProposalsAPI) rejectPendingProposal(ctx context.Context, proposalID uuid.UUID) error {
	return p.db.InTx(func(tx database.Store) error {
		locked, err := tx.GetMCPServerProposalByIDForUpdate(ctx, proposalID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errProposalHandled
			}
			return xerrors.Errorf("lock proposal: %w", err)
		}
		if locked.Status != proposalStatusPending || proposalExpired(locked) {
			return errProposalHandled
		}
		if _, err := tx.UpdateMCPServerProposalStatus(ctx, database.UpdateMCPServerProposalStatusParams{
			ID:                locked.ID,
			Status:            proposalStatusRejected,
			MCPServerConfigID: uuid.NullUUID{},
			AcceptedAt:        sql.NullTime{},
		}); err != nil {
			return xerrors.Errorf("mark proposal rejected: %w", err)
		}
		return nil
	}, nil)
}

// notifyRejected updates the card and posts the rejection system
// message. Shared by the reject endpoint and the Slack Cancel flow.
func (p *ProposalsAPI) notifyRejected(proposal database.MCPServerProposal, chat database.Chat) {
	name := proposalDisplayName(proposal)
	p.updateCard(p.bgCtx, proposal.Channel, proposal.MessageTs, proposalColorGray,
		fmt.Sprintf("~Connect %s?~ The proposal was rejected.", name))
	p.notifyChat(chat, fmt.Sprintf(
		"[system] The user rejected the MCP server proposal %q. The server was NOT created. Acknowledge this in the Slack thread and do not retry unless the user asks.", name))
}

// finalizeAcceptance runs the post-commit side effects of the
// accepting transition: enable the server for the chat, update the
// Slack card, notify the chat, and start the auth poll for
// unauthenticated oauth2 servers. It runs on the background context so
// an aborted HTTP request cannot skip the one-shot notifications.
func (p *ProposalsAPI) finalizeAcceptance(proposal database.MCPServerProposal, chat database.Chat, config database.MCPServerConfig, authenticated bool) {
	ctx := p.bgCtx
	name := proposalDisplayName(proposal)

	var enableErr error
	if p.chat != nil {
		_, enableErr = p.chat.AddChatMCPServerID(ctx, chat.ID, config.ID)
	} else {
		enableErr = xerrors.New("chat daemon is not running")
	}

	switch {
	case enableErr != nil:
		p.logger.Warn(ctx, "enable accepted mcp server for chat",
			slog.F("proposal_id", proposal.ID), slog.F("chat_id", chat.ID), slog.Error(enableErr))
		p.updateCard(ctx, proposal.Channel, proposal.MessageTs, proposalColorPartial,
			fmt.Sprintf("*%s* was created, but enabling it for this chat failed.", name))
		p.notifyChat(chat, fmt.Sprintf(
			"[system] The user accepted the MCP server proposal %q. The server was created, but enabling it for this chat failed: %v. Retry with enable_mcp_server.", name, enableErr))
	case authenticated:
		p.updateCard(ctx, proposal.Channel, proposal.MessageTs, proposalColorSuccess,
			fmt.Sprintf("*%s* is connected.", name))
		p.notifyChat(chat, fmt.Sprintf(
			"[system] The user accepted the MCP server proposal %q. The server was created and enabled for this chat; its tools are available from the next generation step. Continue helping the user.", name))
	default:
		// OAuth2 server awaiting authentication: keep the card pending
		// and poll for the token.
		p.updateCard(ctx, proposal.Channel, proposal.MessageTs, chattool.MCPProposalPendingColor,
			fmt.Sprintf("*%s* was created. Finishing authentication...", name))
		p.startAuthPoll(proposal, chat, config)
	}
}

// startAuthPoll watches the requester's token for the config and
// reports the outcome once: a green card plus system message on
// success, or a reminder system message on timeout.
func (p *ProposalsAPI) startAuthPoll(proposal database.MCPServerProposal, chat database.Chat, config database.MCPServerConfig) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ctx := p.bgCtx
		name := proposalDisplayName(proposal)
		deadline := time.Now().Add(p.authPollTimeout)
		ticker := time.NewTicker(p.authPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			//nolint:gocritic // Token existence checks need system access; the poll is scoped to the accepted config and the requester.
			if p.configAuthenticated(dbauthz.AsSystemRestricted(ctx), config, proposal.RequesterID) {
				p.updateCard(ctx, proposal.Channel, proposal.MessageTs, proposalColorSuccess,
					fmt.Sprintf("*%s* is connected.", name))
				p.notifyChat(chat, fmt.Sprintf(
					"[system] The user accepted the MCP server proposal %q and finished authenticating. The server is enabled for this chat; its tools are available from the next generation step. Continue helping the user.", name))
				return
			}
			if time.Now().After(deadline) {
				p.notifyChat(chat, fmt.Sprintf(
					"[system] The user accepted the MCP server proposal %q, but has not finished authenticating with it yet. Remind them in the Slack thread that they can still connect at %s.",
					name, chattool.MCPOAuth2ConnectURL(p.accessURL, config.ID)))
				return
			}
		}
	}()
}

// HandleBlockActions processes Slack block-actions interactions.
// Cancel-button clicks reject the proposal; Review URL-button
// interactions are ignored (they were already acked).
func (p *ProposalsAPI) HandleBlockActions(ctx context.Context, callback slack.InteractionCallback) {
	for _, action := range callback.ActionCallback.BlockActions {
		if action == nil || action.ActionID != chattool.MCPProposalCancelActionID {
			continue
		}
		p.handleCancelClick(ctx, callback, action.Value)
	}
}

// handleCancelClick implements the in-Slack Cancel button, sharing
// semantics with the reject endpoint. The clicking Slack user is
// resolved to a Coder user and must match the proposal's requester.
func (p *ProposalsAPI) handleCancelClick(ctx context.Context, callback slack.InteractionCallback, value string) {
	channel := callback.Channel.ID
	messageTS := callback.Message.Timestamp

	proposalID, err := uuid.Parse(value)
	if err != nil {
		p.logger.Warn(ctx, "mcp proposal cancel click with invalid value", slog.F("value", value))
		return
	}
	proposal, err := p.db.GetMCPServerProposalByID(ctx, proposalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.updateCard(ctx, channel, messageTS, proposalColorGray,
				"This MCP server proposal has expired or was already handled.")
			return
		}
		p.logger.Warn(ctx, "load mcp proposal for cancel", slog.Error(err))
		return
	}
	switch {
	case proposal.Status == proposalStatusAccepted:
		p.postEphemeral(ctx, channel, callback.User.ID,
			"This proposal was already accepted and can no longer be canceled.")
		return
	case proposal.Status == proposalStatusRejected || proposalExpired(proposal):
		p.updateCard(ctx, channel, messageTS, proposalColorGray,
			"This MCP server proposal has expired or was already handled.")
		return
	case !p.clickerIsRequester(ctx, callback.User.ID, proposal):
		p.postEphemeral(ctx, channel, callback.User.ID,
			"Only the user who requested this proposal can respond to this dialog.")
		return
	}

	chat, err := p.db.GetChatByID(ctx, proposal.ChatID)
	if err != nil {
		p.logger.Warn(ctx, "load chat for mcp proposal cancel", slog.Error(err))
		return
	}
	err = p.rejectPendingProposal(ctx, proposal.ID)
	switch {
	case errors.Is(err, errProposalHandled):
		// Lost a race with an accept or another reject; the winning
		// transition already updated the card.
		return
	case err != nil:
		p.logger.Warn(ctx, "cancel mcp proposal", slog.F("proposal_id", proposal.ID), slog.Error(err))
		return
	}
	p.notifyRejected(proposal, chat)
}

// clickerIsRequester resolves the clicking Slack user to a Coder user
// and reports whether it matches the proposal's requester. Resolution
// failures (no resolver configured, ambiguous links, unusable users)
// fail closed.
func (p *ProposalsAPI) clickerIsRequester(ctx context.Context, slackUserID string, proposal database.MCPServerProposal) bool {
	if p.resolveSlackUser == nil {
		p.logger.Warn(ctx, "mcp proposal cancel click without a slack user resolver, denying",
			slog.F("proposal_id", proposal.ID))
		return false
	}
	coderUserID, err := p.resolveSlackUser(ctx, slackUserID)
	if err != nil {
		p.logger.Warn(ctx, "resolve slack user for mcp proposal cancel, denying",
			slog.F("proposal_id", proposal.ID),
			slog.F("slack_user_id", slackUserID),
			slog.Error(err))
		return false
	}
	return coderUserID == proposal.RequesterID
}

// createProposedMCPServerConfig creates the personal MCP server config
// for an accepted proposal on the transaction handle. For oauth2
// proposals without explicit client credentials it performs automatic
// discovery and Dynamic Client Registration; a discovery failure rolls
// the transaction back.
//
// The validation and insert behavior duplicates the personal branch of
// coderd's createMCPServerConfig on purpose; keep the two in sync.
// Like that path, personal servers currently use an unprotected HTTP
// client for OAuth discovery and MCP connections (SSRF); see the TODOs
// in coderd/mcp.go.
func (p *ProposalsAPI) createProposedMCPServerConfig(
	ctx context.Context,
	tx database.Store,
	ownerID uuid.UUID,
	req chattool.MCPServerProposalRequest,
) (database.MCPServerConfig, error) {
	customHeadersJSON := "{}"
	if len(req.CustomHeaders) > 0 {
		encoded, err := json.Marshal(req.CustomHeaders)
		if err != nil {
			return database.MCPServerConfig{}, xerrors.Errorf("encode custom headers: %w", err)
		}
		customHeadersJSON = string(encoded)
	}

	params := database.InsertMCPServerConfigParams{
		DisplayName:             req.DisplayName,
		Slug:                    req.Slug,
		Description:             req.Description,
		IconURL:                 req.IconURL,
		Transport:               req.Transport,
		Url:                     req.URL,
		AuthType:                req.AuthType,
		OAuth2ClientID:          "",
		OAuth2ClientSecret:      "",
		OAuth2ClientSecretKeyID: sql.NullString{},
		OAuth2AuthURL:           "",
		OAuth2TokenURL:          "",
		OAuth2Scopes:            "",
		APIKeyHeader:            req.APIKeyHeader,
		APIKeyValue:             req.APIKeyValue,
		APIKeyValueKeyID:        sql.NullString{},
		CustomHeaders:           customHeadersJSON,
		CustomHeadersKeyID:      sql.NullString{},
		ToolAllowList:           coalesceStringSlice(req.ToolAllowList),
		ToolDenyList:            coalesceStringSlice(req.ToolDenyList),
		Availability:            "default_off",
		Enabled:                 !req.Disabled,
		ModelIntent:             false,
		AllowInPlanMode:         false,
		ForwardCoderHeaders:     false,
		CreatedBy:               ownerID,
		UpdatedBy:               ownerID,
		OwnerID:                 uuid.NullUUID{UUID: ownerID, Valid: true},
	}
	if params.APIKeyHeader == "" {
		params.APIKeyHeader = "Authorization"
	}

	discover := req.AuthType == "oauth2" &&
		req.OAuth2ClientID == "" && req.OAuth2AuthURL == "" && req.OAuth2TokenURL == ""
	if !discover {
		params.OAuth2ClientID = req.OAuth2ClientID
		params.OAuth2ClientSecret = req.OAuth2ClientSecret
		params.OAuth2AuthURL = req.OAuth2AuthURL
		params.OAuth2TokenURL = req.OAuth2TokenURL
		params.OAuth2Scopes = req.OAuth2Scopes
		return tx.InsertMCPServerConfig(ctx, params)
	}

	// Auto-discovery flow: the config id is needed for the callback
	// URL, so insert first with empty OAuth2 fields, discover, then
	// update. All DB work stays on the transaction handle so a
	// discovery failure rolls the insert back.
	inserted, err := tx.InsertMCPServerConfig(ctx, params)
	if err != nil {
		return database.MCPServerConfig{}, err
	}
	callbackURL := fmt.Sprintf("%s/api/experimental/mcp/servers/%s/oauth2/callback",
		p.accessURLString(), inserted.ID)
	result, err := discoverAndRegisterMCPOAuth2(ctx, p.httpClient, req.URL, callbackURL)
	if err != nil {
		return database.MCPServerConfig{}, xerrors.Errorf("oauth2 auto-discovery: %w", err)
	}
	oauth2Scopes := req.OAuth2Scopes
	if oauth2Scopes == "" {
		oauth2Scopes = result.scopes
	}
	return tx.UpdateMCPServerConfig(ctx, database.UpdateMCPServerConfigParams{
		ID:                      inserted.ID,
		DisplayName:             inserted.DisplayName,
		Slug:                    inserted.Slug,
		Description:             inserted.Description,
		IconURL:                 inserted.IconURL,
		Transport:               inserted.Transport,
		Url:                     inserted.Url,
		AuthType:                inserted.AuthType,
		OAuth2ClientID:          result.clientID,
		OAuth2ClientSecret:      result.clientSecret,
		OAuth2ClientSecretKeyID: sql.NullString{},
		OAuth2AuthURL:           result.authURL,
		OAuth2TokenURL:          result.tokenURL,
		OAuth2Scopes:            oauth2Scopes,
		APIKeyHeader:            inserted.APIKeyHeader,
		APIKeyValue:             inserted.APIKeyValue,
		APIKeyValueKeyID:        inserted.APIKeyValueKeyID,
		CustomHeaders:           inserted.CustomHeaders,
		CustomHeadersKeyID:      inserted.CustomHeadersKeyID,
		ToolAllowList:           inserted.ToolAllowList,
		ToolDenyList:            inserted.ToolDenyList,
		Availability:            inserted.Availability,
		Enabled:                 inserted.Enabled,
		ModelIntent:             inserted.ModelIntent,
		AllowInPlanMode:         inserted.AllowInPlanMode,
		ForwardCoderHeaders:     inserted.ForwardCoderHeaders,
		UpdatedBy:               ownerID,
	})
}

func (p *ProposalsAPI) accessURLString() string {
	if p.accessURL == nil {
		return ""
	}
	return p.accessURL.String()
}

// configAuthenticated reports whether the owner's auth for the config
// is usable, matching the chat tools' view.
func (p *ProposalsAPI) configAuthenticated(ctx context.Context, config database.MCPServerConfig, ownerID uuid.UUID) bool {
	if config.AuthType != "oauth2" {
		return true
	}
	token, err := p.db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
		MCPServerConfigID: config.ID,
		UserID:            ownerID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			p.logger.Warn(ctx, "load mcp server user token", slog.Error(err))
		}
		return false
	}
	return chattool.MCPAuthConnected(config, &token, time.Now())
}

// updateCard replaces the proposal card in place with a single section
// attachment in the given color, clearing the top-level text and
// blocks. Skipped (with a log) when Slack is not configured or the
// proposal has no recorded card message.
func (p *ProposalsAPI) updateCard(ctx context.Context, channel, messageTS, color, text string) {
	if p.webAPI == nil || channel == "" || messageTS == "" {
		return
	}
	attachment := slack.Attachment{
		Color: color,
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, text, false, false), nil, nil),
		}},
	}
	if _, _, _, err := p.webAPI.UpdateMessageContext(ctx, channel, messageTS,
		slack.MsgOptionText("", false),
		slack.MsgOptionBlocks(),
		slack.MsgOptionAttachments(attachment),
	); err != nil {
		p.logger.Warn(ctx, "update mcp proposal card",
			slog.F("channel", channel), slog.F("message_ts", messageTS), slog.Error(err))
	}
}

// postEphemeral posts an ephemeral in-thread message to a Slack user.
func (p *ProposalsAPI) postEphemeral(ctx context.Context, channel, userID, text string) {
	if p.webAPI == nil {
		return
	}
	if _, err := p.webAPI.PostEphemeralContext(ctx, channel, userID,
		slack.MsgOptionText(text, false),
	); err != nil {
		p.logger.Warn(ctx, "post ephemeral mcp proposal message", slog.Error(err))
	}
}

// notifyChat posts a system message to the proposing chat, interrupting
// an active generation so the model observes the outcome promptly.
func (p *ProposalsAPI) notifyChat(chat database.Chat, text string) {
	if p.chat == nil {
		return
	}
	ctx := p.bgCtx
	apiKeyID, err := ensureAPIKeyID(ctx, p.db, chat.OwnerID)
	if err != nil {
		p.logger.Warn(ctx, "ensure api key for mcp proposal notification",
			slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	if _, err := p.chat.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: chat.OwnerID,
		APIKeyID:  apiKeyID,
		Content: []codersdk.ChatMessagePart{{
			Type: codersdk.ChatMessagePartTypeText,
			Text: text,
		}},
		BusyBehavior: chatd.SendMessageBusyBehaviorInterrupt,
	}); err != nil {
		p.logger.Warn(ctx, "notify chat of mcp proposal outcome",
			slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

// parseProposalRequest decodes the persisted proposal request.
func parseProposalRequest(proposal database.MCPServerProposal) (chattool.MCPServerProposalRequest, error) {
	var req chattool.MCPServerProposalRequest
	if err := json.Unmarshal(proposal.Request, &req); err != nil {
		return chattool.MCPServerProposalRequest{}, err
	}
	return req, nil
}

// proposalDisplayName returns the proposed display name, falling back
// to the proposal id when the request cannot be decoded.
func proposalDisplayName(proposal database.MCPServerProposal) string {
	req, err := parseProposalRequest(proposal)
	if err != nil || req.DisplayName == "" {
		return proposal.ID.String()
	}
	return req.DisplayName
}

// convertMCPServerProposal converts the proposal row to the SDK view.
// Secrets are never returned; only which auth material was provided.
func convertMCPServerProposal(proposal database.MCPServerProposal, req chattool.MCPServerProposalRequest) codersdk.MCPServerProposal {
	return codersdk.MCPServerProposal{
		ID:        proposal.ID,
		ChatID:    proposal.ChatID,
		Status:    codersdk.MCPServerProposalStatus(proposal.Status),
		CreatedAt: proposal.CreatedAt,

		DisplayName:  req.DisplayName,
		Slug:         req.Slug,
		Description:  req.Description,
		Instructions: req.Instructions,
		IconURL:      req.IconURL,
		URL:          req.URL,
		Transport:    req.Transport,
		AuthType:     req.AuthType,

		ToolAllowList: req.ToolAllowList,
		ToolDenyList:  req.ToolDenyList,

		HasOAuth2ClientCredentials: req.OAuth2ClientID != "",
		HasAPIKey:                  req.APIKeyValue != "",
		HasCustomHeaders:           len(req.CustomHeaders) > 0,
		SecretPlaceholders: codersdk.MCPServerProposalSecretPlaceholders{
			OAuth2ClientSecret: req.OAuth2ClientSecretPlaceholder,
			APIKeyValue:        req.APIKeyValuePlaceholder,
			CustomHeaders:      req.CustomHeaderPlaceholders,
		},

		CreateDisabled: req.Disabled,
	}
}

// coalesceStringSlice returns ss if non-nil, otherwise an empty
// non-nil slice, preventing pq.Array from sending NULL for NOT NULL
// text[] columns. Duplicated from coderd/mcp.go on purpose.
func coalesceStringSlice(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}

// handleBlockActions routes block-actions interactions to the
// proposals handler when one is configured.
func (s *Server) handleBlockActions(ctx context.Context, callback slack.InteractionCallback) {
	if s.proposals == nil {
		return
	}
	s.proposals.HandleBlockActions(ctx, callback)
}
