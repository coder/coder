package aibridgedserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpmw"
	codermcp "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

var (
	ErrExpiredOrInvalidOAuthToken = xerrors.New("expired or invalid OAuth2 token")
	ErrNoMCPConfigFound           = xerrors.New("no MCP config found")

	// These errors are returned by IsAuthorized. Since they're just returned as
	// a generic dRPC error, it's difficult to tell them apart without string
	// matching.
	// TODO: return these errors to the client in a more structured/comparable
	//       way.
	ErrInvalidKey    = xerrors.New("invalid key")
	ErrUnknownKey    = xerrors.New("unknown key")
	ErrExpired       = xerrors.New("expired")
	ErrUnknownUser   = xerrors.New("unknown user")
	ErrDeletedUser   = xerrors.New("deleted user")
	ErrInactiveUser  = xerrors.New("inactive user")
	ErrSystemUser    = xerrors.New("system user")
	ErrAmbiguousAuth = xerrors.New("both key and key_id set; exactly one required")

	ErrNoExternalAuthLinkFound = xerrors.New("no external auth link found")
)

const (
	InterceptionLogMarker = "interception log"
	MetadataUserAgentKey  = "request_user_agent"
)

var _ aibridged.DRPCServer = &Server{}

type store interface {
	// Recorder-related queries.
	InsertAIBridgeInterception(ctx context.Context, arg database.InsertAIBridgeInterceptionParams) (database.AIBridgeInterception, error)
	InsertAIBridgeTokenUsage(ctx context.Context, arg database.InsertAIBridgeTokenUsageParams) (database.AIBridgeTokenUsage, error)
	InsertAIBridgeUserPrompt(ctx context.Context, arg database.InsertAIBridgeUserPromptParams) (database.AIBridgeUserPrompt, error)
	InsertAIBridgeToolUsage(ctx context.Context, arg database.InsertAIBridgeToolUsageParams) (database.AIBridgeToolUsage, error)
	InsertAIBridgeModelThought(ctx context.Context, arg database.InsertAIBridgeModelThoughtParams) (database.AIBridgeModelThought, error)
	UpdateAIBridgeInterceptionEnded(ctx context.Context, intcID database.UpdateAIBridgeInterceptionEndedParams) (database.AIBridgeInterception, error)
	GetAIBridgeInterceptionLineageByToolCallID(ctx context.Context, toolCallID string) (database.GetAIBridgeInterceptionLineageByToolCallIDRow, error)

	// Cost-attribution queries, used to snapshot price and effective group on
	// each token usage record.
	GetAIBridgeInterceptionByID(ctx context.Context, id uuid.UUID) (database.AIBridgeInterception, error)
	GetAIModelPriceByProviderModel(ctx context.Context, arg database.GetAIModelPriceByProviderModelParams) (database.AIModelPrice, error)
	GetUserAIBudgetOverride(ctx context.Context, userID uuid.UUID) (database.UserAIBudgetOverride, error)
	GetHighestGroupAIBudgetByUser(ctx context.Context, userID uuid.UUID) (database.GetHighestGroupAIBudgetByUserRow, error)
	GetUserAISpendSince(ctx context.Context, arg database.GetUserAISpendSinceParams) (database.GetUserAISpendSinceRow, error)

	// MCPConfigurator-related queries.
	GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error)

	// Authorizer-related queries.
	GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (database.User, error)

	// ProviderConfigurator-related queries. InTx wraps the provider and key
	// reads in a single read-only transaction; AcquireLock serializes against
	// any in-flight env seed holding LockIDAIProvidersEnvSeed.
	GetAIProviders(ctx context.Context, arg database.GetAIProvidersParams) ([]database.AIProvider, error)
	GetAIProviderKeysByProviderIDs(ctx context.Context, providerIDs []uuid.UUID) ([]database.AIProviderKey, error)

	InTx(func(database.Store) error, *database.TxOptions) error
}

type Server struct {
	// lifecycleCtx must be tied to the API server's lifecycle
	// as when the API server shuts down, we want to cancel any
	// long-running operations.
	lifecycleCtx        context.Context
	store               store
	logger              slog.Logger
	externalAuthConfigs map[string]*externalauth.Config

	coderMCPConfig    *proto.MCPServerConfig // may be nil if not available
	structuredLogging bool
	aiSeatTracker     aiseats.SeatTracker
	// budgetPolicy selects the effective group when a user belongs to multiple
	// budgeted groups, used for cost attribution on token usage records.
	budgetPolicy codersdk.AIBudgetPolicy
}

