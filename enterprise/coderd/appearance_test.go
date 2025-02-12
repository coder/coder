package coderd_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestCustomLogoAndCompanyName(t *testing.T) {
	t.Parallel()

	// Prepare enterprise deployment
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
	coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureAppearance: 1,
		},
	})

	anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	// Update logo and application name
	uac := codersdk.UpdateAppearanceConfig{
		ApplicationName: "ACME Ltd",
		LogoURL:         "http://logo-url/file.png",
	}

	err := adminClient.UpdateAppearance(ctx, uac)
	require.NoError(t, err)

	// Verify update
	got, err := anotherClient.Appearance(ctx)
	require.NoError(t, err)

	require.Equal(t, uac.ApplicationName, got.ApplicationName)
	require.Equal(t, uac.LogoURL, got.LogoURL)
}

func TestAnnouncementBanners(t *testing.T) {
	t.Parallel()

	t.Run("User", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		basicUserClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

		// Without a license, there should be no banners.
		sb, err := basicUserClient.Appearance(ctx)
		require.NoError(t, err)
		require.Empty(t, sb.AnnouncementBanners)

		coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAppearance: 1,
			},
		})

		// Default state
		sb, err = basicUserClient.Appearance(ctx)
		require.NoError(t, err)
		require.Empty(t, sb.AnnouncementBanners)

		// Regular user should be unable to set the banner
		uac := codersdk.UpdateAppearanceConfig{
			AnnouncementBanners: []codersdk.BannerConfig{{Enabled: true}},
		}
		err = basicUserClient.UpdateAppearance(ctx, uac)
		require.Error(t, err)
		var sdkError *codersdk.Error
		require.True(t, errors.As(err, &sdkError))
		require.ErrorAs(t, err, &sdkError)
		require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

		// But an admin can
		wantBanner := codersdk.UpdateAppearanceConfig{
			AnnouncementBanners: []codersdk.BannerConfig{{
				Enabled:         true,
				Message:         "The beep-bop will be boop-beeped on Saturday at 12AM PST.",
				BackgroundColor: "#00FF00",
			}},
		}
		err = adminClient.UpdateAppearance(ctx, wantBanner)
		require.NoError(t, err)
		gotBanner, err := adminClient.Appearance(ctx) //nolint:gocritic // we should assert at least once that the owner can get the banner
		require.NoError(t, err)
		require.Equal(t, wantBanner.AnnouncementBanners, gotBanner.AnnouncementBanners)

		// But even an admin can't give a bad color
		wantBanner.AnnouncementBanners[0].BackgroundColor = "#bad color"
		err = adminClient.UpdateAppearance(ctx, wantBanner)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Invalid color format")
		require.Contains(t, sdkErr.Detail, "expected # prefix and 6 characters")
	})

	t.Run("Agent", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		store, ps := dbtestutil.NewDB(t)
		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: store,
				Pubsub:   ps,
			},
			DontAddLicense: true,
		})
		lic := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAppearance: 1,
			},
		})
		cfg := codersdk.UpdateAppearanceConfig{
			AnnouncementBanners: []codersdk.BannerConfig{{
				Enabled:         true,
				Message:         "The beep-bop will be boop-beeped on Saturday at 12AM PST.",
				BackgroundColor: "#00FF00",
			}},
		}
		err := client.UpdateAppearance(ctx, cfg)
		require.NoError(t, err)

		r := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)
		banners := requireGetAnnouncementBanners(ctx, t, agentClient)
		require.Equal(t, cfg.AnnouncementBanners, banners)

		// Create an AGPL Coderd against the same database
		agplClient := coderdtest.New(t, &coderdtest.Options{Database: store, Pubsub: ps})
		agplAgentClient := agentsdk.New(agplClient.URL)
		agplAgentClient.SetSessionToken(r.AgentToken)
		banners = requireGetAnnouncementBanners(ctx, t, agplAgentClient)
		require.Equal(t, []codersdk.BannerConfig{}, banners)

		// No license means no banner.
		err = client.DeleteLicense(ctx, lic.ID)
		require.NoError(t, err)
		banners = requireGetAnnouncementBanners(ctx, t, agentClient)
		require.Equal(t, []codersdk.BannerConfig{}, banners)
	})
}

func requireGetAnnouncementBanners(ctx context.Context, t *testing.T, client *agentsdk.Client) []codersdk.BannerConfig {
	cc, err := client.ConnectRPC(ctx)
	require.NoError(t, err)
	defer func() {
		_ = cc.Close()
	}()
	aAPI := proto.NewDRPCAgentClient(cc)
	bannersProto, err := aAPI.GetAnnouncementBanners(ctx, &proto.GetAnnouncementBannersRequest{})
	require.NoError(t, err)
	banners := make([]codersdk.BannerConfig, 0, len(bannersProto.AnnouncementBanners))
	for _, bannerProto := range bannersProto.AnnouncementBanners {
		banners = append(banners, agentsdk.BannerConfigFromProto(bannerProto))
	}
	return banners
}

func TestCustomSupportLinks(t *testing.T) {
	t.Parallel()

	supportLinks := []codersdk.LinkConfig{
		{
			Name:   "First link",
			Target: "http://first-link-1",
			Icon:   "chat",
		},
		{
			Name:   "Second link",
			Target: "http://second-link-2",
			Icon:   "bug",
		},
	}
	cfg := coderdtest.DeploymentValues(t)
	cfg.Support.Links = serpent.Struct[[]codersdk.LinkConfig]{
		Value: supportLinks,
	}

	adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: cfg,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAppearance: 1,
			},
		},
	})

	anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	appr, err := anotherClient.Appearance(ctx)
	require.NoError(t, err)
	require.Equal(t, supportLinks, appr.SupportLinks)
}

func TestCustomDocsURL(t *testing.T) {
	t.Parallel()

	testURLRawString := "http://google.com"
	testURL, err := url.Parse(testURLRawString)
	require.NoError(t, err)
	cfg := coderdtest.DeploymentValues(t)
	cfg.DocsURL = *serpent.URLOf(testURL)
	adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true, Options: &coderdtest.Options{DeploymentValues: cfg}})
	anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	appr, err := anotherClient.Appearance(ctx)
	require.NoError(t, err)
	require.Equal(t, testURLRawString, appr.DocsURL)
}

func TestDefaultSupportLinksWithCustomDocsUrl(t *testing.T) {
	t.Parallel()

	// Don't need to set the license, as default links are passed without it.
	testURLRawString := "http://google.com"
	testURL, err := url.Parse(testURLRawString)
	require.NoError(t, err)
	cfg := coderdtest.DeploymentValues(t)
	cfg.DocsURL = *serpent.URLOf(testURL)
	adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true, Options: &coderdtest.Options{DeploymentValues: cfg}})
	anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	appr, err := anotherClient.Appearance(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.DefaultSupportLinks(testURLRawString), appr.SupportLinks)
}

func TestDefaultSupportLinks(t *testing.T) {
	t.Parallel()

	// Don't need to set the license, as default links are passed without it.
	adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
	anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	appr, err := anotherClient.Appearance(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.DefaultSupportLinks(codersdk.DefaultDocsURL()), appr.SupportLinks)
}
