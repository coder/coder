package appearance

import (
	"context"
	"fmt"
	"strings"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
)

type Fetcher interface {
	Fetch(ctx context.Context) (codersdk.AppearanceConfig, error)
}

func DefaultSupportLinks(docsURL string) []codersdk.LinkConfig {
	version := buildinfo.Version()
	if docsURL == "" {
		docsURL = "https://coder.com/docs/@" + strings.Split(version, "-")[0]
	}
	buildInfo := fmt.Sprintf("Version: [`%s`](%s)", version, buildinfo.ExternalURL())

	return []codersdk.LinkConfig{
		{
			Name:   "Documentation",
			Target: docsURL,
			Icon:   "docs",
		},
		{
			Name:   "Report a bug",
			Target: "https://github.com/coder/coder/issues/new?labels=needs+grooming&body=" + buildInfo,
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