func NewServer(lifecycleCtx context.Context, store store, logger slog.Logger, accessURL string,
	bridgeCfg codersdk.AIBridgeConfig, externalAuthConfigs []*externalauth.Config, experiments codersdk.Experiments,
	aiSeatTracker aiseats.SeatTracker,
) (*Server, error) {
	eac := make(map[string]*externalauth.Config, len(externalAuthConfigs))

	for _, cfg := range externalAuthConfigs {
		// Only External Auth configs which are configured with an MCP URL are relevant to aibridged.
		if cfg.MCPURL == "" {
			continue
		}
		eac[cfg.ID] = cfg
	}

	srv := &Server{
		lifecycleCtx:        lifecycleCtx,
		store:               store,
		logger:              logger,
		externalAuthConfigs: eac,
		structuredLogging:   bridgeCfg.StructuredLogging.Value(),
		aiSeatTracker:       aiSeatTracker,
		budgetPolicy:        codersdk.NewAIBudgetPolicyFromString(bridgeCfg.BudgetPolicy),
	}

	if bridgeCfg.InjectCoderMCPTools {
		logger.Warn(lifecycleCtx, "inject MCP tools option is deprecated and will be removed in a future release")
		coderMCPConfig, err := getCoderMCPServerConfig(experiments, accessURL)
		if err != nil {
			logger.Warn(lifecycleCtx, "failed to retrieve coder MCP server config, Coder MCP will not be available", slog.Error(err))
		}
		srv.coderMCPConfig = coderMCPConfig
	}

	return srv, nil
}

func (s *Server) RecordInterception(ctx context.Context, in *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	intcID, err := uuid.Parse(in.GetId())
	if err != nil {
		return nil, xerrors.Errorf("invalid interception ID %q: %w", in.GetId(), err)
	}
	initID, err := uuid.Parse(in.GetInitiatorId())
	if err != nil {
		return nil, xerrors.Errorf("invalid initiator ID %q: %w", in.GetInitiatorId(), err)
	}
	if in.ApiKeyId == "" {
		return nil, xerrors.Errorf("empty API key ID")
	}

	metadata := metadataToMap(in.GetMetadata())

	if in.UserAgent != "" {
		if _, ok := metadata[MetadataUserAgentKey]; ok {
			s.logger.Warn(ctx, "interception metadata contains user agent key, will be overwritten")
		}
		metadata[MetadataUserAgentKey] = in.UserAgent
	}

	// Look up the interception lineage using the correlating tool call ID.
	parentID, rootID := s.findInterceptionLineage(ctx, in.GetCorrelatingToolCallId())

	if s.structuredLogging {
		s.logger.Info(ctx, InterceptionLogMarker,
			slog.F("record_type", "interception_start"),
			slog.F("interception_id", intcID.String()),
			slog.F("initiator_id", initID.String()),
			slog.F("api_key_id", in.ApiKeyId),
			slog.F("provider", in.Provider),
			slog.F("model", in.Model),
			slog.F("client", in.Client),
			slog.F("client_session_id", in.GetClientSessionId()),
			slog.F("started_at", in.StartedAt.AsTime()),
			slog.F("metadata", metadata),
			slog.F("correlating_tool_call_id", in.GetCorrelatingToolCallId()),
			slog.F("thread_parent_id", parentID),
			slog.F("thread_root_id", rootID),
		)
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn(ctx, "failed to marshal aibridge metadata from proto to JSON", slog.F("metadata", in), slog.Error(err))
	}

	providerName := strings.TrimSpace(in.ProviderName)
	if providerName == "" {
		providerName = in.Provider
	}

	agentFirewallSessionID, err := parseOptionalUUID(in.AgentFirewallSessionId)
	if err != nil {
		s.logger.Warn(ctx, "invalid agent firewall session ID in interception request",
			slog.F("agent_firewall_session_id", in.GetAgentFirewallSessionId()), slog.Error(err))
	}

	_, err = s.store.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
		ID:                          intcID,
		APIKeyID:                    sql.NullString{String: in.ApiKeyId, Valid: true},
		Client:                      sql.NullString{String: in.Client, Valid: in.Client != ""},
		ClientSessionID:             sql.NullString{String: in.GetClientSessionId(), Valid: in.GetClientSessionId() != ""},
		InitiatorID:                 initID,
		Provider:                    in.Provider,
		ProviderName:                providerName,
		Model:                       in.Model,
		Metadata:                    out,
		StartedAt:                   in.StartedAt.AsTime(),
		ThreadParentInterceptionID:  uuid.NullUUID{UUID: parentID, Valid: parentID != uuid.Nil},
		ThreadRootInterceptionID:    uuid.NullUUID{UUID: rootID, Valid: rootID != uuid.Nil},
		CredentialKind:              credentialKindOrDefault(in.CredentialKind),
		CredentialHint:              in.CredentialHint,
		AgentFirewallSessionID:      agentFirewallSessionID,
		AgentFirewallSequenceNumber: parseOptionalInt32(in.AgentFirewallSequenceNumber),
	})
	if err != nil {
		return nil, xerrors.Errorf("start interception: %w", err)
	}

	reason := aiseats.ReasonAIBridge("provider=" + in.Provider + ", model=" + in.Model)
	s.aiSeatTracker.RecordUsage(ctx, initID, reason)
	return &proto.RecordInterceptionResponse{}, nil
}

