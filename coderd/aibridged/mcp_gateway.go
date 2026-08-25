package aibridged

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/mcptools"
	"github.com/coder/coder/v2/codersdk"
)

const (
	mcpGatewayRoutePrefix      = "/mcp/"
	mcpGatewayRecorderProvider = "mcp-gateway"
)

type mcpGatewayEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpGatewayRequest struct {
	Batch bool
	Items []mcpGatewayRequestItem
}

type mcpGatewayRequestItem struct {
	Raw      json.RawMessage
	Envelope mcpGatewayEnvelope
}

const mcpGatewayCredentialTTL = time.Minute

type mcpGatewayCredentialCacheKey struct {
	InitiatorID string
	ServerID    string
}

type mcpGatewayCredentialCacheEntry struct {
	AccessToken string
	ProviderID  string
	ExpiresAt   time.Time
}

type mcpGatewayCredentialFailure struct {
	Message string
	Data    map[string]any
}

type mcpGatewayPolicy struct {
	tools     mcptools.Policy
	allowlist *regexp.Regexp
	denylist  *regexp.Regexp
}

type mcpGatewayPlan struct {
	forward        []json.RawMessage
	local          []json.RawMessage
	toolCalls      []mcpGatewayToolCall
	escalatedCall  *mcpGatewayToolCall
	toolsListIDs   map[string]struct{}
	filterResponse bool
	batch          bool
	policy         mcpGatewayPolicy
}

type mcpGatewayToolCall struct {
	Tool            string
	Input           string
	JSONRPCID       string
	JSONRPCIDRaw    json.RawMessage
	InvocationError string
	Disposition     string
	EscalationID    string
}

type mcpGatewayError struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data,omitempty"`
	} `json:"error"`
}

// writeMCPGatewayNotFound writes a JSON 404 that names the problem. The
// bare http.NotFound plain-text body is indistinguishable from a router
// miss, which makes misconfigured slugs needlessly hard to debug.
func writeMCPGatewayNotFound(rw http.ResponseWriter, message string) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(rw).Encode(map[string]string{"message": message})
}

func mcpGatewayServerSlug(path string) (string, bool) {
	if path == "/mcp" || path == "/mcp/" {
		return "", true
	}
	if !strings.HasPrefix(path, mcpGatewayRoutePrefix) {
		return "", false
	}
	slug := strings.TrimPrefix(path, mcpGatewayRoutePrefix)
	if slug == "" || strings.Contains(slug, "/") {
		return "", true
	}
	return slug, true
}

func parseMCPGatewayRequest(body []byte) (mcpGatewayRequest, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return mcpGatewayRequest{}, xerrors.New("empty JSON-RPC request")
	}

	var raws []json.RawMessage
	batch := trimmed[0] == '['
	if batch {
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return mcpGatewayRequest{}, xerrors.Errorf("decode JSON-RPC batch: %w", err)
		}
		if len(raws) == 0 {
			return mcpGatewayRequest{}, xerrors.New("empty JSON-RPC batch")
		}
	} else {
		raws = []json.RawMessage{append(json.RawMessage(nil), trimmed...)}
	}

	request := mcpGatewayRequest{
		Batch: batch,
		Items: make([]mcpGatewayRequestItem, 0, len(raws)),
	}
	for _, raw := range raws {
		var envelope mcpGatewayEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return mcpGatewayRequest{}, xerrors.Errorf("decode JSON-RPC request: %w", err)
		}
		request.Items = append(request.Items, mcpGatewayRequestItem{
			Raw:      append(json.RawMessage(nil), raw...),
			Envelope: envelope,
		})
	}
	return request, nil
}

func newMCPGatewayPolicy(cfg *proto.MCPGatewayServerConfig) (mcpGatewayPolicy, error) {
	rules := make([]codersdk.MCPServerToolRule, 0, len(cfg.GetToolRules()))
	for _, rule := range cfg.GetToolRules() {
		rules = append(rules, codersdk.MCPServerToolRule{
			Tool:    rule.GetTool(),
			Action:  codersdk.MCPServerToolAction(rule.GetAction()),
			Enabled: rule.GetEnabled(),
		})
	}
	policy := mcpGatewayPolicy{
		tools: mcptools.Policy{
			AllowList: cfg.GetToolAllowList(),
			DenyList:  cfg.GetToolDenyList(),
			Rules:     rules,
			Default:   cfg.GetToolDefault(),
		},
	}
	var err error
	if cfg.GetToolAllowRegex() != "" {
		policy.allowlist, err = regexp.Compile(cfg.GetToolAllowRegex())
		if err != nil {
			return mcpGatewayPolicy{}, xerrors.Errorf("compile MCP tool allow regex: %w", err)
		}
	}
	if cfg.GetToolDenyRegex() != "" {
		policy.denylist, err = regexp.Compile(cfg.GetToolDenyRegex())
		if err != nil {
			return mcpGatewayPolicy{}, xerrors.Errorf("compile MCP tool deny regex: %w", err)
		}
	}
	return policy, nil
}

func (p mcpGatewayPolicy) evaluate(tool string) mcptools.Action {
	// The regex lists are binary and apply on top of the rule layers, so a
	// regex-excluded tool cannot be escalated into existence.
	if p.denylist != nil && p.denylist.MatchString(tool) {
		return mcptools.ActionBlock
	}
	if p.allowlist != nil && !p.allowlist.MatchString(tool) {
		return mcptools.ActionBlock
	}
	return mcptools.Evaluate(p.tools, tool)
}

