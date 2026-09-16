package insights

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestConvertTemplateInsights(t *testing.T) {
	t.Parallel()

	t.Run("Malformed", func(t *testing.T) {
		t.Parallel()

		// Reporting zero usage would look like an idle template, so the
		// collector must fail the tick instead.
		templateID := uuid.New()
		rows, err := convertTemplateInsights([]database.GetTemplateInsightsByTemplateRow{
			{TemplateID: templateID, SessionAppUsageSeconds: json.RawMessage(`{"ssh": "sixty"}`)},
		})
		require.ErrorContains(t, err, "template "+templateID.String())
		require.Nil(t, rows)
	})

	t.Run("FoldsAppsIntoFamily", func(t *testing.T) {
		t.Parallel()

		rows, err := convertTemplateInsights([]database.GetTemplateInsightsByTemplateRow{
			{
				TemplateID:             uuid.New(),
				SessionAppUsageSeconds: json.RawMessage(`{"vscode": 300, "cursor": 120, "zed": 60}`),
			},
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.EqualValues(t, 420, rows[0].usageSeconds(codersdk.AppFamilyVSCode))
		require.EqualValues(t, 60, rows[0].usageSeconds(codersdk.AppFamilySSH))
	})
}
