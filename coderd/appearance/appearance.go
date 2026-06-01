package appearance

import (
	"context"
	"sync/atomic"

	"github.com/coder/coder/v2/codersdk"
)

type Fetcher interface {
	Fetch(ctx context.Context) (codersdk.AppearanceConfig, error)
}

type AGPLFetcher struct {
	docsURL string
	// aiProvidersEnvDrift reports whether deprecated CODER_AIBRIDGE_* env
	// configuration is ineffective because it differs from the database.
	// It may be nil when no source is wired in.
	aiProvidersEnvDrift *atomic.Bool
}

func (f AGPLFetcher) Fetch(context.Context) (codersdk.AppearanceConfig, error) {
	return codersdk.AppearanceConfig{
		AnnouncementBanners:         []codersdk.BannerConfig{},
		SupportLinks:                codersdk.DefaultSupportLinks(f.docsURL),
		DocsURL:                     f.docsURL,
		AIProvidersEnvDriftDetected: f.aiProvidersEnvDrift != nil && f.aiProvidersEnvDrift.Load(),
	}, nil
}

func NewDefaultFetcher(docsURL string, aiProvidersEnvDrift *atomic.Bool) Fetcher {
	if docsURL == "" {
		docsURL = codersdk.DefaultDocsURL()
	}
	return &AGPLFetcher{
		docsURL:             docsURL,
		aiProvidersEnvDrift: aiProvidersEnvDrift,
	}
}