func planMCPGatewayRequest(request mcpGatewayRequest, policy mcpGatewayPolicy) (mcpGatewayPlan, error) {
	plan := mcpGatewayPlan{
		forward:      make([]json.RawMessage, 0, len(request.Items)),
		toolsListIDs: make(map[string]struct{}),
		batch:        request.Batch,
		policy:       policy,
	}

	for _, item := range request.Items {
		switch mcp.MCPMethod(item.Envelope.Method) {
		case mcp.MethodToolsList:
			plan.forward = append(plan.forward, item.Raw)
			if key, ok := mcpGatewayIDKey(item.Envelope.ID); ok {
				plan.toolsListIDs[key] = struct{}{}
				plan.filterResponse = true
			}
		case mcp.MethodToolsCall:
			call := mcpGatewayToolCall{
				JSONRPCID:    mcpGatewayIDValue(item.Envelope.ID),
				JSONRPCIDRaw: append(json.RawMessage(nil), item.Envelope.ID...),
			}
			var params mcp.CallToolParams
			if err := json.Unmarshal(item.Envelope.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
				call.Tool = params.Name
				call.Input = "null"
				call.InvocationError = "invalid tools/call parameters"
				call.Disposition = "blocked"
				plan.toolCalls = append(plan.toolCalls, call)
				if len(item.Envelope.ID) > 0 {
					plan.local = append(plan.local, marshalMCPGatewayError(
						item.Envelope.ID,
						mcp.INVALID_PARAMS,
						call.InvocationError,
						nil,
					))
				}
				continue
			}
			input, err := json.Marshal(params.Arguments)
			if err != nil {
				return mcpGatewayPlan{}, xerrors.Errorf("encode MCP tool arguments: %w", err)
			}
			call.Tool = params.Name
			call.Input = string(input)
			switch policy.evaluate(params.Name) {
			case mcptools.ActionBlock:
				call.InvocationError = fmt.Sprintf("tool %q denied by MCP gateway policy", params.Name)
				call.Disposition = "blocked"
				plan.toolCalls = append(plan.toolCalls, call)
				if len(item.Envelope.ID) > 0 {
					plan.local = append(plan.local, marshalMCPGatewayError(
						item.Envelope.ID,
						mcp.INTERNAL_ERROR,
						call.InvocationError,
						map[string]any{"tool": params.Name},
					))
				}
				continue
			case mcptools.ActionEscalate:
				if !request.Batch {
					plan.escalatedCall = &call
					continue
				}
				call.InvocationError = fmt.Sprintf("tool %q cannot be called because escalated tools cannot be called in batches", params.Name)
				call.Disposition = "blocked"
				plan.toolCalls = append(plan.toolCalls, call)
				if len(item.Envelope.ID) > 0 {
					plan.local = append(plan.local, marshalMCPGatewayError(
						item.Envelope.ID,
						mcp.INTERNAL_ERROR,
						call.InvocationError,
						map[string]any{"tool": params.Name, "disposition": "escalate"},
					))
				}
				continue
			case mcptools.ActionPermit:
				call.Disposition = "permitted"
			}
			plan.toolCalls = append(plan.toolCalls, call)
			plan.forward = append(plan.forward, item.Raw)
		default:
			plan.forward = append(plan.forward, item.Raw)
		}
	}
	if len(plan.local) > 0 {
		plan.filterResponse = true
	}
	return plan, nil
}

func marshalMCPGatewayError(id json.RawMessage, code int, message string, data map[string]any) json.RawMessage {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	response := mcpGatewayError{
		JSONRPC: "2.0",
		ID:      append(json.RawMessage(nil), id...),
	}
	response.Error.Code = code
	response.Error.Message = message
	response.Error.Data = data
	encoded, _ := json.Marshal(response)
	return encoded
}

func mcpGatewayIDKey(id json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(id)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", false
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return "", false
	}
	return compact.String(), true
}

func mcpGatewayIDValue(id json.RawMessage) string {
	trimmed := bytes.TrimSpace(id)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var stringID string
	if err := json.Unmarshal(trimmed, &stringID); err == nil {
		return stringID
	}
	key, _ := mcpGatewayIDKey(trimmed)
	return key
}

// MCP gateway recording is best-effort observability. Recording failures must
// never change the proxied request or response.
func (s *Server) startMCPGatewayRecording(
	ctx context.Context,
	client DRPCClient,
	authz *proto.AuthorizeMCPGatewayResponse,
	cfg *proto.MCPGatewayServerConfig,
	userAgent string,
) string {
	interceptionID := uuid.NewString()
	startedAt := time.Now()
	_, err := client.RecordInterception(ctx, &proto.RecordInterceptionRequest{
		Id:             interceptionID,
		InitiatorId:    authz.GetInitiatorId(),
		SponsorUserId:  authz.GetSponsorUserId(),
		Provider:       mcpGatewayRecorderProvider,
		ProviderName:   mcpGatewayRecorderProvider,
		Model:          cfg.GetSlug(),
		StartedAt:      timestamppb.New(startedAt),
		ApiKeyId:       authz.GetApiKeyId(),
		Client:         mcpGatewayRecorderProvider,
		UserAgent:      userAgent,
		CredentialKind: "centralized",
	})
	if err != nil {
		s.logger.Warn(ctx, "failed to record MCP gateway interception", slog.F("server_slug", cfg.GetSlug()), slog.Error(err))
		return ""
	}

	return interceptionID
}

