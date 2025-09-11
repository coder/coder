package aibridgedserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"sync"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpmw"
)

var _ proto.DRPCRecorderServer = &Server{}
var _ proto.DRPCMCPConfiguratorServer = &Server{}
var _ proto.DRPCAuthenticatorServer = &Server{}

type store interface {
	// Recorder-related queries.
	InsertAIBridgeInterception(ctx context.Context, arg database.InsertAIBridgeInterceptionParams) (database.AIBridgeInterception, error)
	InsertAIBridgeTokenUsage(ctx context.Context, arg database.InsertAIBridgeTokenUsageParams) error
	InsertAIBridgeUserPrompt(ctx context.Context, arg database.InsertAIBridgeUserPromptParams) error
	InsertAIBridgeToolUsage(ctx context.Context, arg database.InsertAIBridgeToolUsageParams) error

	// MCPConfigurator-related queries.
	GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error)

	// Authenticator-related queries.
	GetAPIKeyByID(ctx context.Context, id string) (database.APIKey, error)
}

type Server struct {
	// lifecycleCtx must be tied to the API server's lifecycle
	// as when the API server shuts down, we want to cancel any
	// long-running operations.
	lifecycleCtx        context.Context
	store               store
	logger              slog.Logger
	accessURL           string
	externalAuthConfigs map[string]*externalauth.Config
}

func NewServer(lifecycleCtx context.Context, store store, logger slog.Logger, accessURL string, externalAuthConfigs []*externalauth.Config) (*Server, error) {
	eac := make(map[string]*externalauth.Config, len(externalAuthConfigs))

	for _, cfg := range externalAuthConfigs {
		eac[cfg.ID] = cfg
	}

	return &Server{
		lifecycleCtx:        lifecycleCtx,
		store:               store,
		logger:              logger.Named("aibridgedserver"),
		accessURL:           accessURL,
		externalAuthConfigs: eac,
	}, nil
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

	_, err = s.store.InsertAIBridgeInterception(ctx, database.InsertAIBridgeInterceptionParams{
		ID:          intcID,
		InitiatorID: initID,
		Provider:    in.Provider,
		Model:       in.Model,
		StartedAt:   in.StartedAt.AsTime(),
	})
	if err != nil {
		return nil, xerrors.Errorf("start interception: %w", err)
	}

	return &proto.RecordInterceptionResponse{}, nil
}

