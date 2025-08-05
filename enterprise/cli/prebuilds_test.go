package cli_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
)

func TestPrebuildsPause(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		inv, conf := newCLI(t, "prebuilds", "pause")
		var buf bytes.Buffer
		inv.Stderr = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Verify the output message
		assert.Contains(t, buf.String(), "Prebuilds are now paused.")

		// Verify the settings were actually updated
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv.Context())
		require.NoError(t, err)
		assert.True(t, settings.ReconciliationPaused)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		t.Parallel()

		adminClient, admin := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Create a regular user without admin privileges
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

		inv, conf := newCLI(t, "prebuilds", "pause")
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
		})

		inv, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		// Should fail without license
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})

	t.Run("AlreadyPaused", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// First pause
		inv1, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)
		err := inv1.Run()
		require.NoError(t, err)

		// Try to pause again
		inv2, conf2 := newCLI(t, "prebuilds", "pause")
		clitest.SetupConfig(t, client, conf2)
		err = inv2.Run()
		require.NoError(t, err) // Should succeed even if already paused

		// Verify still paused
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv2.Context())
		require.NoError(t, err)
		assert.True(t, settings.ReconciliationPaused)
	})
}

func TestPrebuildsResume(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// First pause prebuilds
		inv1, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)
		err := inv1.Run()
		require.NoError(t, err)

		// Then resume
		inv2, conf2 := newCLI(t, "prebuilds", "resume")
		var buf bytes.Buffer
		inv2.Stderr = &buf
		clitest.SetupConfig(t, client, conf2)

		err = inv2.Run()
		require.NoError(t, err)

		// Verify the output message
		assert.Contains(t, buf.String(), "Prebuilds are now resumed.")

		// Verify the settings were actually updated
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv2.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)
	})

	t.Run("ResumeWhenNotPaused", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Resume without first pausing
		inv, conf := newCLI(t, "prebuilds", "resume")
		var buf bytes.Buffer
		inv.Stderr = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Should succeed and show the message
		assert.Contains(t, buf.String(), "Prebuilds are now resumed.")

		// Verify still not paused
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(inv.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		t.Parallel()

		adminClient, admin := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Create a regular user without admin privileges
		client, _ := coderdtest.CreateAnotherUser(t, adminClient, admin.OrganizationID)

		inv, conf := newCLI(t, "prebuilds", "resume")
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})

	t.Run("NoLicense", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			DontAddLicense: true,
		})

		inv, conf := newCLI(t, "prebuilds", "resume")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.Error(t, err)
		// Should fail without license
		var sdkError *codersdk.Error
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		assert.Equal(t, http.StatusForbidden, sdkError.StatusCode())
	})
}

func TestPrebuildsCommand(t *testing.T) {
	t.Parallel()

	t.Run("Help", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		inv, conf := newCLI(t, "prebuilds", "--help")
		var buf bytes.Buffer
		inv.Stdout = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Verify help output contains expected information
		output := buf.String()
		assert.Contains(t, output, "Manage Coder prebuilds")
		assert.Contains(t, output, "pause")
		assert.Contains(t, output, "resume")
		assert.Contains(t, output, "Administrators can use these commands")
	})

	t.Run("NoSubcommand", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		inv, conf := newCLI(t, "prebuilds")
		var buf bytes.Buffer
		inv.Stdout = &buf
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)

		err := inv.Run()
		require.NoError(t, err)

		// Should show help when no subcommand is provided
		output := buf.String()
		assert.Contains(t, output, "Manage Coder prebuilds")
		assert.Contains(t, output, "pause")
		assert.Contains(t, output, "resume")
	})
}

func TestPrebuildsSettingsAPI(t *testing.T) {
	t.Parallel()

	t.Run("GetSettings", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureWorkspacePrebuilds: 1,
				},
			},
		})

		// Get initial settings
		//nolint:gocritic // Only owners can change deployment settings
		settings, err := client.GetPrebuildsSettings(t.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)

		// Pause prebuilds
		inv1, conf := newCLI(t, "prebuilds", "pause")
		//nolint:gocritic // Only owners can change deployment settings
		clitest.SetupConfig(t, client, conf)
		err = inv1.Run()
		require.NoError(t, err)

		// Get settings again
		settings, err = client.GetPrebuildsSettings(t.Context())
		require.NoError(t, err)
		assert.True(t, settings.ReconciliationPaused)

		// Resume prebuilds
		inv2, conf2 := newCLI(t, "prebuilds", "resume")
		clitest.SetupConfig(t, client, conf2)
		err = inv2.Run()
		require.NoError(t, err)

		// Get settings one more time
		settings, err = client.GetPrebuildsSettings(t.Context())
		require.NoError(t, err)
		assert.False(t, settings.ReconciliationPaused)
	})
}
