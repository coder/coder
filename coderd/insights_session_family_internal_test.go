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

	t.Run("SFTPLegacy", func(t *testing.T) {
		t.Parallel()

		sftpTemplateID := uuid.New()
		apps, err := convertTemplateInsightsApps(database.GetTemplateInsightsRow{
			SessionFamilyUsageSeconds: json.RawMessage(`{"sftp": 300}`),
			SessionFamilyTemplateIds:  json.RawMessage(`{"sftp": ["` + sftpTemplateID.String() + `"]}`),
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
				SessionFamilyUsageSeconds: json.RawMessage(`{"vscode": {}}`),
				SessionFamilyTemplateIds:  json.RawMessage(`{}`),
			},
			"TemplateIDs": {
				SessionFamilyUsageSeconds: json.RawMessage(`{}`),
				SessionFamilyTemplateIds:  json.RawMessage(`{"vscode": "not-a-uuid"}`),
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				apps, err := convertTemplateInsightsApps(usage, nil)
				require.ErrorContains(t, err, "convert template insights apps: decode session family")
				require.Nil(t, apps)
			})
		}
	})
}