func (s *Server) RecordTokenUsage(ctx context.Context, in *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	intcID, err := uuid.Parse(in.GetInterceptionId())
	if err != nil {
		return nil, xerrors.Errorf("failed to parse interception_id %q: %w", in.GetInterceptionId(), err)
	}

	err = s.store.InsertAIBridgeTokenUsage(ctx, database.InsertAIBridgeTokenUsageParams{
		ID:                 uuid.New(),
		InterceptionID:     intcID,
		ProviderResponseID: in.GetMsgId(),
		InputTokens:        in.GetInputTokens(),
		OutputTokens:       in.GetOutputTokens(),
		Metadata:           s.marshalMetadata(in.GetMetadata()),
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

	err = s.store.InsertAIBridgeUserPrompt(ctx, database.InsertAIBridgeUserPromptParams{
		ID:                 uuid.New(),
		InterceptionID:     intcID,
		ProviderResponseID: in.GetMsgId(),
		Prompt:             in.GetPrompt(),
		Metadata:           s.marshalMetadata(in.GetMetadata()),
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

	err = s.store.InsertAIBridgeToolUsage(ctx, database.InsertAIBridgeToolUsageParams{
		ID:                 uuid.New(),
		InterceptionID:     intcID,
		ProviderResponseID: in.GetMsgId(),
		ServerUrl:          sql.NullString{String: in.GetServerUrl(), Valid: in.ServerUrl != nil},
		Tool:               in.GetTool(),
		Input:              in.GetInput(),
		Injected:           in.GetInjected(),
		InvocationError:    sql.NullString{String: in.GetInvocationError(), Valid: in.GetInvocationError() != ""},
		Metadata:           s.marshalMetadata(in.GetMetadata()),
		CreatedAt:          in.GetCreatedAt().AsTime(),
	})
	if err != nil {
		return nil, xerrors.Errorf("insert tool usage: %w", err)
	}
	return &proto.RecordToolUsageResponse{}, nil
}

func (s *Server) marshalMetadata(in map[string]*anypb.Any) []byte {
	mdMap := make(map[string]any, len(in))
	for k, v := range in {
		if v == nil {
			continue
		}
		var sv structpb.Value
		if err := v.UnmarshalTo(&sv); err == nil {
			mdMap[k] = sv.AsInterface()
		}
	}
	out, err := json.Marshal(mdMap)
	if err != nil {
		s.logger.Warn(s.lifecycleCtx, "failed to marshal metadata", slog.Error(err))
	}
	return out
}

func (s *Server) GetMCPServerConfigs(ctx context.Context, _ *proto.GetMCPServerConfigsRequest) (*proto.GetMCPServerConfigsResponse, error) {
	cfgs := make([]*proto.MCPServerConfig, 0, len(s.externalAuthConfigs))
	for _, eac := range s.externalAuthConfigs {
		var allowlist, denylist string
		if eac.MCPToolAllowlistPattern != nil {
			allowlist = eac.MCPToolAllowlistPattern.String()
		}
		if eac.MCPToolDenylistPattern != nil {
			denylist = eac.MCPToolDenylistPattern.String()
		}

		cfgs = append(cfgs, &proto.MCPServerConfig{
			Id:            eac.ID,
			Url:           eac.MCPURL,
			ToolAllowlist: allowlist,
			ToolDenylist:  denylist,
		})
	}

	return &proto.GetMCPServerConfigsResponse{
		CoderMcpConfig:         s.getCoderMCPServerConfig(),
		ExternalAuthMcpConfigs: cfgs,
	}, nil
}

func (s *Server) getCoderMCPServerConfig() *proto.MCPServerConfig {
	u, _ := url.JoinPath(s.accessURL, "/api/experimental/mcp/http")
	return &proto.MCPServerConfig{
		Id:  "coder",
		Url: u,
	}
}

func (s *Server) GetMCPServerAccessTokensBatch(ctx context.Context, in *proto.GetMCPServerAccessTokensBatchRequest) (*proto.GetMCPServerAccessTokensBatchResponse, error) {
	if len(in.GetMcpServerConfigIds()) == 0 {
		return nil, nil
	}

	id, err := uuid.Parse(in.GetUserId())
	if err != nil {
		return nil, xerrors.Errorf("parse user_id: %w", err)
	}

	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)
	links, err := s.store.GetExternalAuthLinksByUserID(ctx, id)
	if err != nil {
		return nil, xerrors.Errorf("fetch external auth links: %w", err)
	}

	if len(links) == 0 {
		return nil, nil
	}

	var (
		wg   sync.WaitGroup
		errs error

		mu        sync.Mutex
		tokens    = make(map[string]string, len(in.GetMcpServerConfigIds()))
		tokenErrs = make(map[string]string, len(in.GetMcpServerConfigIds()))
	)

	for _, id := range in.GetMcpServerConfigIds() {
		eac, ok := s.externalAuthConfigs[id]
		if !ok {
			s.logger.Warn(ctx, "no MCP server found by given ID", slog.F("id", id))
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
				valid, _, err := eac.ValidateToken(ctx, link.OAuthToken())
				if !valid {
					// TODO: attempt refresh.
					s.logger.Warn(ctx, "invalid/expired access token, cannot auto-configure MCP", slog.F("provider", link.ProviderID), slog.Error(err))
				}

				mu.Lock()
				if err != nil {
					errs = multierror.Append(errs, err)
					tokenErrs[id] = err.Error()
				} else {
					tokens[id] = link.OAuthAccessToken
				}
				mu.Unlock()
			}()
		}
	}

	wg.Wait()
	return &proto.GetMCPServerAccessTokensBatchResponse{
		AccessTokens: tokens,
		Errors:       tokenErrs,
	}, errs
}

// AuthenticateKey authenticates a given session/API key and returns the user ID to which it belongs.
func (s *Server) AuthenticateKey(ctx context.Context, in *proto.AuthenticateKeyRequest) (*proto.AuthenticateKeyResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	id, _, err := httpmw.SplitAPIToken(in.GetKey())
	if err != nil {
		return nil, xerrors.Errorf("split API token: %w", err)
	}

	key, err := s.store.GetAPIKeyByID(ctx, id)
	if err != nil {
		s.logger.Warn(ctx, "failed to retrieve API key by id", slog.F("id", id), slog.Error(err))
		return nil, xerrors.Errorf("get key by id: %w", err)
	}

	return &proto.AuthenticateKeyResponse{
		OwnerId: key.UserID.String(),
	}, nil
}
