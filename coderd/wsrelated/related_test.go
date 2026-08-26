package wsrelated_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/wsrelated"
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
	require.True(t, all.LatestBuild.Resources.Agents.Apps.Statuses)
	require.True(t, all.LatestBuild.Resources.Agents.Scripts)
	require.True(t, all.LatestBuild.Resources.Agents.LogSources)
	require.True(t, all.LatestBuild.AppStatuses())
}

// TestLatestBuildAppStatuses verifies the nil-safe AppStatuses accessor only
// reports true when the full apps.statuses path is present.
func TestLatestBuildAppStatuses(t *testing.T) {
	t.Parallel()

	require.False(t, (*wsrelated.LatestBuild)(nil).AppStatuses())
	require.False(t, (&wsrelated.LatestBuild{}).AppStatuses())
	require.False(t, (&wsrelated.LatestBuild{Resources: &wsrelated.Resources{}}).AppStatuses())
	require.False(t, (&wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{}}}).AppStatuses())
	require.False(t, (&wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Apps: &wsrelated.Apps{}}}}).AppStatuses())
	require.True(t, (&wsrelated.LatestBuild{Resources: &wsrelated.Resources{Agents: &wsrelated.Agents{Apps: &wsrelated.Apps{Statuses: true}}}}).AppStatuses())
}
