package bigcli

import (
	"gopkg.in/yaml.v3"
)

func (s *OptionSet) ToYAML() (*yaml.Node, error) {
	root := yaml.Node{
		Kind: yaml.MappingNode,
	}

	// First, write all groups so that we can reference
	// them when writing individual options.
	for level := 1; ; level++ {
		foundGroups := make(map[string]struct{})
		// Find all groups at this level.
		for _, opt := range *s {
			if len(opt.Group) < level {
				continue
			}
			name := opt.Group[level-1]
			foundGroups[name] = struct{}{}
		}

		for groupName := range foundGroups {
			// Write group name.
			nameNode := yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: groupName,
			}
			root.Content = append(root.Content, &nameNode)
			// Write group value.
			valueNode := yaml.Node{
				Kind: yaml.MappingNode,
			}
			root.Content = append(root.Content, &valueNode)
		}
		if len(foundGroups) == 0 {
			break
		}
	}
	return &root, nil
}
