package aibridged

import (
	"context"
	"net/url"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/externalauth"
)

type MCPServerConfig struct {
	Name, URL, AccessToken string
	ValidateFn             func(ctx context.Context) (bool, error)
	RefreshFn              func(ctx context.Context) (bool, error)
}

type store interface {
	GetExternalAuthLinksByUserID(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error)
}

type MCPConfigurator interface {
	GetMCPConfigs(ctx context.Context, sessionKey string, userID uuid.UUID) ([]*MCPServerConfig, error)
}

type StoreMCPConfigurator struct {
	accessURL           string
	store               store
	logger              slog.Logger
	externalAuthConfigs []*externalauth.Config
}

func NewStoreMCPConfigurator(accessURL string, store store, externalAuthConfigs []*externalauth.Config, logger slog.Logger) *StoreMCPConfigurator {
	return &StoreMCPConfigurator{accessURL: accessURL, store: store, logger: logger, externalAuthConfigs: externalAuthConfigs}
}

func (m *StoreMCPConfigurator) GetMCPConfigs(ctx context.Context, sessionKey string, userID uuid.UUID) ([]*MCPServerConfig, error) {
	var out []*MCPServerConfig
	var merr multierror.Error

	coder, err := m.getCoderMCPServerConfig(sessionKey)
	if err != nil {
		merr.Errors = append(merr.Errors, xerrors.Errorf("get coder MCP server config: %w", err))
	} else {
		out = append(out, coder)
	}

	others, err := m.getExternalAuthMCPServerConfigs(ctx, m.logger, m.externalAuthConfigs, userID)
	if err != nil {
		merr.Errors = append(merr.Errors, xerrors.Errorf("get external auth MCP server config: %w", err))
	} else {
		out = append(out, others...)
	}

	return out, merr.ErrorOrNil()
}

func (m *StoreMCPConfigurator) getCoderMCPServerConfig(sessionKey string) (*MCPServerConfig, error) {
	mcpURL, err := url.JoinPath(m.accessURL, "/api/experimental/mcp/http")
	if err != nil {
		return nil, xerrors.Errorf("build MCP URL: %w", err)
	}

	return &MCPServerConfig{
		Name:        "coder",
		URL:         mcpURL,
		AccessToken: sessionKey,
		ValidateFn: func(_ context.Context) (bool, error) {
			// No-op since request would not proceed if session key was invalid.
			return true, nil
		},
	}, nil
}

func (m *StoreMCPConfigurator) getExternalAuthMCPServerConfigs(ctx context.Context, logger slog.Logger, externalAuthConfigs []*externalauth.Config, userID uuid.UUID) ([]*MCPServerConfig, error) {
	externalAuthLinks, err := m.store.GetExternalAuthLinksByUserID(ctx, userID)
	if err != nil {
		return nil, xerrors.Errorf("load external auth links: %w", err)
	}

	if len(externalAuthLinks) == 0 {
		return nil, nil
	}

	cfgs := make([]*MCPServerConfig, 0, len(externalAuthLinks))

	for _, link := range externalAuthLinks {
		var externalAuthConfig *externalauth.Config
		for _, eac := range externalAuthConfigs {
			if eac.ID == link.ProviderID {
				externalAuthConfig = eac
				break
			}
		}

		if externalAuthConfig == nil {
			logger.Warn(ctx, "failed to find external auth config matching known external auth link", slog.F("id", link.ProviderID))
			continue
		}

		cfgs = append(cfgs, &MCPServerConfig{
			Name:        link.ProviderID,
			URL:         externalAuthConfig.MCPURL,
			AccessToken: link.OAuthAccessToken,
			ValidateFn: func(ctx context.Context) (bool, error) {
				valid, _, err := externalAuthConfig.ValidateToken(ctx, link.OAuthToken())
				if err != nil {
					return false, xerrors.Errorf("validate token for %q MCP init: %w", link.ProviderID, err)
				}
				return valid, nil
			},
			// TODO: implement RefreshFn.
		})
	}

	return cfgs, nil
}
