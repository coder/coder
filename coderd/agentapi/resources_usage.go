package agentapi

import (
	"context"

	"github.com/coder/coder/v2/agent/proto"
	// "github.com/coder/coder/v2/coderd/appearance"
)

type ResourcesUsageAPI struct {
	// appearanceFetcher *atomic.Pointer[appearance.Fetcher]
}

func (a *ResourcesUsageAPI) PushResourcesUsage(ctx context.Context, _ *proto.PushResourcesUsageRequest) (*proto.PushResourcesUsageResponse, error) {
	// cfg, err := (*a.appearanceFetcher.Load()).Fetch(ctx)
	// if err != nil {
	// 	return nil, xerrors.Errorf("fetch appearance: %w", err)
	// }
	// banners := make([]*proto.BannerConfig, 0, len(cfg.AnnouncementBanners))
	// for _, banner := range cfg.AnnouncementBanners {
	// 	banners = append(banners, agentsdk.ProtoFromBannerConfig(banner))
	// }
	// return &proto.GetAnnouncementBannersResponse{
	// 	AnnouncementBanners: banners,
	// }, nil
	return nil, nil
}
