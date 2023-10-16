package coderd_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestCustomLogoAndCompanyName(t *testing.T) {
	t.Parallel()

	// Prepare enterprise deployment
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	adminClient, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})
	coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureAppearance: 1,
		},
	})

	// Update logo and application name
	uac := codersdk.UpdateAppearanceConfig{
		ApplicationName: "ACME Ltd",
		LogoURL:         "http://logo-url/file.png",
	}

	err := adminClient.UpdateAppearance(ctx, uac)
	require.NoError(t, err)

	// Verify update
	got, err := adminClient.Appearance(ctx)
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

		// Even without a license, the banner should return as disabled.
		sb, err := adminClient.Appearance(ctx)
		require.NoError(t, err)
		require.False(t, sb.ServiceBanner.Enabled)

		coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAppearance: 1,
			},
		})

		// Default state
		sb, err = adminClient.Appearance(ctx)
		require.NoError(t, err)
		require.False(t, sb.ServiceBanner.Enabled)

		basicUserClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)

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
		gotBanner, err := adminClient.Appearance(ctx)
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

		client, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			DontAddLicense: true,
		})
		license := coderdenttest.AddLicense(t, client, coderdenttest.LicenseOptions{
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

		authToken := uuid.NewString()
		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
			Parse:          echo.ParseComplete,
			ProvisionPlan:  echo.PlanComplete,
			ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
		})
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

		banner, err := agentClient.GetServiceBanner(ctx)
		require.NoError(t, err)
		require.Equal(t, cfg.ServiceBanner, banner)

		// No enterprise means a 404 on the endpoint meaning no banner.
		client = coderdtest.New(t, &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		})
		agentClient = agentsdk.New(client.URL)
		agentClient.SetSessionToken(authToken)
		banner, err = agentClient.GetServiceBanner(ctx)
		require.NoError(t, err)
		require.Equal(t, codersdk.ServiceBannerConfig{}, banner)

		// No license means no banner.
		client.DeleteLicense(ctx, license.ID)
		banner, err = agentClient.GetServiceBanner(ctx)
		require.NoError(t, err)
		require.Equal(t, codersdk.ServiceBannerConfig{}, banner)
	})
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

	client, _ := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: cfg,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureAppearance: 1,
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	appearance, err := client.Appearance(ctx)
	require.NoError(t, err)
	require.Equal(t, supportLinks, appearance.SupportLinks)
}

func TestDefaultSupportLinks(t *testing.T) {
	t.Parallel()

	// Don't need to set the license, as default links are passed without it.
	client, _ := coderdenttest.New(t, &coderdenttest.Options{DontAddLicense: true})

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()

	appearance, err := client.Appearance(ctx)
	require.NoError(t, err)
	require.Equal(t, coderd.DefaultSupportLinks, appearance.SupportLinks)
}
