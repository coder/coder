// Package wsrelated models which workspace-related database objects to load when
// building a codersdk.Workspace, and parses the include_related query parameter
// into that model. Loading a fully populated workspace is expensive, so callers
// use a selection to avoid querying data they will not use.
//
// The selection is a tree that mirrors the parent/child relationships between
// those objects: a build has a job, resources, and a template version; a
// resource has agents; an agent has apps; and so on. Branch nodes are pointers
// that are non-nil when selected; leaf nodes are bools. Modeling it as a tree
// makes selecting a child without its parent unrepresentable, which is exactly
// the constraint loading requires: a parent must be queried to learn the
// identifiers of its children.
package wsrelated

// Config is the root of the selection tree. A zero value (nil branches) selects
// nothing but the workspace itself. All selects everything.
//
// The trailing comment on each field is that node's dotted path from the root of
// the tree, e.g. latest_build.resources.agents.
type Config struct {
	Template    bool         // template
	LatestBuild *LatestBuild // latest_build
}

type LatestBuild struct {
	Job             *Job       // latest_build.job
	Resources       *Resources // latest_build.resources
	TemplateVersion bool       // latest_build.template_version
}

type Job struct {
	QueuePosition bool // latest_build.job.queue_position
}

type Resources struct {
	Metadata bool    // latest_build.resources.metadata
	Agents   *Agents // latest_build.resources.agents
}

type Agents struct {
	Apps       *Apps // latest_build.resources.agents.apps
	Scripts    bool  // latest_build.resources.agents.scripts
	LogSources bool  // latest_build.resources.agents.log_sources
}

type Apps struct {
	Statuses bool // latest_build.resources.agents.apps.statuses
}

// All returns a selection that loads every related object. It reproduces the
// behavior of callers that have not been narrowed to a specific subset.
func All() Config {
	latestBuild := AllLatestBuild()
	return Config{
		Template:    true,
		LatestBuild: &latestBuild,
	}
}

// AllLatestBuild returns the latest-build subtree with every node selected.
func AllLatestBuild() LatestBuild {
	return LatestBuild{
		Job: &Job{QueuePosition: true},
		Resources: &Resources{
			Metadata: true,
			Agents: &Agents{
				Apps:       &Apps{Statuses: true},
				Scripts:    true,
				LogSources: true,
			},
		},
		TemplateVersion: true,
	}
}

// AppStatuses reports whether app statuses
// (latest_build.resources.agents.apps.statuses) are selected. It is nil-safe so
// callers holding a possibly-nil subtree can descend without a chain of guards.
func (c *LatestBuild) AppStatuses() bool {
	return c != nil &&
		c.Resources != nil &&
		c.Resources.Agents != nil &&
		c.Resources.Agents.Apps != nil &&
		c.Resources.Agents.Apps.Statuses
}
