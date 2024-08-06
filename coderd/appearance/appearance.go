package appearance

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

type Fetcher interface {
	Fetch(ctx context.Context) (codersdk.AppearanceConfig, error)
}

func DefaultSupportLinks(docsURL string) []codersdk.LinkConfig {
	if docsURL == "" {
		docsURL = "https://coder.com/docs/{CODER_VERSION}"
	}

	docsLink := codersdk.LinkConfig{
		Name:   "Documentation",
		Target: docsURL,
		Icon:   "docs",
	}

	defaultSupportLinks := []codersdk.LinkConfig{
		{
			Name:   "Report a bug",
			Target: "https://github.com/coder/coder/issues/new?labels=needs+grooming&body={CODER_BUILD_INFO}",
			Icon:   "bug",
		},
		{
			Name:   "Join the Coder Discord",
			Target: "https://coder.com/chat?utm_source=coder&utm_medium=coder&utm_campaign=server-footer",
			Icon:   "chat",
		},
		{
			Name:   "Star the Repo",
			Target: "https://github.com/coder/coder",
			Icon:   "star",
		},
	}

	return append([]codersdk.LinkConfig{docsLink}, defaultSupportLinks...)
}

type AGPLFetcher struct {
	docsURL string
}

func (f AGPLFetcher) Fetch(context.Context) (codersdk.AppearanceConfig, error) {
	return codersdk.AppearanceConfig{
		AnnouncementBanners: []codersdk.BannerConfig{},
		SupportLinks:        DefaultSupportLinks(f.docsURL),
	}, nil
}

func NewDefaultFetcher(docsURL string) Fetcher {
	return &AGPLFetcher{
		docsURL: docsURL,
	}
}
