package appearance

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

type Fetcher interface {
	Fetch(ctx context.Context) (codersdk.AppearanceConfig, error)
}

type AGPLFetcher struct {
	database database.Store
	docsURL  string
}

func (f AGPLFetcher) Fetch(ctx context.Context) (codersdk.AppearanceConfig, error) {
	codernautsEnabled, err := f.database.GetCodernautsEnabled(ctx)
	if err != nil {
		return codersdk.AppearanceConfig{}, xerrors.Errorf("get codernauts enabled: %w", err)
	}
	return codersdk.AppearanceConfig{
		AnnouncementBanners: []codersdk.BannerConfig{},
		SupportLinks:        codersdk.DefaultSupportLinks(f.docsURL),
		DocsURL:             f.docsURL,
		CodernautsEnabled:   codernautsEnabled,
	}, nil
}

func NewDefaultFetcher(db database.Store, docsURL string) Fetcher {
	if docsURL == "" {
		docsURL = codersdk.DefaultDocsURL()
	}
	return &AGPLFetcher{
		database: db,
		docsURL:  docsURL,
	}
}
