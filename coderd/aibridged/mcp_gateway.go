package aibridged

import (
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
	forward      []json.RawMessage
	local        []json.RawMessage
	toolCalls    []mcpGatewayToolCall
	toolsListIDs map[string]struct{}
	forceJSON    bool
	batch        bool
	policy       mcpGatewayPolicy
}

type mcpGatewayToolCall struct {
	Tool            string
	Input           string
	JSONRPCID       string
	InvocationError string
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

func (p mcpGatewayPolicy) allowed(tool string) bool {
	if !mcptools.Allowed(p.tools, tool) {
		return false
	}
	if p.denylist != nil && p.denylist.MatchString(tool) {
		return false
	}
	return p.allowlist == nil || p.allowlist.MatchString(tool)
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
				plan.forceJSON = true
			}
		case mcp.MethodToolsCall:
			call := mcpGatewayToolCall{JSONRPCID: mcpGatewayIDValue(item.Envelope.ID)}
			var params mcp.CallToolParams
			if err := json.Unmarshal(item.Envelope.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
				call.Tool = params.Name
				call.Input = "null"
				call.InvocationError = "invalid tools/call parameters"
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
			if !policy.allowed(params.Name) {
				call.InvocationError = fmt.Sprintf("tool %q denied by MCP gateway policy", params.Name)
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
			}
			plan.toolCalls = append(plan.toolCalls, call)
			plan.forward = append(plan.forward, item.Raw)
		default:
			plan.forward = append(plan.forward, item.Raw)
		}
	}
	if len(plan.local) > 0 {
		plan.forceJSON = true
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
	toolCalls []mcpGatewayToolCall,
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
		})
		if err != nil {
			s.logger.Warn(ctx, "failed to record MCP gateway tool usage", slog.F("server_slug", cfg.GetSlug()), slog.F("tool", call.Tool), slog.Error(err))
		}
	}
	return interceptionID
}

func (s *Server) endMCPGatewayRecording(ctx context.Context, client DRPCClient, interceptionID, slug string) {
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
		http.NotFound(rw, r)
		return
	}

	configResponse, err := client.GetMCPGatewayServerConfig(ctx, &proto.GetMCPGatewayServerConfigRequest{Slug: slug})
	if err != nil {
		s.logger.Warn(ctx, "failed to retrieve MCP gateway server config", slog.F("server_slug", slug), slog.Error(err))
		http.Error(rw, "failed to retrieve MCP server configuration", http.StatusInternalServerError)
		return
	}
	if !configResponse.GetFound() || configResponse.GetConfig() == nil {
		http.NotFound(rw, r)
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
	if len(plan.toolCalls) > 0 {
		interceptionID := s.startMCPGatewayRecording(ctx, client, authz, cfg, r.UserAgent(), plan.toolCalls)
		if interceptionID != "" {
			defer s.endMCPGatewayRecording(ctx, client, interceptionID, cfg.GetSlug())
		}
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
	if plan != nil && plan.forceJSON {
		acceptOverride = "application/json"
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

	if plan == nil || (!plan.forceJSON && len(plan.toolsListIDs) == 0) {
		copyMCPGatewayResponse(rw, response)
		return
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		copyMCPGatewayResponse(rw, response)
		return
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "application/json") {
		writeMCPGatewayJSON(rw, http.StatusOK, marshalMCPGatewayError(nil, mcp.INTERNAL_ERROR, "upstream MCP server did not return JSON for a policy-filtered request", nil))
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
		if plan.policy.allowed(descriptor.Name) {
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
