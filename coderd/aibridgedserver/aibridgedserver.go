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

	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/externalauth"
)

var _ aibridged.DRPCServer = &Server{}

type store interface {
	// Recorder-related queries.
	InsertAIBridgeInterception(ctx context.Context, arg database.InsertAIBridgeInterceptionParams) (database.AIBridgeInterception, error)
	InsertAIBridgeTokenUsage(ctx context.Context, arg database.InsertAIBridgeTokenUsageParams) error
	InsertAIBridgeUserPrompt(ctx context.Context, arg database.InsertAIBridgeUserPromptParams) error
	InsertAIBridgeToolUsage(ctx context.Context, arg database.InsertAIBridgeToolUsageParams) error

	// MCPConfigurator-related queries.
	GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error)
}

type Server struct {
	// lifecycleCtx must be tied to the API server's lifecycle
	// as when the API server shuts down, we want to cancel any
	// long-running operations.
	lifecycleCtx        context.Context
	store               store
	logger              slog.Logger
	accessURL           string
	externalAuthConfigs []*externalauth.Config
}

func NewServer(lifecycleCtx context.Context, store store, logger slog.Logger, accessURL string, externalAuthConfigs []*externalauth.Config) (*Server, error) {
	return &Server{
		lifecycleCtx:        lifecycleCtx,
		store:               store,
		logger:              logger.Named("aibridgedserver"),
		accessURL:           accessURL,
		externalAuthConfigs: externalAuthConfigs,
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

func (s *Server) RetrieveMCPServerConfigs(ctx context.Context, in *proto.RetrieveMCPServerConfigsRequest) (*proto.RetrieveMCPServerConfigsResponse, error) {
	//nolint:gocritic // AIBridged has specific authz rules.
	ctx = dbauthz.AsAIBridged(ctx)

	id, err := uuid.Parse(in.GetUserId())
	if err != nil {
		return nil, xerrors.Errorf("parse user_id: %w", err)
	}

	extAuthMCPCfgs, err := s.getExternalAuthLinkedMCPServerConfigs(ctx, id)
	// If any problems arise, we still want to return what we were able to successfully build.
	if err != nil {
		err = xerrors.Errorf("get external auth linked MCP server configs: %w", err)
	}

	return &proto.RetrieveMCPServerConfigsResponse{
		CoderMCPConfig:         s.getCoderMCPServerConfig(),
		ExternalAuthMCPConfigs: extAuthMCPCfgs,
	}, err
}

func (s *Server) getCoderMCPServerConfig() *proto.MCPServerConfig {
	u, _ := url.JoinPath(s.accessURL, "/api/experimental/mcp/http")
	return &proto.MCPServerConfig{
		Name:        "coder",
		Url:         u,
		AccessToken: "", // Deliberately left blank. Caller must set this instead of passing it over the wire.
	}
}

func (s *Server) getExternalAuthLinkedMCPServerConfigs(ctx context.Context, id uuid.UUID) ([]*proto.MCPServerConfig, error) {
	links, err := s.store.GetExternalAuthLinksByUserID(ctx, id)
	if err != nil {
		return nil, xerrors.Errorf("fetch external auth links for user_id: %w", err)
	}

	if len(links) == 0 {
		return nil, nil
	}

	var (
		errs error

		wg sync.WaitGroup

		cfgs   []*proto.MCPServerConfig
		cfgsMu sync.Mutex
	)

	for _, link := range links {
		var externalAuthCfg *externalauth.Config
		for _, eac := range s.externalAuthConfigs {
			if eac.ID == link.ProviderID {
				if eac.MCPURL == "" {
					s.logger.Debug(ctx, "external auth link found, but no MCP URL was configured", slog.F("provider", link.ProviderID))
					break
				}

				externalAuthCfg = eac
			}
		}

		if externalAuthCfg == nil {
			continue
		}

		// Validate all configured External Auth links concurrently.
		wg.Add(1)
		go func() {
			defer wg.Done()

			// TODO: timeout.
			valid, _, err := externalAuthCfg.ValidateToken(ctx, link.OAuthToken())
			if !valid {
				// TODO: attempt refresh.
				s.logger.Warn(ctx, "invalid/expired access token, cannot auto-configure MCP", slog.F("provider", link.ProviderID), slog.Error(err))

				if err != nil {
					errs = multierror.Append(err, errs)
				}
				return
			}

			cfgsMu.Lock()
			cfgs = append(cfgs, &proto.MCPServerConfig{
				Name:        link.ProviderID,
				Url:         externalAuthCfg.MCPURL,
				AccessToken: link.OAuthAccessToken,
			})
			cfgsMu.Unlock()
		}()
	}

	wg.Wait()
	return cfgs, errs
}
