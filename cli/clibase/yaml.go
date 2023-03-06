package clibase

import (
	"github.com/iancoleman/strcase"
	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// deepMapNode returns the mapping node at the given path,
// creating it if it doesn't exist.
func deepMapNode(n *yaml.Node, path []string, headComment string) *yaml.Node {
	if len(path) == 0 {
		return n
	}

	// Name is every two nodes.
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == path[0] {
			// Found matching name, recurse.
			return deepMapNode(n.Content[i+1], path[1:], headComment)
		}
	}

	// Not found, create it.
	nameNode := yaml.Node{
		Kind:        yaml.ScalarNode,
		Value:       path[0],
		HeadComment: headComment,
	}
	valueNode := yaml.Node{
		Kind: yaml.MappingNode,
	}
	n.Content = append(n.Content, &nameNode)
	n.Content = append(n.Content, &valueNode)
	return deepMapNode(&valueNode, path[1:], headComment)
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
		var valueNode yaml.Node
		if m, ok := opt.Value.(yaml.Marshaler); ok {
			v, err := m.MarshalYAML()
			if err != nil {
				return nil, xerrors.Errorf(
					"marshal %q: %w", opt.Name, err,
				)
			}
			valueNode, ok = v.(yaml.Node)
			if !ok {
				return nil, xerrors.Errorf(
					"marshal %q: unexpected underlying type %T",
					opt.Name, v,
				)
			}
		} else {
			valueNode = yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: opt.Value.String(),
			}
		}
		var group []string
		for _, g := range opt.Group.Ancestry() {
			if g.Name == "" {
				return nil, xerrors.Errorf(
					"group name is empty for %q, groups: %+v",
					opt.Name,
					opt.Group,
				)
			}
			group = append(group, strcase.ToLowerCamel(g.Name))
		}
		var groupDesc string
		if opt.Group != nil {
			groupDesc = wordwrap.WrapString(opt.Group.Description, 80)
		}
		parentValueNode := deepMapNode(
			&root, group,
			groupDesc,
		)
		parentValueNode.Content = append(
			parentValueNode.Content,
			&nameNode,
			&valueNode,
		)
	}
	return &root, nil
}
