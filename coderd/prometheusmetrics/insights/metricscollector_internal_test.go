package insights

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestCapSessionApps(t *testing.T) {
	t.Parallel()

	limit := codersdk.TemplateInsightsMaxUnregisteredApps
	byApp := map[string]int64{
		"vscode":                          1,
		"cursor":                          1,
		string(codersdk.AppFamilyUnknown): 1,
		codersdk.AppNameOverflow:          10,
	}
	// One more unregistered name than the limit. The busiest are kept, ties
	// broken by name, so the quietest name is the one folded.
	for i := range limit + 1 {
		byApp[fmt.Sprintf("tool_%03d", i)] = int64(1000 - i)
	}

	capped := capSessionApps(byApp)

	// Registered and accounting names never count toward the limit.
	require.EqualValues(t, 1, capped["vscode"])
	require.EqualValues(t, 1, capped["cursor"])
	require.EqualValues(t, 1, capped[string(codersdk.AppFamilyUnknown)])
	require.Len(t, capped, limit+4)
	require.EqualValues(t, 1000, capped["tool_000"])
	require.NotContains(t, capped, fmt.Sprintf("tool_%03d", limit))
	require.EqualValues(t, 10+1000-limit, capped[codersdk.AppNameOverflow])
}

func TestCapSessionAppsTiesByName(t *testing.T) {
	t.Parallel()

	byApp := map[string]int64{}
	for i := range codersdk.TemplateInsightsMaxUnregisteredApps + 1 {
		byApp[fmt.Sprintf("tool_%03d", i)] = 60
	}

	capped := capSessionApps(byApp)

	require.Contains(t, capped, "tool_000")
	require.NotContains(t, capped, fmt.Sprintf("tool_%03d", codersdk.TemplateInsightsMaxUnregisteredApps))
	require.EqualValues(t, 60, capped[codersdk.AppNameOverflow])
}
