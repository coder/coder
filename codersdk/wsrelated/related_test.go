package wsrelated_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/wsrelated"
)

// TestAll verifies that All selects every node in the related-data hierarchy.
func TestAll(t *testing.T) {
	t.Parallel()

	all := wsrelated.All()
	require.True(t, all.Template)
	require.NotNil(t, all.LatestBuild)
	require.NotNil(t, all.LatestBuild.Job)
	require.True(t, all.LatestBuild.Job.QueuePosition)
	require.True(t, all.LatestBuild.TemplateVersion)
	require.NotNil(t, all.LatestBuild.Resources)
	require.True(t, all.LatestBuild.Resources.Metadata)
	require.NotNil(t, all.LatestBuild.Resources.Agents)
	require.NotNil(t, all.LatestBuild.Resources.Agents.Apps)
	require.True(t, all.LatestBuild.Resources.Agents.Scripts)
	require.True(t, all.LatestBuild.Resources.Agents.LogSources)
}
