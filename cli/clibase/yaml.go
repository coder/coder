package clibase

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

var (
	_ yaml.Marshaler   = new(OptionSet)
	_ yaml.Unmarshaler = new(OptionSet)
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

// MarshalYAML converts the option set to a YAML node, that can be
// converted into bytes via yaml.Marshal.
//
// The node is returned to enable post-processing higher up in
// the stack.
//
// It is isomorphic with FromYAML.
func (optSet *OptionSet) MarshalYAML() (any, error) {
	root := yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, opt := range *optSet {
		if opt.YAML == "" {
			continue
		}

		defValue := opt.Default
		if defValue == "" {
			defValue = "<unset>"
		}
		comment := wordwrap.WrapString(
			fmt.Sprintf("%s\n(default: %s, type: %s)", opt.Description, defValue, opt.Value.Type()),
			80,
		)
		nameNode := yaml.Node{
			Kind:        yaml.ScalarNode,
			Value:       opt.YAML,
			HeadComment: comment,
		}
		var valueNode yaml.Node
		if opt.Value == nil {
			valueNode = yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: "null",
			}
		} else if m, ok := opt.Value.(yaml.Marshaler); ok {
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

// mapYAMLNodes converts parent into a map with keys of form "group.subgroup.option"
// and values as the corresponding YAML nodes.
func mapYAMLNodes(parent *yaml.Node) (map[string]*yaml.Node, error) {
	if parent.Kind != yaml.MappingNode {
		return nil, xerrors.Errorf("expected mapping node, got type %v", parent.Kind)
	}
	if len(parent.Content)%2 != 0 {
		return nil, xerrors.Errorf("expected an even number of k/v pairs, got %d", len(parent.Content))
	}
	var (
		key  string
		m    = make(map[string]*yaml.Node, len(parent.Content)/2)
		merr error
	)
	for i, child := range parent.Content {
		if i%2 == 0 {
			if child.Kind != yaml.ScalarNode {
				// We immediately because the rest of the code is bound to fail
				// if we don't know to expect a key or a value.
				return nil, xerrors.Errorf("expected scalar node for key, got type %v", child.Kind)
			}
			key = child.Value
			continue
		}

		// We don't know if this is a grouped simple option or complex option,
		// so we store both "key" and "group.key". Since we're storing pointers,
		// the additional memory is of little concern.
		m[key] = child
		if child.Kind != yaml.MappingNode {
			continue
		}

		sub, err := mapYAMLNodes(child)
		if err != nil {
			merr = errors.Join(merr, xerrors.Errorf("mapping node %q: %w", key, err))
			continue
		}
		for k, v := range sub {
			m[key+"."+k] = v
		}
	}

	return m, nil
}

func (o *Option) setFromYAMLNode(n *yaml.Node) error {
	o.ValueSource = ValueSourceYAML
	if um, ok := o.Value.(yaml.Unmarshaler); ok {
		return um.UnmarshalYAML(n)
	}

	switch n.Kind {
	case yaml.ScalarNode:
		return o.Value.Set(n.Value)
	case yaml.SequenceNode:
		// We treat empty values as nil for consistency with other option
		// mechanisms.
		if len(n.Content) == 0 {
			o.Value = nil
			return nil
		}
		return n.Decode(o.Value)
	case yaml.MappingNode:
		return xerrors.Errorf("mapping nodes must implement yaml.Unmarshaler")
	default:
		return xerrors.Errorf("unexpected node kind %v", n.Kind)
	}
}

// UnmarshalYAML converts the given YAML node into the option set.
// It is isomorphic with ToYAML.
func (optSet *OptionSet) UnmarshalYAML(rootNode *yaml.Node) error {
	// The rootNode will be a DocumentNode if it's read from a file. We do
	// not support multiple documents in a single file.
	if rootNode.Kind == yaml.DocumentNode {
		if len(rootNode.Content) != 1 {
			return xerrors.Errorf("expected one node in document, got %d", len(rootNode.Content))
		}
		rootNode = rootNode.Content[0]
	}

	yamlNodes, err := mapYAMLNodes(rootNode)
	if err != nil {
		return xerrors.Errorf("mapping nodes: %w", err)
	}

	matchedNodes := make(map[string]*yaml.Node, len(yamlNodes))

	var merr error
	for i := range *optSet {
		opt := &(*optSet)[i]
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
			delete(yamlNodes, strings.Join(group, "."))
		}

		key := strings.Join(append(group, opt.YAML), ".")
		node, ok := yamlNodes[key]
		if !ok {
			continue
		}

		matchedNodes[key] = node
		if opt.ValueSource != ValueSourceNone {
			continue
		}
		if err := opt.setFromYAMLNode(node); err != nil {
			merr = errors.Join(merr, xerrors.Errorf("setting %q: %w", opt.YAML, err))
		}
	}

	// Remove all matched nodes and their descendants from yamlNodes so we
	// can accurately report unknown options.
	for k := range yamlNodes {
		var key string
		for _, part := range strings.Split(k, ".") {
			if key != "" {
				key += "."
			}
			key += part
			if _, ok := matchedNodes[key]; ok {
				delete(yamlNodes, k)
			}
		}
	}
	for k := range yamlNodes {
		merr = errors.Join(merr, xerrors.Errorf("unknown option %q", k))
	}

	return merr
}