func (s *Server) recordMCPGatewayToolUsages(ctx context.Context, client DRPCClient, interceptionID string, cfg *proto.MCPGatewayServerConfig, toolCalls []mcpGatewayToolCall) {
	if interceptionID == "" {
		return
	}
	serverURL := cfg.GetUrl()
	for _, call := range toolCalls {
		var invocationError *string
		if call.InvocationError != "" {
			invocationError = &call.InvocationError
		}
		_, err := client.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
			InterceptionId:  interceptionID,
			MsgId:           call.JSONRPCID,
			ServerUrl:       &serverURL,
			Tool:            call.Tool,
			Input:           call.Input,
			InvocationError: invocationError,
			CreatedAt:       timestamppb.Now(),
			ToolCallId:      call.JSONRPCID,
			ItemId:          call.JSONRPCID,
			Disposition:     call.Disposition,
			EscalationId:    call.EscalationID,
		})
		if err != nil {
			s.logger.Warn(ctx, "failed to record MCP gateway tool usage", slog.F("server_slug", cfg.GetSlug()), slog.F("tool", call.Tool), slog.Error(err))
		}
	}
}

func (s *Server) endMCPGatewayRecording(ctx context.Context, client DRPCClient, interceptionID, slug string) {
	if ctx.Err() != nil {
		return
	}
	_, err := client.RecordInterceptionEnded(ctx, &proto.RecordInterceptionEndedRequest{
		Id:      interceptionID,
		EndedAt: timestamppb.Now(),
	})
	if err != nil {
		s.logger.Warn(ctx, "failed to end MCP gateway interception", slog.F("server_slug", slug), slog.Error(err))
	}
}

func (s *Server) serveMCPGateway(rw http.ResponseWriter, r *http.Request, client DRPCClient, token, slug string) {
	ctx := r.Context()
	if slug == "" {
		writeMCPGatewayNotFound(rw, "the request path must name an MCP server slug: /mcp/{server-slug}")
		return
	}

	configResponse, err := client.GetMCPGatewayServerConfig(ctx, &proto.GetMCPGatewayServerConfigRequest{Slug: slug})
	if err != nil {
		s.logger.Warn(ctx, "failed to retrieve MCP gateway server config", slog.F("server_slug", slug), slog.Error(err))
		http.Error(rw, "failed to retrieve MCP server configuration", http.StatusInternalServerError)
		return
	}
	if !configResponse.GetFound() || configResponse.GetConfig() == nil {
		writeMCPGatewayNotFound(rw, fmt.Sprintf(
			"no enabled MCP server with slug %q; check the slug and enabled state under admin AI settings, MCP Servers", slug))
		return
	}
	cfg := configResponse.GetConfig()

	authz, err := client.AuthorizeMCPGateway(ctx, &proto.AuthorizeMCPGatewayRequest{
		Key:               token,
		McpServerConfigId: cfg.GetId(),
	})
	if err != nil {
		s.logger.Warn(ctx, "mcp gateway authorization failed", slog.F("server_slug", slug), slog.Error(err))
		http.Error(rw, "failed to authorize MCP gateway request", http.StatusInternalServerError)
		return
	}
	if !authz.GetAuthorized() {
		// Return the same response as an unknown slug so callers cannot use this
		// endpoint to enumerate MCP servers they are not permitted to access.
		http.NotFound(rw, r)
		return
	}

	if cfg.GetTransport() == "sse" {
		http.Error(rw, "SSE upstream MCP transport is not supported by the AI gateway", http.StatusNotImplemented)
		return
	}
	if cfg.GetTransport() != "" && cfg.GetTransport() != "streamable_http" {
		http.Error(rw, "unsupported upstream MCP transport", http.StatusNotImplemented)
		return
	}

	policy, err := newMCPGatewayPolicy(cfg)
	if err != nil {
		s.logger.Error(ctx, "invalid MCP gateway tool policy", slog.F("server_slug", slug), slog.Error(err))
		http.Error(rw, "invalid MCP server tool policy", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodDelete:
		s.forwardMCPGatewayResponse(rw, r, client, token, authz.GetInitiatorId(), cfg, nil, nil, nil)
		return
	case http.MethodPost:
	default:
		rw.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(rw, "failed to read MCP request", http.StatusBadRequest)
		return
	}
	request, err := parseMCPGatewayRequest(body)
	if err != nil {
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.PARSE_ERROR, err.Error(), nil))
		return
	}
	plan, err := planMCPGatewayRequest(request, policy)
	if err != nil {
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, err.Error(), nil))
		return
	}
	interceptionID := ""
	if len(plan.toolCalls) > 0 || plan.escalatedCall != nil {
		interceptionID = s.startMCPGatewayRecording(ctx, client, authz, cfg, r.UserAgent())
		if interceptionID != "" {
			defer s.endMCPGatewayRecording(ctx, client, interceptionID, cfg.GetSlug())
		}
	}
	if len(plan.toolCalls) > 0 {
		s.recordMCPGatewayToolUsages(ctx, client, interceptionID, cfg, plan.toolCalls)
	}
	if plan.escalatedCall != nil {
		s.holdMCPGatewayEscalation(rw, r, client, token, authz.GetInitiatorId(), cfg, body, interceptionID, *plan.escalatedCall)
		return
	}
	if len(plan.forward) == 0 {
		writeMCPGatewayLocalResponses(rw, plan)
		return
	}

	forwardBody := body
	if request.Batch && len(plan.forward) != len(request.Items) {
		forwardBody, err = json.Marshal(plan.forward)
		if err != nil {
			writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, "failed to encode filtered JSON-RPC batch", nil))
			return
		}
	}
	s.forwardMCPGatewayResponse(rw, r, client, token, authz.GetInitiatorId(), cfg, forwardBody, &plan, mcpGatewayResponseID(request))
}

