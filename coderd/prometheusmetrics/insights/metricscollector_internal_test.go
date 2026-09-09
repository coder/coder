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

		for name, raw := range map[string]json.RawMessage{
			"NotJSON":     json.RawMessage(`{`),
			"NotAnObject": json.RawMessage(`[1, 2]`),
			"WrongValue":  json.RawMessage(`{"ssh": "sixty"}`),
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				// Reporting zero usage would look like an idle template, so
				// the collector must fail the tick instead.
				rows, err := convertTemplateInsights([]database.GetTemplateInsightsByTemplateRow{
					{TemplateID: uuid.New(), SessionFamilyUsageSeconds: raw},
				})
				require.Error(t, err)
				require.Nil(t, rows)
			})
		}
	})

	t.Run("AbsentPayload", func(t *testing.T) {
		t.Parallel()

		rows, err := convertTemplateInsights([]database.GetTemplateInsightsByTemplateRow{
			{TemplateID: uuid.New()},
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.NotNil(t, rows[0].usageSecondsByFamily)
		require.EqualValues(t, 0, rows[0].usageSeconds(codersdk.AppFamilySSH))
	})
}
