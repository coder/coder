package clibase

import (
	"errors"
	"fmt"

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
func (s OptionSet) ToYAML() (*yaml.Node, error) {
	root := yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, opt := range s {
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

// FromYAML converts the given YAML node into the option set.
// It is isomorphic with ToYAML.

func (s *OptionSet) FromYAML(n *yaml.Node) error {
	return fromYAML(*s, nil, n)
}

func fromYAML(os OptionSet, ofGroup *Group, n *yaml.Node) error {
	if n.Kind == yaml.DocumentNode && ofGroup == nil {
		// The root may be a document node.
		if len(n.Content) != 1 {
			return xerrors.Errorf("expected one content node, got %d", len(n.Content))
		}
		return fromYAML(os, ofGroup, n.Content[0])
	}

	if n.Kind != yaml.MappingNode {
		byt, _ := yaml.Marshal(n)
		return xerrors.Errorf("expected mapping node, got type %v, contents:\n%v", n.Kind, string(byt))
	}

	var (
		subGroupsByName = make(map[string]*Group)
		optionsByName   = make(map[string]*Option)
	)
	for i, opt := range os {
		if opt.YAML == "" {
			continue
		}

		// We only want to process options that are of the identified group,
		// even if that group is nil.
		if opt.Group != ofGroup {
			if opt.Group != nil && opt.Group.Parent == ofGroup {
				if opt.Group.YAML == "" {
					return xerrors.Errorf("group yaml name is empty for %q, groups: %+v", opt.Name, opt.Group)
				}
				subGroupsByName[opt.Group.YAML] = opt.Group
			}
			continue
		}

		if opt.Group != nil && opt.Group.YAML == "" {
			return xerrors.Errorf("group yaml name is empty for %q", opt.Name)
		}

		if _, ok := optionsByName[opt.YAML]; ok {
			return xerrors.Errorf("duplicate option name %q", opt.YAML)
		}

		optionsByName[opt.YAML] = &os[i]
	}

	for k := range subGroupsByName {
		if _, ok := optionsByName[k]; !ok {
			continue
		}
		return xerrors.Errorf("there is both an option and a group with name %q", k)
	}

	var (
		name string
		merr error
	)

	for i, item := range n.Content {
		if isName := i%2 == 0; isName {
			if item.Kind != yaml.ScalarNode {
				return xerrors.Errorf("expected scalar node for name, got %v", item.Kind)
			}
			name = item.Value
			continue
		}

		opt, foundOpt := optionsByName[name]
		if foundOpt {
			if opt.ValueSource != ValueSourceNone {
				continue
			}
			opt.ValueSource = ValueSourceYAML
		}

		switch item.Kind {
		case yaml.MappingNode:
			// Item is either a group or an option with a complex object.
			if foundOpt {
				unmarshaler, ok := opt.Value.(yaml.Unmarshaler)
				if !ok {
					return xerrors.Errorf("complex option %q must support unmarshaling", opt.Name)
				}
				err := unmarshaler.UnmarshalYAML(item)
				if err != nil {
					merr = errors.Join(merr, xerrors.Errorf("unmarshal %q: %w", opt.Name, err))
				}
				continue
			}
			if g, ok := subGroupsByName[name]; ok {
				// Group, recurse.
				err := fromYAML(os, g, item)
				if err != nil {
					merr = errors.Join(merr, xerrors.Errorf("group %q: %w", g.YAML, err))
				}
				continue
			}
			merr = errors.Join(merr, xerrors.Errorf("unknown option or subgroup %q", name))
		case yaml.ScalarNode, yaml.SequenceNode:
			if !foundOpt {
				merr = errors.Join(merr, xerrors.Errorf("unknown option %q", name))
				continue
			}

			unmarshaler, _ := opt.Value.(yaml.Unmarshaler)
			switch {
			case unmarshaler == nil && item.Kind == yaml.ScalarNode:
				err := opt.Value.Set(item.Value)
				if err != nil {
					merr = errors.Join(merr, xerrors.Errorf("set %q: %w", opt.Name, err))
				}
			case unmarshaler == nil && item.Kind == yaml.SequenceNode:
				// Item is an option with a slice value.
				err := item.Decode(opt.Value)
				if err != nil {
					merr = errors.Join(merr, xerrors.Errorf("decode %q: %w", opt.Name, err))
				}
			case unmarshaler != nil:
				err := unmarshaler.UnmarshalYAML(item)
				if err != nil {
					merr = errors.Join(merr, xerrors.Errorf("unmarshal %q: %w", opt.Name, err))
				}
			default:
				panic("unreachable?")
			}
			continue

		default:
			return xerrors.Errorf("unexpected kind for value %v", item.Kind)
		}
	}
	return merr
}
