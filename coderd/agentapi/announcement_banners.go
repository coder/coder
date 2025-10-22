package agentapi

import (
	"context"
	"sync/atomic"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type AnnouncementBannerAPI struct {
	appearanceFetcher *atomic.Pointer[appearance.Fetcher]
}

// Deprecated: GetServiceBanner has been deprecated in favor of GetAnnouncementBanners.
func (a *AnnouncementBannerAPI) GetServiceBanner(ctx context.Context, _ *proto.GetServiceBannerRequest) (*proto.ServiceBanner, error) {
	cfg, err := (*a.appearanceFetcher.Load()).Fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("fetch appearance: %w", err)
	}
	return agentsdk.ProtoFromServiceBanner(cfg.ServiceBanner), nil
}

func (a *AnnouncementBannerAPI) GetAnnouncementBanners(ctx context.Context, _ *proto.GetAnnouncementBannersRequest) (*proto.GetAnnouncementBannersResponse, error) {
	cfg, err := (*a.appearanceFetcher.Load()).Fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("fetch appearance: %w", err)
	}
	banners := make([]*proto.BannerConfig, 0, len(cfg.AnnouncementBanners))
	for _, banner := range cfg.AnnouncementBanners {
		banners = append(banners, agentsdk.ProtoFromBannerConfig(banner))
	}
	return &proto.GetAnnouncementBannersResponse{
		AnnouncementBanners: banners,
	}, nil
}
