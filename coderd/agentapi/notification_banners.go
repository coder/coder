package agentapi

import (
	"context"
	"sync/atomic"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type NotificationBannerAPI struct {
	appearanceFetcher *atomic.Pointer[appearance.Fetcher]
}

// Deprecated: GetServiceBanner has been deprecated in favor of GetNotificationBanners.
func (a *NotificationBannerAPI) GetServiceBanner(ctx context.Context, _ *proto.GetServiceBannerRequest) (*proto.ServiceBanner, error) {
	cfg, err := (*a.appearanceFetcher.Load()).Fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("fetch appearance: %w", err)
	}
	return agentsdk.ProtoFromServiceBanner(cfg.ServiceBanner), nil
}

func (a *NotificationBannerAPI) GetNotificationBanners(ctx context.Context, _ *proto.GetNotificationBannersRequest) (*proto.GetNotificationBannersResponse, error) {
	cfg, err := (*a.appearanceFetcher.Load()).Fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("fetch appearance: %w", err)
	}
	banners := make([]*proto.BannerConfig, 0, len(cfg.NotificationBanners))
	for _, banner := range cfg.NotificationBanners {
		banners = append(banners, agentsdk.ProtoFromBannerConfig(banner))
	}
	return &proto.GetNotificationBannersResponse{
		NotificationBanners: banners,
	}, nil
}