func (s *Server) RecordInterceptionEnded(ctx context.Context, in *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	intcID, err := uuid.Parse(in.GetId())
	if err != nil {
		return nil, xerrors.Errorf("invalid interception ID %q: %w", in.GetId(), err)
	}

	if s.structuredLogging {
		s.logger.Info(ctx, InterceptionLogMarker,
			slog.F("record_type", "interception_end"),
			slog.F("interception_id", intcID.String()),
			slog.F("ended_at", in.EndedAt.AsTime()),
		)
	}

	_, err = s.store.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
		ID:             intcID,
		EndedAt:        in.EndedAt.AsTime(),
		CredentialHint: in.CredentialHint,
	})
	if err != nil {
		return nil, xerrors.Errorf("end interception: %w", err)
	}

	return &proto.RecordInterceptionEndedResponse{}, nil
}

func (s *Server) RecordTokenUsage(ctx context.Context, in *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	intcID, err := uuid.Parse(in.GetInterceptionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse interception_id %q: %w", in.GetInterceptionId(), err)
	}

	metadata := metadataToMap(in.GetMetadata())

	if s.structuredLogging {
		s.logger.Info(ctx, InterceptionLogMarker,
			slog.F("record_type", "token_usage"),
			slog.F("interception_id", intcID.String()),
			slog.F("msg_id", in.GetMsgId()),
			slog.F("input_tokens", in.GetInputTokens()),
			slog.F("output_tokens", in.GetOutputTokens()),
			slog.F("cache_read_input_tokens", in.GetCacheReadInputTokens()),
			slog.F("cache_write_input_tokens", in.GetCacheWriteInputTokens()),
			slog.F("created_at", in.GetCreatedAt().AsTime()),
			slog.F("metadata", metadata),
		)
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn(ctx, "failed to marshal aibridge metadata from proto to JSON", slog.F("metadata", in), slog.Error(err))
	}

	// The interception is always recorded before any of its token usages,
	// so it must exist. It carries the provider, model, and initiator needed
	// for cost attribution.
	intc, err := s.store.GetAIBridgeInterceptionByID(ctx, intcID)
	if err != nil {
		return nil, xerrors.Errorf("get interception %q: %w", intcID, err)
	}

	// Snapshot the effective group, per-token prices and compute cost. A
	// missing price row or no effective group yields NULL columns.
	cost, err := s.resolveTokenUsageCost(ctx, intc, in)
	if err != nil {
		return nil, xerrors.Errorf("resolve token usage cost: %w", err)
	}

	if err := s.recordTokenUsageAndSpend(ctx, intc, cost, in, out); err != nil {
		return nil, xerrors.Errorf("record token usage and spend: %w", err)
	}

	return &proto.RecordTokenUsageResponse{}, nil
}

// recordTokenUsageAndSpend atomically records the token usage (including the
// interception's cost) and, when the user is budgeted and the computed cost is
// positive, accumulates that cost into the user's daily spend.
func (s *Server) recordTokenUsageAndSpend(ctx context.Context, intc database.AIBridgeInterception, cost tokenUsageCost, in *proto.RecordTokenUsageRequest, metadataJSON []byte) error {
	createdAt := in.GetCreatedAt().AsTime()
	return s.store.InTx(func(tx database.Store) error {
		if _, err := tx.InsertAIBridgeTokenUsage(ctx, database.InsertAIBridgeTokenUsageParams{
			ID:                    uuid.New(),
			InterceptionID:        intc.ID,
			ProviderResponseID:    in.GetMsgId(),
			InputTokens:           in.GetInputTokens(),
			OutputTokens:          in.GetOutputTokens(),
			CacheReadInputTokens:  in.GetCacheReadInputTokens(),
			CacheWriteInputTokens: in.GetCacheWriteInputTokens(),
			Metadata:              metadataJSON,
			CreatedAt:             createdAt,
			EffectiveGroupID:      cost.effectiveGroupID,
			InputPriceMicros:      cost.inputPriceMicros,
			OutputPriceMicros:     cost.outputPriceMicros,
			CacheReadPriceMicros:  cost.cacheReadPriceMicros,
			CacheWritePriceMicros: cost.cacheWritePriceMicros,
			CostMicros:            cost.costMicros,
		}); err != nil {
			return xerrors.Errorf("insert token usage: %w", err)
		}

		// Skip the spend update when there is no effective group or the interception has no cost.
		if !cost.effectiveGroupID.Valid || !cost.costMicros.Valid || cost.costMicros.Int64 <= 0 {
			s.logger.Debug(ctx, "skipping spend update",
				slog.F("interception_id", intc.ID),
				slog.F("initiator_id", intc.InitiatorID),
				slog.F("has_effective_group", cost.effectiveGroupID.Valid),
				slog.F("has_cost", cost.costMicros.Valid),
				slog.F("cost_micros", cost.costMicros.Int64),
			)
			return nil
		}

		if _, err := tx.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID:           intc.InitiatorID,
			EffectiveGroupID: cost.effectiveGroupID.UUID,
			// Day is derived from the record usage request CreatedAt
			// so it matches the token usage row's created_at column.
			Day:        dbtime.StartOfDay(createdAt.UTC()),
			CostMicros: cost.costMicros.Int64,
		}); err != nil {
			return xerrors.Errorf("increment user daily spend: %w", err)
		}
		return nil
	}, nil)
}