type mcpGatewayEscalationWaitResult struct {
	status string
	err    error
}

func (s *Server) holdMCPGatewayEscalation(
	rw http.ResponseWriter,
	r *http.Request,
	client DRPCClient,
	token string,
	initiatorID string,
	cfg *proto.MCPGatewayServerConfig,
	body []byte,
	interceptionID string,
	call mcpGatewayToolCall,
) {
	ctx := r.Context()
	created, err := client.CreateMCPGatewayEscalation(ctx, &proto.CreateMCPGatewayEscalationRequest{
		Key:               token,
		McpServerConfigId: cfg.GetId(),
		ServerSlug:        cfg.GetSlug(),
		ServerUrl:         cfg.GetUrl(),
		Tool:              call.Tool,
		Input:             call.Input,
		WorkspaceName:     "",
	})
	if err != nil || created.GetId() == "" {
		call.Disposition = "blocked"
		call.InvocationError = "failed to create MCP gateway escalation"
		s.recordMCPGatewayToolUsages(ctx, client, interceptionID, cfg, []mcpGatewayToolCall{call})
		s.logger.Warn(ctx, call.InvocationError, slog.F("server_slug", cfg.GetSlug()), slog.Error(err))
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(call.JSONRPCIDRaw, mcp.INTERNAL_ERROR, call.InvocationError, nil))
		return
	}

	call.EscalationID = created.GetId()
	expiresAt := time.Now().Add(5 * time.Minute)
	if created.GetExpiresAt() != nil && created.GetExpiresAt().IsValid() {
		expiresAt = created.GetExpiresAt().AsTime()
	}

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(rw)
	_ = controller.Flush()
	_, _ = fmt.Fprintf(rw, ": escalation pending %s\n\n", call.EscalationID)
	_ = controller.Flush()

	status, err := s.waitMCPGatewayEscalation(ctx, rw, controller, client, call.EscalationID, expiresAt)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn(ctx, "failed while waiting for MCP gateway escalation", slog.F("escalation_id", call.EscalationID), slog.Error(err))
		}
		return
	}

	switch status {
	case "approved":
		call.Disposition = "escalated_approved"
		s.forwardApprovedMCPGatewayEscalation(rw, r, client, token, initiatorID, cfg, body, interceptionID, call)
	case "denied", "expired":
		call.Disposition = "escalated_" + status
		call.InvocationError = fmt.Sprintf("MCP gateway escalation was %s", status)
		s.recordMCPGatewayToolUsages(ctx, client, interceptionID, cfg, []mcpGatewayToolCall{call})
		writeMCPGatewaySSEMessage(rw, controller, marshalMCPGatewayError(
			call.JSONRPCIDRaw,
			mcp.INTERNAL_ERROR,
			call.InvocationError,
			map[string]any{"escalation_id": call.EscalationID, "status": status},
		))
	default:
		call.Disposition = "escalated_denied"
		call.InvocationError = fmt.Sprintf("MCP gateway escalation returned unexpected status %q", status)
		s.recordMCPGatewayToolUsages(ctx, client, interceptionID, cfg, []mcpGatewayToolCall{call})
		writeMCPGatewaySSEMessage(rw, controller, marshalMCPGatewayError(
			call.JSONRPCIDRaw,
			mcp.INTERNAL_ERROR,
			call.InvocationError,
			map[string]any{"escalation_id": call.EscalationID, "status": status},
		))
	}
}

func (s *Server) waitMCPGatewayEscalation(
	ctx context.Context,
	rw http.ResponseWriter,
	controller *http.ResponseController,
	client DRPCClient,
	escalationID string,
	expiresAt time.Time,
) (string, error) {
	holdCtx, cancel := context.WithDeadline(ctx, expiresAt.Add(s.mcpEscalationHoldGrace))
	defer cancel()
	keepalive := time.NewTicker(s.mcpEscalationKeepaliveInterval)
	defer keepalive.Stop()

	writeKeepalive := func() {
		_, _ = io.WriteString(rw, ": keepalive\n\n")
		_ = controller.Flush()
	}
	waitForPoll := func() error {
		timer := time.NewTimer(s.mcpEscalationPollInterval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-holdCtx.Done():
				return holdCtx.Err()
			case <-keepalive.C:
				writeKeepalive()
			case <-timer.C:
				return nil
			}
		}
	}

	for {
		result := make(chan mcpGatewayEscalationWaitResult, 1)
		go func() {
			response, err := client.WaitMCPGatewayEscalation(holdCtx, &proto.WaitMCPGatewayEscalationRequest{Id: escalationID})
			result <- mcpGatewayEscalationWaitResult{status: response.GetStatus(), err: err}
		}()

	waitResponse:
		for {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-holdCtx.Done():
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				return "expired", nil
			case <-keepalive.C:
				writeKeepalive()
			case waited := <-result:
				if waited.err != nil {
					if holdCtx.Err() != nil && ctx.Err() == nil {
						return "expired", nil
					}
					return "", waited.err
				}
				if waited.status != "" && waited.status != "pending" {
					return waited.status, nil
				}
				if err := waitForPoll(); err != nil {
					if holdCtx.Err() != nil && ctx.Err() == nil {
						return "expired", nil
					}
					return "", err
				}
				break waitResponse
			}
		}
	}
}

