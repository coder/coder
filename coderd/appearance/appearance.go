package appearance

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

type Fetcher interface {
	Fetch(ctx context.Context) (codersdk.AppearanceConfig, error)
}

// Option overlays process-local, deployment-derived values onto a
// resolved AppearanceConfig. Options run at fetch time, so they may read
// values that change after startup (such as atomics). They let callers
// surface dynamic fields without adding parameters to fetcher
// constructors, and keep this package agnostic of those fields.
type Option func(*codersdk.AppearanceConfig)

type AGPLFetcher struct {
	docsURL string
	options []Option
}

func (f AGPLFetcher) Fetch(context.Context) (codersdk.AppearanceConfig, error) {
	cfg := codersdk.AppearanceConfig{
		AnnouncementBanners: []codersdk.BannerConfig{},
		SupportLinks:        codersdk.DefaultSupportLinks(f.docsURL),
		DocsURL:             f.docsURL,
	}
	for _, opt := range f.options {
		opt(&cfg)
	}
	return cfg, nil
}

func NewDefaultFetcher(docsURL string, opts ...Option) Fetcher {
	if docsURL == "" {
		docsURL = codersdk.DefaultDocsURL()
	}
	return &AGPLFetcher{
		docsURL: docsURL,
		options: opts,
	}
}
