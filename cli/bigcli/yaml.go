package bigcli

import (
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"gopkg.in/yaml.v3"
)

// deepMapNode returns the mapping node at the given path,
// creating it if it doesn't exist.
func deepMapNode(n *yaml.Node, path []string) *yaml.Node {
	if len(path) == 0 {
		return n
	}

	// Name is every two nodes.
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == path[0] {
			// Found matching name, recurse.
			return deepMapNode(n.Content[i+1], path[1:])
		}
	}

	// Not found, create it.
	nameNode := yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: path[0],
	}
	valueNode := yaml.Node{
		Kind: yaml.MappingNode,
	}
	n.Content = append(n.Content, &nameNode)
	n.Content = append(n.Content, &valueNode)
	return deepMapNode(&valueNode, path[1:])
}

// ToYAML converts the option set to a YAML node, that can be
// converted into bytes via yaml.Marshal.
//
// The node is returned to enable post-processing higher up in
// the stack.
func (s OptionSet) ToYAML() (*yaml.Node, error) {
	root := yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, opt := range s {
		if opt.YAML == "" {
			continue
		}
		nameNode := yaml.Node{
			Kind:        yaml.ScalarNode,
			Value:       opt.YAML,
			HeadComment: wordwrap.WrapString(opt.Description, 80),
		}
		valueNode := yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: opt.Value.String(),
		}
		var group []string
		for _, g := range opt.Group.Ancestry() {
			group = append(group, strings.ToLower(g.Name))
		}
		parent := deepMapNode(&root, group)
		parent.Content = append(
			parent.Content,
			&nameNode,
			&valueNode,
		)
	}
	return &root, nil
}
