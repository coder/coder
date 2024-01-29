package coderd_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
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

func TestServiceBanners(t *testing.T) {
	t.Parallel()

	t.Run("User", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
		basicUserClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

		// Even without a license, the banner should return as disabled.
		sb, err := basicUserClient.Appearance(ctx)
		require.NoError(t, err)
		require.False(t, sb.ServiceBanner.Enabled)

		coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAppearance: 1,
			},
		})

		// Default state
		sb, err = basicUserClient.Appearance(ctx)
		require.NoError(t, err)
		require.False(t, sb.ServiceBanner.Enabled)

		uac := codersdk.UpdateAppearanceConfig{
			ServiceBanner: sb.ServiceBanner,
		}
		// Regular user should be unable to set the banner
		uac.ServiceBanner.Enabled = true

		err = basicUserClient.UpdateAppearance(ctx, uac)
		require.Error(t, err)
		var sdkError *codersdk.Error
		require.True(t, errors.As(err, &sdkError))
		require.Equal(t, http.StatusForbidden, sdkError.StatusCode())

		// But an admin can
		wantBanner := uac
		wantBanner.ServiceBanner.Enabled = true
		wantBanner.ServiceBanner.Message = "Hey"
		wantBanner.ServiceBanner.BackgroundColor = "#00FF00"
		err = adminClient.UpdateAppearance(ctx, wantBanner)
		require.NoError(t, err)
		gotBanner, err := adminClient.Appearance(ctx) //nolint:gocritic // we should assert at least once that the owner can get the banner
		require.NoError(t, err)
		gotBanner.SupportLinks = nil // clean "support links" before comparison
		require.Equal(t, wantBanner.ServiceBanner, gotBanner.ServiceBanner)

		// But even an admin can't give a bad color
		wantBanner.ServiceBanner.BackgroundColor = "#bad color"
		err = adminClient.UpdateAppearance(ctx, wantBanner)
		require.Error(t, err)

		var sdkErr *codersdk.Error
		if assert.ErrorAs(t, err, &sdkErr) {
			assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			assert.Contains(t, sdkErr.Message, "Invalid color format")
			assert.Contains(t, sdkErr.Detail, "expected # prefix and 6 characters")
		}
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
			ServiceBanner: codersdk.ServiceBannerConfig{
				Enabled:         true,
				Message:         "Hey",
				BackgroundColor: "#00FF00",
			},
		}
		err := client.UpdateAppearance(ctx, cfg)
		require.NoError(t, err)

		r := dbfake.WorkspaceBuild(t, store, database.Workspace{
			OrganizationID: user.OrganizationID,
			OwnerID:        user.UserID,
		}).WithAgent().Do()

		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(r.AgentToken)
		banner, err := agentClient.GetServiceBanner(ctx)
		require.NoError(t, err)
		require.Equal(t, cfg.ServiceBanner, banner)
		banner = requireGetServiceBannerV2(ctx, t, agentClient)
		require.Equal(t, cfg.ServiceBanner, banner)

		// Create an AGPL Coderd against the same database
		agplClient := coderdtest.New(t, &coderdtest.Options{Database: store, Pubsub: ps})
		agplAgentClient := agentsdk.New(agplClient.URL)
		agplAgentClient.SetSessionToken(r.AgentToken)
		banner, err = agplAgentClient.GetServiceBanner(ctx)
		require.NoError(t, err)
		require.Equal(t, codersdk.ServiceBannerConfig{}, banner)
		banner = requireGetServiceBannerV2(ctx, t, agplAgentClient)
		require.Equal(t, codersdk.ServiceBannerConfig{}, banner)

		// No license means no banner.
		err = client.DeleteLicense(ctx, lic.ID)
		require.NoError(t, err)
		banner, err = agentClient.GetServiceBanner(ctx)
		require.NoError(t, err)
		require.Equal(t, codersdk.ServiceBannerConfig{}, banner)
		banner = requireGetServiceBannerV2(ctx, t, agentClient)
		require.Equal(t, codersdk.ServiceBannerConfig{}, banner)
	})
}

func requireGetServiceBannerV2(ctx context.Context, t *testing.T, client *agentsdk.Client) codersdk.ServiceBannerConfig {
	cc, err := client.Listen(ctx)
	require.NoError(t, err)
	defer func() {
		_ = cc.Close()
	}()
	aAPI := proto.NewDRPCAgentClient(cc)
	sbp, err := aAPI.GetServiceBanner(ctx, &proto.GetServiceBannerRequest{})
	require.NoError(t, err)
	return proto.SDKServiceBannerFromProto(sbp)
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
	cfg.Support.Links = clibase.Struct[[]codersdk.LinkConfig]{
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

func TestDefaultSupportLinks(t *testing.T) {
	t.Parallel()

	// Don't need to set the license, as default links are passed without it.
	adminClient, adminUser := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
	anotherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	appr, err := anotherClient.Appearance(ctx)
	require.NoError(t, err)
	require.Equal(t, appearance.DefaultSupportLinks, appr.SupportLinks)
}
