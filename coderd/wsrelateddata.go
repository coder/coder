package coderd

// workspaceRelated selects which workspace-related database objects to load when
// building a codersdk.Workspace. Loading a fully populated workspace is
// expensive, so callers use this to avoid querying data they will not use.
//
// The type is a tree that mirrors the parent/child relationships between those
// objects: a build has a job, resources, and a template version; a resource has
// agents; an agent has apps; and so on. Branch nodes are pointers that are
// non-nil when selected; leaf nodes are bools. Modeling it as a tree makes
// selecting a child without its parent unrepresentable, which is exactly the
// constraint loading requires: a parent must be queried to learn the
// identifiers of its children.
//
// A zero value (nil branches) selects nothing but the workspace itself.
// allWorkspaceRelated selects everything.
//
// The trailing comment on each field is that node's dotted path from the root
// of the tree, e.g. latest_build.resources.agents.
type workspaceRelated struct {
	Template    bool                // template
	LatestBuild *latestBuildRelated // latest_build
}

type latestBuildRelated struct {
	Job             *jobRelated       // latest_build.job
	Resources       *resourcesRelated // latest_build.resources
	TemplateVersion bool              // latest_build.template_version
}

type jobRelated struct {
	QueuePosition bool // latest_build.job.queue_position
}

type resourcesRelated struct {
	Metadata bool           // latest_build.resources.metadata
	Agents   *agentsRelated // latest_build.resources.agents
}

type agentsRelated struct {
	Apps       *appsRelated // latest_build.resources.agents.apps
	Scripts    bool         // latest_build.resources.agents.scripts
	LogSources bool         // latest_build.resources.agents.log_sources
}

type appsRelated struct {
	Statuses bool // latest_build.resources.agents.apps.statuses
}

// allWorkspaceRelated returns a selection that loads every related object. It
// reproduces the behavior of callers that have not been narrowed to a specific
// subset.
func allWorkspaceRelated() workspaceRelated {
	latestBuild := allLatestBuildRelated()
	return workspaceRelated{
		Template:    true,
		LatestBuild: &latestBuild,
	}
}

// allLatestBuildRelated returns the latest-build subtree with every node
// selected.
func allLatestBuildRelated() latestBuildRelated {
	return latestBuildRelated{
		Job: &jobRelated{QueuePosition: true},
		Resources: &resourcesRelated{
			Metadata: true,
			Agents: &agentsRelated{
				Apps:       &appsRelated{Statuses: true},
				Scripts:    true,
				LogSources: true,
			},
		},
		TemplateVersion: true,
	}
}

// appStatuses reports whether app statuses
// (latest_build.resources.agents.apps.statuses) are selected. It is nil-safe so
// callers holding a possibly-nil subtree can descend without a chain of guards.
func (c *latestBuildRelated) appStatuses() bool {
	return c != nil &&
		c.Resources != nil &&
		c.Resources.Agents != nil &&
		c.Resources.Agents.Apps != nil &&
		c.Resources.Agents.Apps.Statuses
}