func (s *Server) RecordPromptUsage(ctx context.Context, in *proto.RecordPromptUsageRequest) (*proto.RecordPromptUsageResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	intcID, err := uuid.Parse(in.GetInterceptionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse interception_id %q: %w", in.GetInterceptionId(), err)
	}

	metadata := metadataToMap(in.GetMetadata())

	if s.structuredLogging {
		s.logger.Info(ctx, InterceptionLogMarker,
			slog.F("record_type", "prompt_usage"),
			slog.F("interception_id", intcID.String()),
			slog.F("msg_id", in.GetMsgId()),
			slog.F("prompt", in.GetPrompt()),
			slog.F("created_at", in.GetCreatedAt().AsTime()),
			slog.F("metadata", metadata),
		)
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn(ctx, "failed to marshal aibridge metadata from proto to JSON", slog.F("metadata", in), slog.Error(err))
	}

	_, err = s.store.InsertAIBridgeUserPrompt(ctx, database.InsertAIBridgeUserPromptParams{
		ID:                 uuid.New(),
		InterceptionID:     intcID,
		ProviderResponseID: in.GetMsgId(),
		Prompt:             in.GetPrompt(),
		Metadata:           out,
		CreatedAt:          in.GetCreatedAt().AsTime(),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert user prompt: %w", err)
	}

	return &proto.RecordPromptUsageResponse{}, nil
}

func (s *Server) RecordToolUsage(ctx context.Context, in *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	intcID, err := uuid.Parse(in.GetInterceptionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse interception_id %q: %w", in.GetInterceptionId(), err)
	}

	metadata := metadataToMap(in.GetMetadata())

	if s.structuredLogging {
		s.logger.Info(ctx, InterceptionLogMarker,
			slog.F("record_type", "tool_usage"),
			slog.F("interception_id", intcID.String()),
			slog.F("msg_id", in.GetMsgId()),
			slog.F("tool_call_id", in.GetToolCallId()),
			slog.F("item_id", in.GetItemId()),
			slog.F("tool", in.GetTool()),
			slog.F("input", in.GetInput()),
			slog.F("server_url", in.GetServerUrl()),
			slog.F("injected", in.GetInjected()),
			slog.F("invocation_error", in.GetInvocationError()),
			slog.F("created_at", in.GetCreatedAt().AsTime()),
			slog.F("metadata", metadata),
		)
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn(ctx, "failed to marshal aibridge metadata from proto to JSON", slog.F("metadata", in), slog.Error(err))
	}

	_, err = s.store.InsertAIBridgeToolUsage(ctx, database.InsertAIBridgeToolUsageParams{
		ID:                 uuid.New(),
		InterceptionID:     intcID,
		ProviderResponseID: in.GetMsgId(),
		ProviderToolCallID: sql.NullString{String: in.GetToolCallId(), Valid: in.GetToolCallId() != ""},
		ProviderItemID:     sql.NullString{String: in.GetItemId(), Valid: in.GetItemId() != ""},
		ServerUrl:          sql.NullString{String: in.GetServerUrl(), Valid: in.ServerUrl != nil},
		Tool:               in.GetTool(),
		Input:              in.GetInput(),
		Injected:           in.GetInjected(),
		InvocationError:    sql.NullString{String: in.GetInvocationError(), Valid: in.InvocationError != nil},
		Metadata:           out,
		CreatedAt:          in.GetCreatedAt().AsTime(),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert tool usage: %w", err)
	}

	return &proto.RecordToolUsageResponse{}, nil
}

func (s *Server) RecordModelThought(ctx context.Context, in *proto.RecordModelThoughtRequest) (*proto.RecordModelThoughtResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	intcID, err := uuid.Parse(in.GetInterceptionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse interception_id %q: %w", in.GetInterceptionId(), err)
	}

	metadata := metadataToMap(in.GetMetadata())

	if s.structuredLogging {
		s.logger.Info(ctx, InterceptionLogMarker,
			slog.F("record_type", "model_thought"),
			slog.F("interception_id", intcID.String()),
			slog.F("content", in.GetContent()),
			slog.F("created_at", in.GetCreatedAt().AsTime()),
			slog.F("metadata", metadata),
		)
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn(ctx, "failed to marshal aibridge metadata from proto to JSON", slog.F("metadata", in), slog.Error(err))
	}

	_, err = s.store.InsertAIBridgeModelThought(ctx, database.InsertAIBridgeModelThoughtParams{
		InterceptionID: intcID,
		Content:        in.GetContent(),
		Metadata:       out,
		CreatedAt:      in.GetCreatedAt().AsTime(),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert model thought: %w", err)
	}

	return &proto.RecordModelThoughtResponse{}, nil
}

// findInterceptionLineage looks up the parent interception and the root
// of the thread by finding which interception recorded a tool usage with
// the given tool call ID. Returns (parentID, rootID); both will be
// uuid.Nil if no match is found or the tool call ID is empty.
func (s *Server) findInterceptionLineage(ctx context.Context, toolCallID string) (parent uuid.UUID, root uuid.UUID) {
	if toolCallID == "" {
		return uuid.Nil, uuid.Nil
	}

	lineage, err := s.store.GetAIBridgeInterceptionLineageByToolCallID(ctx, toolCallID)
	if err != nil {
		s.logger.Warn(ctx, "failed to retrieve interception lineage",
			slog.Error(err), slog.F("tool_call_id", toolCallID))
		return uuid.Nil, uuid.Nil
	}

	return lineage.ThreadParentID, lineage.ThreadRootID
}

func (s *Server) GetMCPServerConfigs(_ context.Context, _ *proto.GetMCPServerConfigsRequest) (*proto.GetMCPServerConfigsResponse, error) {
	cfgs := make([]*proto.MCPServerConfig, 0, len(s.externalAuthConfigs))
	for _, eac := range s.externalAuthConfigs {
		var allowlist, denylist string
		if eac.MCPToolAllowRegex != nil {
			allowlist = eac.MCPToolAllowRegex.String()
		}
		if eac.MCPToolDenyRegex != nil {
			denylist = eac.MCPToolDenyRegex.String()
		}

		cfgs = append(cfgs, &proto.MCPServerConfig{
			Id:             eac.ID,
			Url:            eac.MCPURL,
			ToolAllowRegex: allowlist,
			ToolDenyRegex:  denylist,
		})
	}

	return &proto.GetMCPServerConfigsResponse{
		CoderMcpConfig:         s.coderMCPConfig, // it's fine if this is nil
		ExternalAuthMcpConfigs: cfgs,
	}, nil
}

func (s *Server) GetMCPServerAccessTokensBatch(ctx context.Context, in *proto.GetMCPServerAccessTokensBatchRequest) (*proto.GetMCPServerAccessTokensBatchResponse, error) {
	if len(in.GetMcpServerConfigIds()) == 0 {
		return &proto.GetMCPServerAccessTokensBatchResponse{}, nil
	}

	userID, err := uuid.Parse(in.GetUserId())
	if err != nil {
		return nil, xerrors.Errorf("parse user_id: %w", err)
	}

	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)
	links, err := s.store.GetExternalAuthLinksByUserID(ctx, userID)
	if err != nil {
		return nil, xerrors.Errorf("fetch external auth links: %w", err)
	}

	if len(links) == 0 {
		return &proto.GetMCPServerAccessTokensBatchResponse{}, nil
	}

	// Ensure unique to prevent unnecessary effort.
	ids := in.GetMcpServerConfigIds()
	slices.Sort(ids)
	ids = slices.Compact(ids)

	var (
		wg   sync.WaitGroup
		errs error

		mu        sync.Mutex
		tokens    = make(map[string]string, len(ids))
		tokenErrs = make(map[string]string)
	)

externalAuthLoop:
	for _, id := range ids {
		eac, ok := s.externalAuthConfigs[id]
		if !ok {
			mu.Lock()
			s.logger.Warn(ctx, "no MCP server config found by given ID", slog.F("id", id))
			tokenErrs[id] = ErrNoMCPConfigFound.Error()
			mu.Unlock()
			continue
		}

		for _, link := range links {
			if link.ProviderID != eac.ID {
				continue
			}

			// Validate all configured External Auth links concurrently.
			wg.Add(1)
			go func() {
				defer wg.Done()

				// TODO: timeout.
				valid, _, validateErr := eac.ValidateToken(ctx, link.OAuthToken())
				mu.Lock()
				defer mu.Unlock()
				if !valid {
					// TODO: attempt refresh.
					s.logger.Warn(ctx, "invalid/expired access token, cannot auto-configure MCP", slog.F("provider", link.ProviderID), slog.Error(validateErr))
					tokenErrs[id] = ErrExpiredOrInvalidOAuthToken.Error()
					return
				}

				if validateErr != nil {
					errs = multierror.Append(errs, validateErr)
					tokenErrs[id] = validateErr.Error()
				} else {
					tokens[id] = link.OAuthAccessToken
				}
			}()

			continue externalAuthLoop
		}

		// No link found for this external auth config, so include a generic
		// error.
		mu.Lock()
		tokenErrs[id] = ErrNoExternalAuthLinkFound.Error()
		mu.Unlock()
	}

	wg.Wait()
	return &proto.GetMCPServerAccessTokensBatchResponse{
		AccessTokens: tokens,
		Errors:       tokenErrs,
	}, errs
}

// IsAuthorized validates a given Coder API key and returns the user ID to which it belongs (if valid).
//
// SECURITY: when in.KeyId is set (the "delegated" path), this method trusts the
// caller's claim of identity and skips the key-secret check. This DRPCServer is
// reachable both in-process via [aibridged.MemTransportPipe] and over the network
// via the /api/v2/ai-gateway/serve endpoint. That endpoint admits only holders of
// AI Gateway key, which are fully trusted. Standalone AI Gateway authenticates its
// own users and acts on their behalf, much like a provisioner daemon. A Gateway key
// holder can therefore act as any user without that user's secret. Per-user
// authorization on this surface is a known gap.
//
// NOTE: this should really be using the code from [httpmw.ExtractAPIKey]. That function not only validates the key
// but handles many other cases like updating last used, expiry, etc. This code does not currently use it for
// a few reasons:
//
//  1. [httpmw.ExtractAPIKey] relies on keys being given in specific headers [httpmw.APITokenFromRequest] which AI
//     bridge requests will not conform to.
//  2. The code mixes many different concerns, and handles HTTP responses too, which is undesirable here.
//  3. The core logic would need to be extracted, but that will surely be a complex & time-consuming distraction right now.
//  4. Once we have an Early Access release of AI Bridge, we need to return to this.
//
// TODO: replace with logic from [httpmw.ExtractAPIKey].
func (s *Server) IsAuthorized(ctx context.Context, in *proto.IsAuthorizedRequest) (*proto.IsAuthorizedResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	var (
		keyID     string
		keySecret string
		// delegated requests skip the secret check: the caller never
		// has the secret. Trust is established at the in-process
		// transport boundary, not in this RPC.
		delegated bool
	)
	switch {
	case in.GetKey() != "" && in.GetKeyId() != "":
		return nil, ErrAmbiguousAuth
	case in.GetKeyId() != "":
		keyID = in.GetKeyId()
		delegated = true
	default:
		var err error
		keyID, keySecret, err = httpmw.SplitAPIToken(in.GetKey())
		if err != nil {
			return nil, ErrInvalidKey
		}
	}

	// Key exists.
	key, err := s.store.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		s.logger.Warn(ctx, "failed to retrieve API key by id", slog.F("key_id", keyID), slog.Error(err))
		return nil, ErrUnknownKey
	}

	// Key has not expired.
	now := dbtime.Now()
	if key.ExpiresAt.Before(now) {
		return nil, ErrExpired
	}

	// Key secret matches (skipped for delegated callers).
	if !delegated && !apikey.ValidateHash(key.HashedSecret, keySecret) {
		return nil, ErrInvalidKey
	}

	// User exists.
	user, err := s.store.GetUserByID(ctx, key.UserID)
	if err != nil {
		s.logger.Warn(ctx, "failed to retrieve API key user", slog.F("key_id", keyID), slog.F("user_id", key.UserID), slog.Error(err))
		return nil, ErrUnknownUser
	}

	// User is active, not deleted, and not a system user.
	if user.Deleted {
		return nil, ErrDeletedUser
	}
	if user.Status != database.UserStatusActive {
		return nil, ErrInactiveUser
	}
	if user.IsSystem {
		return nil, ErrSystemUser
	}

	return &proto.IsAuthorizedResponse{
		OwnerId:  key.UserID.String(),
		ApiKeyId: key.ID,
		Username: user.Username,
	}, nil
}

// IsBudgetExceeded reports whether the user's AI spend has reached their
// effective limit over [PeriodStart, now].
func (s *Server) IsBudgetExceeded(ctx context.Context, in *proto.IsBudgetExceededRequest) (*proto.IsBudgetExceededResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	userID, err := uuid.Parse(in.GetUserId())
	if err != nil {
		return nil, xerrors.Errorf("invalid user_id %q: %w", in.GetUserId(), err)
	}
	// An unset PeriodStart deserializes to time.Unix(0, 0), which would
	// incorrectly aggregate the user's lifetime spend against a period budget.
	if in.PeriodStart == nil {
		return nil, xerrors.New("period_start is required")
	}
	periodStart := in.GetPeriodStart().AsTime()

	userBudget, err := s.checkUserAIBudget(ctx, userID, periodStart)
	if err != nil {
		return nil, err
	}
	return &proto.IsBudgetExceededResponse{
		Exceeded:         userBudget.Exceeded,
		SpendLimitMicros: userBudget.SpendLimitMicros,
	}, nil
}

// userAIBudget is a snapshot of a user's AI budget status. SpendLimitMicros
// is nil when no budget is configured for the user (unlimited).
type userAIBudget struct {
	Exceeded         bool
	SpendLimitMicros *int64
}

// checkUserAIBudget evaluates the user's AI budget status aggregated over
// [periodStart, now].
//
// Note: there is a potential race condition where two concurrent requests
// from the same user can both pass the check if processed in parallel,
// allowing brief overage. This is acceptable because:
//   - Cost is only known after the LLM API returns.
//   - Overage is bounded by request cost × concurrency; once the accumulated
//     spend crosses the limit, subsequent requests are blocked.
//   - Cost accounting is advisory, not strict. The goal is to prevent
//     overages, not build an accounting system.
//   - Fail-open is acceptable for this case.
func (s *Server) checkUserAIBudget(ctx context.Context, userID uuid.UUID, periodStart time.Time) (userAIBudget, error) {
	effectiveBudget, ok, err := budget.ResolveUserAIBudget(ctx, s.store, userID, s.budgetPolicy)
	if err != nil {
		return userAIBudget{}, xerrors.Errorf("resolve effective AI budget for user %q with budget policy %q: %w", userID, s.budgetPolicy, err)
	}
	if !ok {
		// No budget configured for the user; return zero-valued status.
		return userAIBudget{}, nil
	}

	spend, err := s.store.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
		UserID:           userID,
		EffectiveGroupID: effectiveBudget.GroupID,
		PeriodStart:      periodStart,
	})
	if err != nil {
		return userAIBudget{}, xerrors.Errorf("get user AI spend for user %q in group %q: %w", userID, effectiveBudget.GroupID, err)
	}

	exceeded := spend.SpendMicros >= effectiveBudget.SpendLimitMicros

	logger := s.logger.With(
		slog.F("user_id", userID),
		slog.F("effective_group_id", effectiveBudget.GroupID),
		slog.F("period_start", periodStart),
		slog.F("current_spend_micros", spend.SpendMicros),
		slog.F("spend_limit_micros", effectiveBudget.SpendLimitMicros),
		slog.F("exceeded", exceeded),
	)
	logger.Debug(ctx, "user AI spend status")
	if exceeded {
		logger.Warn(ctx, "user AI budget exceeded")
	}

	return userAIBudget{
		Exceeded:         exceeded,
		SpendLimitMicros: ptr.Ref(effectiveBudget.SpendLimitMicros),
	}, nil
}