func (s *Server) forwardApprovedMCPGatewayEscalation(
	rw http.ResponseWriter,
	r *http.Request,
	client DRPCClient,
	token string,
	initiatorID string,
	cfg *proto.MCPGatewayServerConfig,
	body []byte,
	interceptionID string,
	call mcpGatewayToolCall,
) {
	ctx := r.Context()
	controller := http.NewResponseController(rw)
	authHeaders, externalAuth, failure, err := s.resolveMCPGatewayUpstreamAuth(ctx, client, token, initiatorID, cfg)
	if err == nil && failure == nil {
		var response *http.Response
		response, err = doMCPGatewayUpstreamRequest(ctx, r, cfg.GetUrl(), body, "application/json, text/event-stream", authHeaders)
		if err == nil && response.StatusCode == http.StatusUnauthorized && externalAuth {
			_ = response.Body.Close()
			authHeaders, failure, err = s.refreshMCPGatewayUpstreamAuth(ctx, client, token, initiatorID, cfg)
			if err == nil && failure == nil {
				response, err = doMCPGatewayUpstreamRequest(ctx, r, cfg.GetUrl(), body, "application/json, text/event-stream", authHeaders)
			}
		}
		if response != nil {
			defer response.Body.Close()
			if err == nil && failure == nil {
				contentType := strings.ToLower(response.Header.Get("Content-Type"))
				switch {
				case strings.Contains(contentType, "text/event-stream"):
					s.recordMCPGatewayToolUsages(ctx, client, interceptionID, cfg, []mcpGatewayToolCall{call})
					relayMCPGatewaySSE(rw, controller, response.Body)
					return
				case strings.Contains(contentType, "application/json"):
					responseBody, readErr := io.ReadAll(response.Body)
					if readErr == nil {
						var compact bytes.Buffer
						if json.Compact(&compact, responseBody) == nil {
							responseBody = compact.Bytes()
						}
						s.recordMCPGatewayToolUsages(ctx, client, interceptionID, cfg, []mcpGatewayToolCall{call})
						writeMCPGatewaySSEMessage(rw, controller, responseBody)
						return
					}
					err = readErr
				default:
					err = xerrors.New("upstream MCP server did not return JSON or SSE")
				}
			}
		}
	}

	message := "failed to forward approved MCP gateway escalation"
	data := map[string]any{"escalation_id": call.EscalationID, "status": "approved"}
	if failure != nil {
		message = failure.Message
		for key, value := range failure.Data {
			data[key] = value
		}
	}
	call.InvocationError = message
	s.recordMCPGatewayToolUsages(ctx, client, interceptionID, cfg, []mcpGatewayToolCall{call})
	if err != nil {
		s.logger.Warn(ctx, message, slog.F("escalation_id", call.EscalationID), slog.Error(err))
	}
	writeMCPGatewaySSEMessage(rw, controller, marshalMCPGatewayError(call.JSONRPCIDRaw, mcp.INTERNAL_ERROR, message, data))
}

func writeMCPGatewaySSEMessage(rw http.ResponseWriter, controller *http.ResponseController, body []byte) {
	_, _ = fmt.Fprintf(rw, "event: message\ndata: %s\n\n", bytes.TrimSpace(body))
	_ = controller.Flush()
}

func relayMCPGatewaySSE(rw http.ResponseWriter, controller *http.ResponseController, body io.Reader) {
	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			_, _ = io.WriteString(rw, line)
			if line == "\n" || line == "\r\n" {
				_ = controller.Flush()
			}
		}
		if err != nil {
			_ = controller.Flush()
			return
		}
	}
}

func mcpGatewayResponseID(request mcpGatewayRequest) json.RawMessage {
	for _, item := range request.Items {
		if len(bytes.TrimSpace(item.Envelope.ID)) > 0 {
			return item.Envelope.ID
		}
	}
	return nil
}

