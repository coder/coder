package wsrelated

import (
	"strings"

	"golang.org/x/xerrors"
)

// schema holds the root nodes of the include_related hierarchy. It is built once
// in init rather than on every Parse call, since it is immutable.
var schema []relatedNode

func init() {
	schema = buildSchema()
}

// relatedNode describes one node in the related-data hierarchy for the purpose
// of parsing the include_related query parameter. selectInto marks this node as
// selected in the target tree; via the ensure* helpers it also creates the
// node's ancestors, so selecting a node implicitly selects its ancestors.
type relatedNode struct {
	name       string
	selectInto func(*Config)
	children   []relatedNode
}

func (c *Config) ensureLatestBuild() *LatestBuild {
	if c.LatestBuild == nil {
		c.LatestBuild = &LatestBuild{}
	}
	return c.LatestBuild
}

func (c *LatestBuild) ensureJob() *Job {
	if c.Job == nil {
		c.Job = &Job{}
	}
	return c.Job
}

func (c *LatestBuild) ensureResources() *Resources {
	if c.Resources == nil {
		c.Resources = &Resources{}
	}
	return c.Resources
}

func (c *Resources) ensureAgents() *Agents {
	if c.Agents == nil {
		c.Agents = &Agents{}
	}
	return c.Agents
}

func (c *Agents) ensureApps() *Apps {
	if c.Apps == nil {
		c.Apps = &Apps{}
	}
	return c.Apps
}

// buildSchema constructs the include_related hierarchy. Each node's name matches
// a segment in a dotted path, and its selectInto closure mirrors the
// corresponding node in Config.
func buildSchema() []relatedNode {
	return []relatedNode{
		{
			name:       "template",
			selectInto: func(r *Config) { r.Template = true },
		},
		{
			name:       "latest_build",
			selectInto: func(r *Config) { r.ensureLatestBuild() },
			children: []relatedNode{
				{
					name:       "job",
					selectInto: func(r *Config) { r.ensureLatestBuild().ensureJob() },
					children: []relatedNode{
						{
							name:       "queue_position",
							selectInto: func(r *Config) { r.ensureLatestBuild().ensureJob().QueuePosition = true },
						},
					},
				},
				{
					name:       "resources",
					selectInto: func(r *Config) { r.ensureLatestBuild().ensureResources() },
					children: []relatedNode{
						{
							name:       "metadata",
							selectInto: func(r *Config) { r.ensureLatestBuild().ensureResources().Metadata = true },
						},
						{
							name:       "agents",
							selectInto: func(r *Config) { r.ensureLatestBuild().ensureResources().ensureAgents() },
							children: []relatedNode{
								{
									name:       "apps",
									selectInto: func(r *Config) { r.ensureLatestBuild().ensureResources().ensureAgents().ensureApps() },
									children: []relatedNode{
										{
											name:       "statuses",
											selectInto: func(r *Config) { r.ensureLatestBuild().ensureResources().ensureAgents().ensureApps().Statuses = true },
										},
									},
								},
								{
									name:       "scripts",
									selectInto: func(r *Config) { r.ensureLatestBuild().ensureResources().ensureAgents().Scripts = true },
								},
								{
									name:       "log_sources",
									selectInto: func(r *Config) { r.ensureLatestBuild().ensureResources().ensureAgents().LogSources = true },
								},
							},
						},
					},
				},
				{
					name:       "template_version",
					selectInto: func(r *Config) { r.ensureLatestBuild().TemplateVersion = true },
				},
			},
		},
	}
}

// Parse parses the include_related query parameter: a comma-separated list of
// dotted hierarchy paths, e.g. "template,latest_build.resources.agents.*". Each
// path may end in a single wildcard "*" that selects the node and all of its
// descendants; "*" on its own selects everything. Selecting a node implicitly
// selects its ancestors.
//
// Surrounding whitespace and empty entries are ignored. An empty string selects
// nothing but the workspace itself; callers that want the RFC's "absent
// parameter means everything" behavior should use All when the parameter is not
// present. An unknown path or a wildcard that is not the final segment returns
// an error.
func Parse(includeRelated string) (Config, error) {
	var result Config

	tokens := strings.Split(includeRelated, ",")
	for i, raw := range tokens {
		token := strings.TrimSpace(raw)
		if token == "" {
			// Reject empty entries such as "a,,b" or ", ,", but allow a single
			// optional trailing comma (an empty final element).
			if i == len(tokens)-1 {
				continue
			}
			return Config{}, xerrors.New("empty include_related path")
		}

		if token == "*" {
			for _, n := range schema {
				selectSubtree(n, &result)
			}
			continue
		}

		path := token
		wildcard := false
		if strings.HasSuffix(path, ".*") {
			wildcard = true
			path = strings.TrimSuffix(path, ".*")
		}

		segments := strings.Split(path, ".")
		for _, s := range segments {
			if s == "" || s == "*" {
				return Config{}, xerrors.Errorf("invalid include_related path %q", token)
			}
		}

		node, ok := findRelatedNode(schema, segments)
		if !ok {
			return Config{}, xerrors.Errorf("unknown include_related path %q", token)
		}

		if wildcard {
			selectSubtree(node, &result)
		} else {
			node.selectInto(&result)
		}
	}

	return result, nil
}

// findRelatedNode walks the schema following the dotted path segments, returning
// the matching node.
func findRelatedNode(nodes []relatedNode, segments []string) (relatedNode, bool) {
	if len(segments) == 0 {
		return relatedNode{}, false
	}
	for _, n := range nodes {
		if n.name != segments[0] {
			continue
		}
		if len(segments) == 1 {
			return n, true
		}
		return findRelatedNode(n.children, segments[1:])
	}
	return relatedNode{}, false
}

// selectSubtree selects a node and all of its descendants.
func selectSubtree(n relatedNode, r *Config) {
	n.selectInto(r)
	for _, c := range n.children {
		selectSubtree(c, r)
	}
}
