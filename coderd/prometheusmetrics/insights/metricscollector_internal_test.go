package insights

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestConvertTemplateInsights(t *testing.T) {
	t.Parallel()

	rows := convertTemplateInsights([]database.GetTemplateInsightsByTemplateRow{
		{
			TemplateID:             uuid.New(),
			SessionAppUsageSeconds: database.StringMapOfInt{"vscode": 300, "cursor": 120, "zed": 60},
		},
	})
	require.Len(t, rows, 1)
	require.EqualValues(t, 420, rows[0].usageSecondsByFamily[codersdk.AppFamilyVSCode])
	require.EqualValues(t, 60, rows[0].usageSecondsByFamily[codersdk.AppFamilySSH])
}