// GetAIProviders returns the full AI provider set (enabled and disabled) from
// the database, which is the single source of truth seeded from coderd's
// environment. Embedded and standalone AI Gateway daemons call this over DRPC
// to build their provider pool instead of reading the database directly.
//
// The handler reads under a read-only transaction that first acquires
// LockIDAIProvidersEnvSeed, so it blocks until any in-flight env seed commits
// or rolls back. This guarantees the response is never a partial, mid-seed
// snapshot.
//
// Keys are populated only for enabled providers; disabled providers never call
// upstream, so their secrets are withheld.
//
// SECURITY: the response carries plaintext API keys and Bedrock credentials.
// Do not log the response struct.
func (s *Server) GetAIProviders(ctx context.Context, _ *proto.GetAIProvidersRequest) (*proto.GetAIProvidersResponse, error) {
	//nolint:gocritic // AIBridged has a minimal permission set scoped to AI Bridge queries.
	ctx = dbauthz.AsAIBridged(ctx)

	var (
		rows           []database.AIProvider
		keysByProvider map[uuid.UUID][]database.AIProviderKey
	)
	// Wrap both reads in a read-only transaction so the provider list and the
	// key list are consistent with each other, and so the seed lock is held
	// for the duration of the reads.
	err := s.store.InTx(func(tx database.Store) error {
		// Block on any in-flight seed transaction holding the advisory lock so
		// the response reflects a fully-seeded snapshot.
		if err := tx.AcquireLock(ctx, database.LockIDAIProvidersEnvSeed); err != nil {
			return xerrors.Errorf("acquire ai providers env seed lock: %w", err)
		}

		var err error
		rows, err = tx.GetAIProviders(ctx, database.GetAIProvidersParams{IncludeDisabled: true})
		if err != nil {
			return xerrors.Errorf("get ai providers: %w", err)
		}

		// Load keys only for enabled providers to avoid materializing secrets
		// for disabled rows.
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			if !row.Enabled {
				continue
			}
			ids = append(ids, row.ID)
		}
		keysByProvider = make(map[uuid.UUID][]database.AIProviderKey, len(ids))
		if len(ids) == 0 {
			return nil
		}
		keyRows, err := tx.GetAIProviderKeysByProviderIDs(ctx, ids)
		if err != nil {
			return xerrors.Errorf("get ai provider keys: %w", err)
		}
		for _, k := range keyRows {
			keysByProvider[k.ProviderID] = append(keysByProvider[k.ProviderID], k)
		}
		return nil
	}, &database.TxOptions{ReadOnly: true, TxIdentifier: "get_ai_providers"})
	if err != nil {
		return nil, err
	}

	providers := make([]*proto.AIProvider, 0, len(rows))
	for _, row := range rows {
		p, err := aiProviderToProto(row, keysByProvider[row.ID])
		if err != nil {
			// Skip the offending row rather than failing the whole fetch:
			// one row with a corrupt settings blob must not break provider
			// configuration for every gateway, which would otherwise loop
			// forever on the empty pool.
			s.logger.Error(ctx, "skipping ai provider with invalid settings; it will be absent from the gateway pool",
				slog.F("provider_id", row.ID),
				slog.F("provider_name", row.Name),
				slog.F("provider_type", string(row.Type)),
				slog.Error(err),
			)
			continue
		}
		providers = append(providers, p)
	}

	return &proto.GetAIProvidersResponse{Providers: providers}, nil
}

