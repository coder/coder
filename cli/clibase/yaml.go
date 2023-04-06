package clibase

import (
	"errors"
	"fmt"
	"strings"

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
//
// It is isomorphic with FromYAML.
func (s *OptionSet) ToYAML() (*yaml.Node, error) {
	root := yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, opt := range *s {
		if opt.YAML == "" {
			continue
		}

		defValue := opt.Default
		if defValue == "" {
			defValue = "<unset>"
		}
		comment := wordwrap.WrapString(
			fmt.Sprintf("%s (default: %s)", opt.Description, defValue),
			80,
		)
		nameNode := yaml.Node{
			Kind:        yaml.ScalarNode,
			Value:       opt.YAML,
			HeadComment: wordwrap.WrapString(comment, 80),
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
			// The all-other types case.
			//
			// A bit of a hack, we marshal and then unmarshal to get
			// the underlying node.
			byt, err := yaml.Marshal(opt.Value)
			if err != nil {
				return nil, xerrors.Errorf(
					"marshal %q: %w", opt.Name, err,
				)
			}

			var docNode yaml.Node
			err = yaml.Unmarshal(byt, &docNode)
			if err != nil {
				return nil, xerrors.Errorf(
					"unmarshal %q: %w", opt.Name, err,
				)
			}
			if len(docNode.Content) != 1 {
				return nil, xerrors.Errorf(
					"unmarshal %q: expected one node, got %d",
					opt.Name, len(docNode.Content),
				)
			}

			valueNode = *docNode.Content[0]
		}
		var group []string
		for _, g := range opt.Group.Ancestry() {
			if g.YAML == "" {
				return nil, xerrors.Errorf(
					"group yaml name is empty for %q, groups: %+v",
					opt.Name,
					opt.Group,
				)
			}
			group = append(group, g.YAML)
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

func (g *Group) filterOptionSet(s *OptionSet) []*Option {
	var opts []*Option
	for i := range *s {
		opt := (*s)[i]
		if opt.Group != g {
			continue
		}
		opts = append(opts, &opt)
	}
	return opts
}

// mapYAMLNodes converts n into a map with keys of form "group.subgroup.option"
// and values of the corresponding YAML nodes.
func mapYAMLNodes(n *yaml.Node) (map[string]*yaml.Node, error) {
	if n.Kind != yaml.MappingNode {
		return nil, xerrors.Errorf("expected mapping node, got type %v", n.Kind)
	}
	var (
		key  string
		m    = make(map[string]*yaml.Node)
		merr error
	)
	for i, node := range n.Content {
		if i&2 == 0 {
			if node.Kind != yaml.ScalarNode {
				return nil, xerrors.Errorf("expected scalar node for key, got type %v", n.Content[i].Kind)
			}
			key = node.Value
			continue
		}
		// Even if we have a mapping node, we don't know if it's a group or a
		// complex option, so we store both.
		m[key] = node
		if node.Kind == yaml.MappingNode {
			sub, err := mapYAMLNodes(node)
			if err != nil {
				merr = errors.Join(merr, xerrors.Errorf("mapping node %q: %w", key, err))
				continue
			}
			for k, v := range sub {
				m[key+"."+k] = v
			}
		}
	}

	return m, nil
}

func (o *Option) setFromYAMLNode(n *yaml.Node) error {
	if um, ok := o.Value.(yaml.Unmarshaler); ok {
		return um.UnmarshalYAML(n)
	}

	switch n.Kind {
	case yaml.ScalarNode:
		return o.Value.Set(n.Value)
	case yaml.SequenceNode:
		return n.Decode(o.Value)
	case yaml.MappingNode:
		return xerrors.Errorf("mapping node must implement yaml.Unmarshaler")
	default:
		return xerrors.Errorf("unexpected node kind %v", n.Kind)
	}
}

// FromYAML converts the given YAML node into the option set.
// It is isomorphic with ToYAML.
func (s *OptionSet) FromYAML(rootNode *yaml.Node) error {
	// The rootNode will be a DocumentNode if it's read from a file. Currently,
	// we don't support multiple YAML documents.
	if rootNode.Kind == yaml.DocumentNode {
		if len(rootNode.Content) != 1 {
			return xerrors.Errorf("expected one node in document, got %d", len(rootNode.Content))
		}
		rootNode = rootNode.Content[0]
	}

	m, err := mapYAMLNodes(rootNode)
	if err != nil {
		return xerrors.Errorf("mapping nodes: %w", err)
	}

	var merr error
	for _, opt := range *s {
		if opt.YAML == "" {
			continue
		}
		var group []string
		for _, g := range opt.Group.Ancestry() {
			if g.YAML == "" {
				return xerrors.Errorf(
					"group yaml name is empty for %q, groups: %+v",
					opt.Name,
					opt.Group,
				)
			}
			group = append(group, g.YAML)
		}
		key := strings.Join(append(group, opt.YAML), ".")
		node, ok := m[key]
		if !ok {
			continue
		}
		if err := opt.setFromYAMLNode(node); err != nil {
			merr = errors.Join(merr, xerrors.Errorf("setting %q: %w", opt.YAML, err))
		}
		delete(m, key)
	}

	for k := range m {
		merr = errors.Join(merr, xerrors.Errorf("unknown option %q", k))
	}

	return merr
}
