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
	// database may be nil when no store is available, in which case
	// runtime settings fall back to their zero values.
	database database.Store
	docsURL  string
}

func (f AGPLFetcher) Fetch(ctx context.Context) (codersdk.AppearanceConfig, error) {
	hideCodernauts := false
	if f.database != nil {
		var err error
		hideCodernauts, err = f.database.GetHideCodernauts(ctx)
		if err != nil {
			return codersdk.AppearanceConfig{}, xerrors.Errorf("get hide codernauts: %w", err)
		}
	}
	return codersdk.AppearanceConfig{
		AnnouncementBanners: []codersdk.BannerConfig{},
		SupportLinks:        codersdk.DefaultSupportLinks(f.docsURL),
		DocsURL:             f.docsURL,
		HideCodernauts:      hideCodernauts,
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
