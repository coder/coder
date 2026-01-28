package aibridgedserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpmw"
	codermcp "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/aibridged"
	"github.com/coder/coder/v2/enterprise/aibridged/proto"
)

var (
	ErrExpiredOrInvalidOAuthToken = xerrors.New("expired or invalid OAuth2 token")
	ErrNoMCPConfigFound           = xerrors.New("no MCP config found")

	// These errors are returned by IsAuthorized. Since they're just returned as
	// a generic dRPC error, it's difficult to tell them apart without string
	// matching.
	// TODO: return these errors to the client in a more structured/comparable
	//       way.
	ErrInvalidKey  = xerrors.New("invalid key")
	ErrUnknownKey  = xerrors.New("unknown key")
	ErrExpired     = xerrors.New("expired")
	ErrUnknownUser = xerrors.New("unknown user")
	ErrDeletedUser = xerrors.New("deleted user")
	ErrSystemUser  = xerrors.New("system user")

	ErrNoExternalAuthLinkFound = xerrors.New("no external auth link found")
)

const (
	InterceptionLogMarker = "interception log"
)

var _ aibridged.DRPCServer = &Server{}

type store interface {
	// Recorder-related queries.
	InsertAIBridgeInterception(ctx context.Context, arg database.InsertAIBridgeInterceptionParams) (database.AIBridgeInterception, error)
	InsertAIBridgeTokenUsage(ctx context.Context, arg database.InsertAIBridgeTokenUsageParams) (database.AIBridgeTokenUsage, error)
	InsertAIBridgeUserPrompt(ctx context.Context, arg database.InsertAIBridgeUserPromptParams) (database.AIBridgeUserPrompt, error)
	InsertAIBridgeToolUsage(ctx context.Context, arg database.InsertAIBridgeToolUsageParams) (database.AIBridgeToolUsage, error)
	UpdateAIBridgeInterceptionEnded(ctx context.Context, intcID database.UpdateAIBridgeInterceptionEndedParams) (database.AIBridgeInterception, error)

	// MCPConfigurator-related queries.
	GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error)

	// Authorizer-related queries.
	GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (database.User, error)
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
}

func NewServer(lifecycleCtx context.Context, store store, logger slog.Logger, accessURL string,
	bridgeCfg codersdk.AIBridgeConfig, externalAuthConfigs []*externalauth.Config, experiments codersdk.Experiments,
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
	}

	if bridgeCfg.InjectCoderMCPTools {
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

	if s.structuredLogging {
		s.logger.Info(ctx, InterceptionLogMarker,
			slog.F("record_type", "interception_start"),
			slog.F("interception_id", intcID.String()),
			slog.F("initiator_id", initID.String()),
			slog.F("api_key_id", in.ApiKeyId),
			slog.F("provider", in.Provider),
			slog.F("model", in.Model),
			slog.F("started_at", in.StartedAt.AsTime()),
			slog.F("metadata", metadata),
		)
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn(ctx, "failed to marshal aibridge metadata from proto to JSON", slog.F("metadata", in), slog.Error(err))
	}

	_, err = s.store.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
		ID:          intcID,
		APIKeyID:    sql.NullString{String: in.ApiKeyId, Valid: true},
		InitiatorID: initID,
		Provider:    in.Provider,
		Model:       in.Model,
		Metadata:    out,
		StartedAt:   in.StartedAt.AsTime(),
	})
	if err != nil {
		return nil, xerrors.Errorf("start interception: %w", err)
	}

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
		ID:      intcID,
		EndedAt: in.EndedAt.AsTime(),
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
			slog.F("created_at", in.GetCreatedAt().AsTime()),
			slog.F("metadata", metadata),
		)
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		s.logger.Warn(ctx, "failed to marshal aibridge metadata from proto to JSON", slog.F("metadata", in), slog.Error(err))
	}

	_, err = s.store.InsertAIBridgeTokenUsage(ctx, database.InsertAIBridgeTokenUsageParams{
		ID:                 uuid.New(),
		InterceptionID:     intcID,
		ProviderResponseID: in.GetMsgId(),
		InputTokens:        in.GetInputTokens(),
		OutputTokens:       in.GetOutputTokens(),
		Metadata:           out,
		CreatedAt:          in.GetCreatedAt().AsTime(),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert token usage: %w", err)
	}

	return &proto.RecordTokenUsageResponse{}, nil
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

	// Key matches expected format.
	keyID, keySecret, err := httpmw.SplitAPIToken(in.GetKey())
	if err != nil {
		return nil, ErrInvalidKey
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

	// Key secret matches.
	if !apikey.ValidateHash(key.HashedSecret, keySecret) {
		return nil, ErrInvalidKey
	}

	// User exists.
	user, err := s.store.GetUserByID(ctx, key.UserID)
	if err != nil {
		s.logger.Warn(ctx, "failed to retrieve API key user", slog.F("key_id", keyID), slog.F("user_id", key.UserID), slog.Error(err))
		return nil, ErrUnknownUser
	}

	// User is not deleted or a system user.
	if user.Deleted {
		return nil, ErrDeletedUser
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
