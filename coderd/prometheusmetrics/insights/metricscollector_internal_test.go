package insights

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestConvertTemplateInsights(t *testing.T) {
	t.Parallel()

	t.Run("Malformed", func(t *testing.T) {
		t.Parallel()

		// Reporting zero usage would look like an idle template, so the
		// collector must fail the tick instead.
		templateID := uuid.New()
		rows, err := convertTemplateInsights([]database.GetTemplateInsightsByTemplateRow{
			{TemplateID: templateID, SessionFamilyUsageSeconds: json.RawMessage(`{"ssh": "sixty"}`)},
		})
		require.ErrorContains(t, err, "template "+templateID.String())
		require.Nil(t, rows)
	})
}
