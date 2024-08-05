package appearance

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

type Fetcher interface {
	Fetch(ctx context.Context) (codersdk.AppearanceConfig, error)
}

var defaultSupportLinks = []codersdk.LinkConfig{
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

func DefaultSupportLinks(docsUrl string) []codersdk.LinkConfig {
	if docsUrl == "" {
		docsUrl = "https://coder.com/docs/coder-oss"
	}

	docsLink := codersdk.LinkConfig{
		Name:   "Documentation",
		Target: docsUrl,
		Icon:   "docs",
	}

	return append([]codersdk.LinkConfig{docsLink}, defaultSupportLinks...)
}

type AGPLFetcher struct {
	docsUrl string
}

func (f AGPLFetcher) Fetch(context.Context) (codersdk.AppearanceConfig, error) {

	return codersdk.AppearanceConfig{
		AnnouncementBanners: []codersdk.BannerConfig{},
		SupportLinks:        DefaultSupportLinks(f.docsUrl),
	}, nil
}

func NewDefaultFetcher(docsUrl string) Fetcher {
	return &AGPLFetcher{
		docsUrl: docsUrl,
	}
}