func (s *Server) forwardMCPGatewayResponse(
	rw http.ResponseWriter,
	r *http.Request,
	client DRPCClient,
	token string,
	initiatorID string,
	cfg *proto.MCPGatewayServerConfig,
	body []byte,
	plan *mcpGatewayPlan,
	responseID json.RawMessage,
) {
	acceptOverride := ""
	if plan != nil && plan.filterResponse {
		// Streamable HTTP upstreams require POST requests to accept both
		// JSON and SSE; strict servers such as GitHub's MCP server reject
		// requests that list only one. Send both and filter whichever
		// representation the upstream chooses.
		acceptOverride = "application/json, text/event-stream"
	}

	authHeaders, externalAuth, failure, err := s.resolveMCPGatewayUpstreamAuth(r.Context(), client, token, initiatorID, cfg)
	if err != nil {
		s.logger.Warn(r.Context(), "failed to resolve upstream MCP credentials", slog.F("server_slug", cfg.GetSlug()), slog.Error(err))
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(responseID, mcp.INTERNAL_ERROR, "failed to resolve upstream MCP credentials", nil))
		return
	}
	if failure != nil {
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(responseID, mcp.INTERNAL_ERROR, failure.Message, failure.Data))
		return
	}

	response, err := doMCPGatewayUpstreamRequest(r.Context(), r, cfg.GetUrl(), body, acceptOverride, authHeaders)
	if err != nil {
		s.logger.Warn(r.Context(), "upstream MCP request failed", slog.F("server_slug", cfg.GetSlug()), slog.Error(err))
		http.Error(rw, "upstream MCP request failed", http.StatusBadGateway)
		return
	}
	if response.StatusCode == http.StatusUnauthorized && externalAuth {
		_ = response.Body.Close()
		authHeaders, failure, err = s.refreshMCPGatewayUpstreamAuth(r.Context(), client, token, initiatorID, cfg)
		if err != nil {
			s.logger.Warn(r.Context(), "failed to refresh upstream MCP credentials", slog.F("server_slug", cfg.GetSlug()), slog.Error(err))
			writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(responseID, mcp.INTERNAL_ERROR, "failed to refresh upstream MCP credentials", nil))
			return
		}
		if failure != nil {
			writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(responseID, mcp.INTERNAL_ERROR, failure.Message, failure.Data))
			return
		}
		response, err = doMCPGatewayUpstreamRequest(r.Context(), r, cfg.GetUrl(), body, acceptOverride, authHeaders)
		if err != nil {
			s.logger.Warn(r.Context(), "upstream MCP retry failed", slog.F("server_slug", cfg.GetSlug()), slog.Error(err))
			http.Error(rw, "upstream MCP request failed", http.StatusBadGateway)
			return
		}
	}
	defer response.Body.Close()

	if plan == nil || (!plan.filterResponse && len(plan.toolsListIDs) == 0) {
		copyMCPGatewayResponse(rw, response)
		return
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		copyMCPGatewayResponse(rw, response)
		return
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		s.relayFilteredMCPGatewaySSE(r.Context(), rw, response, *plan, cfg.GetSlug())
		return
	}
	if !strings.Contains(contentType, "application/json") {
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, "upstream MCP server did not return JSON or SSE for a policy-filtered request", nil))
		return
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, "failed to read upstream MCP response", nil))
		return
	}
	filtered, err := filterMCPGatewayResponse(responseBody, *plan)
	if err != nil {
		s.logger.Warn(r.Context(), "failed to filter upstream MCP response", slog.F("server_slug", cfg.GetSlug()), slog.Error(err))
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, "failed to filter upstream MCP response", nil))
		return
	}
	copyMCPGatewayHeaders(rw.Header(), response.Header)
	writeMCPGatewayJSON(rw, response.StatusCode, filtered)
}

func (s *Server) resolveMCPGatewayUpstreamAuth(
	ctx context.Context,
	client DRPCClient,
	token string,
	initiatorID string,
	cfg *proto.MCPGatewayServerConfig,
) (http.Header, bool, *mcpGatewayCredentialFailure, error) {
	headers := make(http.Header)
	switch authType := strings.TrimSpace(cfg.GetAuthType()); authType {
	case "", "none":
		return headers, false, nil, nil
	case "api_key":
		headerName := strings.TrimSpace(cfg.GetApiKeyHeader())
		if headerName == "" || cfg.GetApiKeyValue() == "" {
			return nil, false, nil, xerrors.New("MCP API key header and value are required")
		}
		headers.Set(headerName, cfg.GetApiKeyValue())
		return headers, false, nil, nil
	case "custom_headers":
		var configured map[string]string
		if err := json.Unmarshal([]byte(cfg.GetCustomHeaders()), &configured); err != nil {
			return nil, false, nil, xerrors.Errorf("decode MCP custom headers: %w", err)
		}
		for key, value := range configured {
			headers.Set(key, value)
		}
		return headers, false, nil, nil
	case "user_oidc", "oauth2":
		return nil, false, &mcpGatewayCredentialFailure{
			Message: fmt.Sprintf("MCP gateway does not support upstream auth type %q yet", authType),
			Data:    map[string]any{"auth_type": authType},
		}, nil
	case "external_auth":
		providerID := strings.TrimSpace(cfg.GetExternalAuthProviderId())
		if providerID == "" {
			return nil, true, nil, xerrors.New("external auth provider ID is required")
		}
		cacheKey := mcpGatewayCredentialCacheKey{
			InitiatorID: initiatorID,
			ServerID:    cfg.GetId(),
		}
		if cached, ok := s.mcpCredentialCache.Load(cacheKey); ok {
			entry, valid := cached.(mcpGatewayCredentialCacheEntry)
			if valid && time.Now().Before(entry.ExpiresAt) && entry.ProviderID == providerID {
				headers.Set("Authorization", "Bearer "+entry.AccessToken)
				return headers, true, nil, nil
			}
			s.mcpCredentialCache.Delete(cacheKey)
		}

		credential, err := client.GetMCPUpstreamCredential(ctx, &proto.GetMCPUpstreamCredentialRequest{
			Key:                    token,
			ExternalAuthProviderId: providerID,
		})
		if err != nil {
			return nil, true, nil, xerrors.Errorf("get MCP upstream credential: %w", err)
		}
		if credential.GetReauthRequired() {
			return nil, true, &mcpGatewayCredentialFailure{
				Message: fmt.Sprintf("Authentication is required for MCP provider %q. Ask the user to authenticate using the provided URL.", credential.GetProviderId()),
				Data: map[string]any{
					"reauth_url":  credential.GetReauthUrl(),
					"provider_id": credential.GetProviderId(),
				},
			}, nil
		}
		if credential.GetAccessToken() == "" {
			return nil, true, nil, xerrors.New("MCP upstream credential is empty")
		}
		entry := mcpGatewayCredentialCacheEntry{
			AccessToken: credential.GetAccessToken(),
			ProviderID:  providerID,
			ExpiresAt:   time.Now().Add(mcpGatewayCredentialTTL),
		}
		s.mcpCredentialCache.Store(cacheKey, entry)
		headers.Set("Authorization", "Bearer "+entry.AccessToken)
		return headers, true, nil, nil
	default:
		return nil, false, nil, xerrors.Errorf("unsupported MCP auth type %q", authType)
	}
}

