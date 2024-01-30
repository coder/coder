package agentapi

import (
	"context"
	"sync/atomic"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type ServiceBannerAPI struct {
	appearanceFetcher *atomic.Pointer[appearance.Fetcher]
}

func (a *ServiceBannerAPI) GetServiceBanner(ctx context.Context, _ *proto.GetServiceBannerRequest) (*proto.ServiceBanner, error) {
	cfg, err := (*a.appearanceFetcher.Load()).Fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("fetch appearance: %w", err)
	}
	return agentsdk.ProtoFromServiceBanner(cfg.ServiceBanner), nil
}