// Deprecated: Injected MCP in AI Bridge is deprecated and will be removed in a future release.
func getCoderMCPServerConfig(experiments codersdk.Experiments, accessURL string) (*proto.MCPServerConfig, error) {
	// Both the MCP & OAuth2 experiments are currently required in order to use our
	// internal MCP server.
	if !experiments.Enabled(codersdk.ExperimentMCPServerHTTP) {
		return nil, xerrors.Errorf("%q experiment not enabled", codersdk.ExperimentMCPServerHTTP)
	}
	if !experiments.Enabled(codersdk.ExperimentOAuth2) {
		return nil, xerrors.Errorf("%q experiment not enabled", codersdk.ExperimentOAuth2)
	}

	u, err := url.JoinPath(accessURL, codermcp.MCPEndpoint)
	if err != nil {
		return nil, xerrors.Errorf("build MCP URL with %q: %w", accessURL, err)
	}

	return &proto.MCPServerConfig{
		Id:  aibridged.InternalMCPServerID,
		Url: u,
	}, nil
}

// credentialKindOrDefault converts the proto credential kind string to
// the database enum, defaulting to "centralized" when the value is
// empty or not a valid enum member.
func credentialKindOrDefault(kind string) database.CredentialKind {
	ck := database.CredentialKind(kind)
	if !ck.Valid() {
		return database.CredentialKindCentralized
	}
	return ck
}