func (s *Server) refreshMCPGatewayUpstreamAuth(
	ctx context.Context,
	client DRPCClient,
	token string,
	initiatorID string,
	cfg *proto.MCPGatewayServerConfig,
) (http.Header, *mcpGatewayCredentialFailure, error) {
	s.mcpCredentialCache.Delete(mcpGatewayCredentialCacheKey{
		InitiatorID: initiatorID,
		ServerID:    cfg.GetId(),
	})
	headers, _, failure, err := s.resolveMCPGatewayUpstreamAuth(ctx, client, token, initiatorID, cfg)
	return headers, failure, err
}

func doMCPGatewayUpstreamRequest(
	ctx context.Context,
	incoming *http.Request,
	upstreamURL string,
	body []byte,
	acceptOverride string,
	authHeaders http.Header,
) (*http.Response, error) {
	upstreamRequest, err := newMCPGatewayUpstreamRequest(ctx, incoming, upstreamURL, body, acceptOverride, authHeaders)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(upstreamRequest)
}

func newMCPGatewayUpstreamRequest(
	ctx context.Context,
	incoming *http.Request,
	upstreamURL string,
	body []byte,
	acceptOverride string,
	authHeaders http.Header,
) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, incoming.Method, upstreamURL, reader)
	if err != nil {
		return nil, err
	}
	for _, header := range []string{
		"Accept",
		"Content-Type",
		"Last-Event-ID",
		"Mcp-Session-Id",
		"MCP-Protocol-Version",
		"User-Agent",
	} {
		if value := incoming.Header.Values(header); len(value) > 0 {
			request.Header[header] = append([]string(nil), value...)
		}
	}
	for key, values := range authHeaders {
		request.Header[key] = append([]string(nil), values...)
	}
	if acceptOverride != "" {
		request.Header.Set("Accept", acceptOverride)
	}
	return request, nil
}

func filterMCPGatewayResponse(body []byte, plan mcpGatewayPlan) ([]byte, error) {
	if plan.batch {
		var responses []json.RawMessage
		if err := json.Unmarshal(body, &responses); err != nil {
			return nil, xerrors.Errorf("decode JSON-RPC batch response: %w", err)
		}
		filtered := make([]json.RawMessage, 0, len(responses)+len(plan.local))
		for _, response := range responses {
			updated, err := filterMCPGatewayResponseObject(response, plan)
			if err != nil {
				return nil, err
			}
			filtered = append(filtered, updated)
		}
		filtered = append(filtered, plan.local...)
		return json.Marshal(filtered)
	}
	updated, err := filterMCPGatewayResponseObject(body, plan)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// relayFilteredMCPGatewaySSE relays an upstream Streamable HTTP SSE response
// to the client, rewriting tools/list results according to the gateway tool
// policy. Locally-generated responses (for example denied tool calls in a
// batch) are emitted as leading message events because the upstream never saw
// those requests.
func (s *Server) relayFilteredMCPGatewaySSE(ctx context.Context, rw http.ResponseWriter, response *http.Response, plan mcpGatewayPlan, slug string) {
	copyMCPGatewayHeaders(rw.Header(), response.Header)
	rw.WriteHeader(response.StatusCode)
	controller := http.NewResponseController(rw)

	for _, local := range plan.local {
		_, _ = fmt.Fprintf(rw, "event: message\ndata: %s\n\n", local)
	}
	if len(plan.local) > 0 {
		_ = controller.Flush()
	}

	reader := bufio.NewReader(response.Body)
	var lines []string
	emit := func() {
		if len(lines) == 0 {
			return
		}
		for _, line := range s.filterMCPGatewaySSEEvent(ctx, lines, plan, slug) {
			_, _ = io.WriteString(rw, line+"\n")
		}
		_, _ = io.WriteString(rw, "\n")
		_ = controller.Flush()
		lines = lines[:0]
	}
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			emit()
		} else {
			lines = append(lines, line)
		}
		if err != nil {
			emit()
			return
		}
	}
}

