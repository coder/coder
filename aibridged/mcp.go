package aibridged

import (
	"context"
	"net/url"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/externalauth"
)

type MCPServerConfig struct {
	Name, URL, AccessToken string
	ValidateFn             func(ctx context.Context) (bool, error)
	RefreshFn              func(ctx context.Context) (bool, error)
}

type MCPConfigurator interface {
	GetMCPConfigs(ctx context.Context, sessionKey string, clientFn func() (DRPCClient, error), userID uuid.UUID) ([]*MCPServerConfig, error)
}

type DRPCMCPConfigurator struct {
	accessURL           string
	logger              slog.Logger
	externalAuthConfigs []*externalauth.Config
}

func NewDRPCMCPConfigurator(accessURL string, logger slog.Logger, externalAuthConfigs []*externalauth.Config) *DRPCMCPConfigurator {
	return &DRPCMCPConfigurator{accessURL: accessURL, logger: logger, externalAuthConfigs: externalAuthConfigs}
}

func (m *DRPCMCPConfigurator) GetMCPConfigs(ctx context.Context, sessionKey string, clientFn func() (DRPCClient, error), userID uuid.UUID) ([]*MCPServerConfig, error) {
	var out []*MCPServerConfig
	var merr multierror.Error

	coder, err := m.getCoderMCPServerConfig(sessionKey)
	if err != nil {
		merr.Errors = append(merr.Errors, xerrors.Errorf("get coder MCP server config: %w", err))
	} else {
		out = append(out, coder)
	}

	others, err := m.getExternalAuthMCPServerConfigs(ctx, m.logger, clientFn, userID)
	if err != nil {
		merr.Errors = append(merr.Errors, xerrors.Errorf("get external auth MCP server config: %w", err))
	} else {
		out = append(out, others...)
	}

	return out, merr.ErrorOrNil()
}

func (m *DRPCMCPConfigurator) getCoderMCPServerConfig(sessionKey string) (*MCPServerConfig, error) {
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

func (m *DRPCMCPConfigurator) getExternalAuthMCPServerConfigs(ctx context.Context, logger slog.Logger, clientFn func() (DRPCClient, error), userID uuid.UUID) ([]*MCPServerConfig, error) {
	client, err := clientFn()
	if err != nil {
		return nil, xerrors.Errorf("acquire client: %w", err)
	}

	externalAuthLinks, err := client.GetExternalAuthLinks(ctx, &proto.GetExternalAuthLinksRequest{UserId: userID.String()})
	if err != nil {
		return nil, xerrors.Errorf("load external auth links: %w", err)
	}

	if len(externalAuthLinks.Links) == 0 {
		return nil, nil
	}

	cfgs := make([]*MCPServerConfig, 0, len(externalAuthLinks.Links))

	for _, link := range externalAuthLinks.Links {
		var externalAuthConfig *externalauth.Config
		for _, eac := range m.externalAuthConfigs {
			if eac.ID == link.GetProviderId() {
				externalAuthConfig = eac
				break
			}
		}

		if externalAuthConfig == nil {
			logger.Warn(ctx, "failed to find external auth config matching known external auth link", slog.F("id", link.GetProviderId()))
			continue
		}

		cfgs = append(cfgs, &MCPServerConfig{
			Name:        link.GetProviderId(),
			URL:         externalAuthConfig.MCPURL,
			AccessToken: link.GetOauthAccessToken(),
			ValidateFn: func(ctx context.Context) (bool, error) {
				valid, _, err := externalAuthConfig.ValidateToken(ctx, &oauth2.Token{
					AccessToken:  link.GetOauthAccessToken(),
					RefreshToken: link.GetOauthRefreshToken(),
					Expiry:       link.GetExpiresAt().AsTime(),
				})
				if err != nil {
					return false, xerrors.Errorf("validate token for %q MCP init: %w", link.GetProviderId(), err)
				}
				return valid, nil
			},
			// TODO: implement RefreshFn.
		})
	}

	return cfgs, nil
}
