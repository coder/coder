package agentmetrics

import (
	"strings"

	"golang.org/x/xerrors"
)

const (
	LabelAgentName     = "agent_name"
	LabelTemplateName  = "template_name"
	LabelUsername      = "username"
	LabelWorkspaceName = "workspace_name"
)

var (
	LabelAll        = []string{LabelAgentName, LabelTemplateName, LabelUsername, LabelWorkspaceName}
	LabelAgentStats = []string{LabelAgentName, LabelUsername, LabelWorkspaceName}
)

// ValidateAggregationLabels ensures a given set of labels are valid aggregation labels.
func ValidateAggregationLabels(labels []string) error {
	acceptable := LabelAll

	seen := make(map[string]any, len(acceptable))
	for _, label := range acceptable {
		seen[label] = nil
	}

	for _, label := range labels {
		if _, found := seen[label]; !found {
			return xerrors.Errorf("%q is not a valid aggregation label; only one or more of %q are acceptable",
				label, strings.Join(acceptable, ", "))
		}
	}

	return nil
}