// filterMCPGatewaySSEEvent rewrites a single SSE event's data payload when it
// carries a JSON-RPC response subject to tools/list filtering. Events without
// a data field, or whose payload is not subject to filtering, pass through
// unchanged. Filter failures fail closed: the event's payload is replaced
// with a JSON-RPC error rather than forwarded unfiltered.
func (s *Server) filterMCPGatewaySSEEvent(ctx context.Context, lines []string, plan mcpGatewayPlan, slug string) []string {
	var data []string
	for _, line := range lines {
		if value, ok := mcpGatewaySSEFieldValue(line, "data"); ok {
			data = append(data, value)
		}
	}
	if len(data) == 0 {
		return lines
	}
	payload := []byte(strings.Join(data, "\n"))
	filtered, err := filterMCPGatewaySSEPayload(payload, plan)
	if err != nil {
		s.logger.Warn(ctx, "failed to filter upstream MCP SSE event", slog.F("server_slug", slug), slog.Error(err))
		filtered = marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, "failed to filter upstream MCP response", nil)
	}
	if bytes.Equal(bytes.TrimSpace(payload), bytes.TrimSpace(filtered)) {
		return lines
	}
	rewritten := make([]string, 0, len(lines))
	replaced := false
	for _, line := range lines {
		if _, ok := mcpGatewaySSEFieldValue(line, "data"); ok {
			if !replaced {
				rewritten = append(rewritten, "data: "+string(filtered))
				replaced = true
			}
			continue
		}
		rewritten = append(rewritten, line)
	}
	return rewritten
}

// filterMCPGatewaySSEPayload applies tools/list filtering to a JSON-RPC
// message (or batch of messages) carried in an SSE data payload. Payloads
// that are not JSON objects or arrays pass through unchanged.
func filterMCPGatewaySSEPayload(payload []byte, plan mcpGatewayPlan) ([]byte, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return payload, nil
	}
	if trimmed[0] == '[' {
		var responses []json.RawMessage
		if err := json.Unmarshal(trimmed, &responses); err != nil {
			return payload, nil
		}
		changed := false
		filtered := make([]json.RawMessage, 0, len(responses))
		for _, response := range responses {
			updated, err := filterMCPGatewayResponseObject(response, plan)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(updated, response) {
				changed = true
			}
			filtered = append(filtered, updated)
		}
		if !changed {
			return payload, nil
		}
		return json.Marshal(filtered)
	}
	return filterMCPGatewayResponseObject(trimmed, plan)
}

// mcpGatewaySSEFieldValue extracts the value of an SSE field line such as
// "data: {...}". Per the SSE specification a single space after the colon is
// stripped; field names are case-sensitive.
func mcpGatewaySSEFieldValue(line, field string) (string, bool) {
	rest, ok := strings.CutPrefix(line, field)
	if !ok {
		return "", false
	}
	value, ok := strings.CutPrefix(rest, ":")
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(value, " "), true
}

func filterMCPGatewayResponseObject(body []byte, plan mcpGatewayPlan) ([]byte, error) {
	var response map[string]json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, xerrors.Errorf("decode JSON-RPC response: %w", err)
	}
	key, ok := mcpGatewayIDKey(response["id"])
	if !ok {
		return body, nil
	}
	if _, filter := plan.toolsListIDs[key]; !filter {
		return body, nil
	}
	resultBody, ok := response["result"]
	if !ok {
		return body, nil
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(resultBody, &result); err != nil {
		return nil, xerrors.Errorf("decode tools/list result: %w", err)
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(result["tools"], &tools); err != nil {
		return nil, xerrors.Errorf("decode tools/list tools: %w", err)
	}
	allowed := make([]json.RawMessage, 0, len(tools))
	for _, tool := range tools {
		var descriptor struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(tool, &descriptor); err != nil {
			return nil, xerrors.Errorf("decode tools/list tool: %w", err)
		}
		// Escalated tools stay listed: the model must be able to see a tool
		// to call it, and the approval hold happens at call time.
		if plan.policy.evaluate(descriptor.Name) != mcptools.ActionBlock {
			allowed = append(allowed, tool)
		}
	}
	encodedTools, err := json.Marshal(allowed)
	if err != nil {
		return nil, err
	}
	result["tools"] = encodedTools
	encodedResult, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	response["result"] = encodedResult
	return json.Marshal(response)
}

func writeMCPGatewayLocalResponses(rw http.ResponseWriter, plan mcpGatewayPlan) {
	if len(plan.local) == 0 {
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	if plan.batch {
		encoded, err := json.Marshal(plan.local)
		if err != nil {
			writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, "failed to encode JSON-RPC response", nil))
			return
		}
		writeMCPGatewayJSON(rw, http.StatusOK, encoded)
		return
	}
	writeMCPGatewayJSON(rw, http.StatusOK, plan.local[0])
}

func writeMCPGatewayJSON(rw http.ResponseWriter, status int, body []byte) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_, _ = rw.Write(body)
}

func copyMCPGatewayResponse(rw http.ResponseWriter, response *http.Response) {
	copyMCPGatewayHeaders(rw.Header(), response.Header)
	rw.WriteHeader(response.StatusCode)
	buffer := make([]byte, 32*1024)
	controller := http.NewResponseController(rw)
	for {
		n, err := response.Body.Read(buffer)
		if n > 0 {
			_, _ = rw.Write(buffer[:n])
			_ = controller.Flush()
		}
		if err != nil {
			return
		}
	}
}

func copyMCPGatewayHeaders(dst, src http.Header) {
	for key, values := range src {
		switch strings.ToLower(key) {
		case "connection", "content-length", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
}
