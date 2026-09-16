package coderd

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestConvertTemplateInsightsApps(t *testing.T) {
	t.Parallel()

	t.Run("FoldsAppsIntoFamily", func(t *testing.T) {
		t.Parallel()

		sharedTemplateID, cursorTemplateID := uuid.New(), uuid.New()
		apps, err := convertTemplateInsightsApps(database.GetTemplateInsightsRow{
			SessionAppUsageSeconds: json.RawMessage(`{"vscode": 300, "cursor": 120, "jetbrains": 60}`),
			SessionAppTemplateIds: json.RawMessage(`{
				"vscode": ["` + sharedTemplateID.String() + `"],
				"cursor": ["` + sharedTemplateID.String() + `", "` + cursorTemplateID.String() + `"]
			}`),
		}, nil)
		require.NoError(t, err)

		vscode := appBySlug(t, apps, "vscode")
		// Minutes the two share count in both, so the family adds them up.
		require.EqualValues(t, 420, vscode.Seconds)
		require.ElementsMatch(t, []uuid.UUID{sharedTemplateID, cursorTemplateID}, vscode.TemplateIDs)
		require.EqualValues(t, 60, appBySlug(t, apps, "jetbrains").Seconds)
		require.Equal(t, []uuid.UUID{}, appBySlug(t, apps, "ssh").TemplateIDs)
	})

	t.Run("UnknownAppIsNotABuiltin", func(t *testing.T) {
		t.Parallel()

		apps, err := convertTemplateInsightsApps(database.GetTemplateInsightsRow{
			SessionAppUsageSeconds: json.RawMessage(`{"some_new_ide": 300}`),
			SessionAppTemplateIds:  json.RawMessage(`{}`),
		}, nil)
		require.NoError(t, err)
		for _, app := range apps {
			require.Zero(t, app.Seconds, "app %q", app.Slug)
		}
	})

	t.Run("SFTPLegacy", func(t *testing.T) {
		t.Parallel()

		sftpTemplateID := uuid.New()
		apps, err := convertTemplateInsightsApps(database.GetTemplateInsightsRow{
			SessionAppUsageSeconds: json.RawMessage(`{"sftp": 300}`),
			SessionAppTemplateIds:  json.RawMessage(`{"sftp": ["` + sftpTemplateID.String() + `"]}`),
		}, nil)
		require.NoError(t, err)
		for _, app := range apps {
			if app.Slug != "sftp" {
				require.Equal(t, []uuid.UUID{}, app.TemplateIDs)
			}
		}
		require.Contains(t, apps, codersdk.TemplateAppUsage{
			// The rollup no longer produces SFTP usage, but rows migrated
			// from the old sftp_mins column still report it.
			TemplateIDs: []uuid.UUID{sftpTemplateID},
			Type:        codersdk.TemplateAppsTypeBuiltin,
			DisplayName: codersdk.TemplateBuiltinAppDisplayNameSFTP,
			Slug:        "sftp",
			Icon:        "/icon/terminal.svg",
			Seconds:     300,
		})
	})

	t.Run("Malformed", func(t *testing.T) {
		t.Parallel()

		for name, usage := range map[string]database.GetTemplateInsightsRow{
			"UsageSeconds": {
				SessionAppUsageSeconds: json.RawMessage(`{"vscode": {}}`),
				SessionAppTemplateIds:  json.RawMessage(`{}`),
			},
			"TemplateIDs": {
				SessionAppUsageSeconds: json.RawMessage(`{}`),
				SessionAppTemplateIds:  json.RawMessage(`{"vscode": "not-a-uuid"}`),
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				apps, err := convertTemplateInsightsApps(usage, nil)
				require.ErrorContains(t, err, "decode session app")
				require.Nil(t, apps)
			})
		}
	})
}

func appBySlug(t *testing.T, apps []codersdk.TemplateAppUsage, slug string) codersdk.TemplateAppUsage {
	t.Helper()

	for _, app := range apps {
		if app.Slug == slug {
			return app
		}
	}
	t.Fatalf("no app with slug %q", slug)
	return codersdk.TemplateAppUsage{}
}
