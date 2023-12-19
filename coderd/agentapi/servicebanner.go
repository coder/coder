package agentapi

import (
	"context"
	"database/sql"
	"encoding/json"

	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

type ServiceBannerAPI struct {
	Database database.Store
}

func (a *ServiceBannerAPI) GetServiceBanner(ctx context.Context, _ *agentproto.GetServiceBannerRequest) (*agentproto.ServiceBanner, error) {
	serviceBannerJSON, err := a.Database.GetServiceBanner(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return nil, xerrors.Errorf("get service banner: %w", err)
	}

	var cfg codersdk.ServiceBannerConfig
	if serviceBannerJSON != "" {
		err = json.Unmarshal([]byte(serviceBannerJSON), &cfg)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal json: %w, raw: %s", err, serviceBannerJSON)
		}
	}

	return &agentproto.ServiceBanner{
		Enabled:         cfg.Enabled,
		Message:         cfg.Message,
		BackgroundColor: cfg.BackgroundColor,
	}, nil
}