func metadataToMap(in map[string]*anypb.Any) map[string]any {
	meta := make(map[string]any, len(in))
	for k, v := range in {
		if v == nil {
			continue
		}
		var sv structpb.Value
		if err := v.UnmarshalTo(&sv); err == nil {
			meta[k] = sv.AsInterface()
		}
	}
	return meta
}

// parseOptionalUUID converts an optional proto string to uuid.NullUUID.
// Returns a zero NullUUID if s is nil. If s is non-nil but not a valid UUID, it
// returns a zero NullUUID along with the parse error so the caller can decide
// how to surface it.
func parseOptionalUUID(s *string) (uuid.NullUUID, error) {
	if s == nil {
		return uuid.NullUUID{}, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: id, Valid: true}, nil
}

// parseOptionalInt32 converts an optional proto int32 to sql.NullInt32.
func parseOptionalInt32(n *int32) sql.NullInt32 {
	if n == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *n, Valid: true}
}

// aiProviderToProto maps a single ai_providers row (and its keys, for enabled
// providers) to the proto representation served to AI Gateway daemons. Keys and
// Bedrock settings are only attached for enabled providers; disabled providers
// never call upstream so their secrets are withheld.
func aiProviderToProto(row database.AIProvider, keys []database.AIProviderKey) (*proto.AIProvider, error) {
	p := &proto.AIProvider{
		Name:    row.Name,
		Type:    string(row.Type),
		Enabled: row.Enabled,
		BaseUrl: row.BaseUrl,
	}
	// Disabled providers are rendered as stubs by the client and never call
	// upstream, so only the identity fields are returned; keys and settings
	// (including Bedrock credentials) are withheld.
	if !row.Enabled {
		return p, nil
	}

	p.Keys = make([]string, 0, len(keys))
	for _, k := range keys {
		p.Keys = append(p.Keys, k.APIKey)
	}

	settings, err := db2sdk.AIProviderSettings(row.Settings)
	if err != nil {
		return nil, xerrors.Errorf("decode settings: %w", err)
	}
	if settings.Bedrock != nil {
		p.Bedrock = &proto.AIProviderKindBedrock{
			Region:          settings.Bedrock.Region,
			AccessKey:       ptr.NilToEmpty(settings.Bedrock.AccessKey),
			AccessKeySecret: ptr.NilToEmpty(settings.Bedrock.AccessKeySecret),
			Model:           settings.Bedrock.Model,
			SmallFastModel:  settings.Bedrock.SmallFastModel,
			RoleArn:         settings.Bedrock.RoleARN,
			ExternalId:      settings.Bedrock.ExternalID,
		}
	}

	return p, nil
}
