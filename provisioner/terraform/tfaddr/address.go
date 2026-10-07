// Package tfaddr parses the Terraform address forms Coder's Terraform
// provisioner needs: managed-resource addresses, module-instance
// paths, and configuration references. It supports concrete count and
// for_each instance keys, but not every Terraform address form.
//
// Terraform Core's address parser is internal, and go-terraform-address
// does not support all valid HCL identifiers, including Unicode.
package tfaddr

import (
	"iter"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"golang.org/x/xerrors"
)

const maxConfigurationReferenceAddressParts = 256

// ManagedResourceAddress is an absolute (rooted at the Terraform root
// module) managed-resource address.
type ManagedResourceAddress struct {
	modulePath   ModulePath
	resourceType string
	resourceName string
	instanceKey  cty.Value
}

// ModulePath returns the evaluated module path containing the resource.
func (a ManagedResourceAddress) ModulePath() ModulePath {
	return a.modulePath
}

// ResourceType returns the resource type.
func (a ManagedResourceAddress) ResourceType() string {
	return a.resourceType
}

// ResourceName returns the resource name.
func (a ManagedResourceAddress) ResourceName() string {
	return a.resourceName
}

// InstanceKey returns the count or for_each instance key. An unindexed
// resource returns cty.NilVal.
func (a ManagedResourceAddress) InstanceKey() cty.Value {
	return a.instanceKey
}

// ConfigurationAddress returns the address without module or resource
// instance keys.
func (a ManagedResourceAddress) ConfigurationAddress() string {
	var address strings.Builder
	if moduleAddress := a.modulePath.ConfigurationAddress(); moduleAddress != "" {
		_, _ = address.WriteString(moduleAddress)
		_ = address.WriteByte('.')
	}
	_, _ = address.WriteString(a.resourceType)
	_ = address.WriteByte('.')
	_, _ = address.WriteString(a.resourceName)
	return address.String()
}

// ModulePath is an absolute evaluated path containing only module
// calls. The zero value identifies the root module.
type ModulePath struct {
	raw   string
	steps []ModuleStep
}

// String returns the evaluated module path, including instance keys.
func (p ModulePath) String() string {
	return p.raw
}

// ConfigurationAddress returns the module path without instance keys.
func (p ModulePath) ConfigurationAddress() string {
	var address strings.Builder
	for index, step := range p.steps {
		if index > 0 {
			_ = address.WriteByte('.')
		}
		_, _ = address.WriteString("module.")
		_, _ = address.WriteString(step.name)
	}
	return address.String()
}

// Steps returns a copy of the ordered module steps.
func (p ModulePath) Steps() []ModuleStep {
	return slices.Clone(p.steps)
}

// ModuleStep is one evaluated module call in a ModulePath.
type ModuleStep struct {
	name        string
	instanceKey cty.Value
}

// Name returns the module call name.
func (s ModuleStep) Name() string {
	return s.name
}

// InstanceKey returns the count or for_each instance key. An unindexed module
// call returns cty.NilVal.
func (s ModuleStep) InstanceKey() cty.Value {
	return s.instanceKey
}

// ConfigurationReference represents a reference to a Terraform value
// with instance keys removed.
type ConfigurationReference struct {
	address            string
	partEndByteOffsets []int
	// For a module reference, moduleOutputAddress caches the Terraform graph
	// address of its named output. It is empty for other references.
	moduleOutputAddress string
}

// ConfigurationAddress returns the reference without instance keys.
func (r ConfigurationReference) ConfigurationAddress() string {
	return r.address
}

// ConfigurationAddresses returns possible Terraform graph configuration
// addresses for the reference, prefixed by declaringModuleAddress. A
// child-module output yields its graph-specific address. Other references
// yield traversal prefixes from most to least specific.
// The declaring module address must not contain instance keys; use an empty
// string for the root module.
func (r ConfigurationReference) ConfigurationAddresses(
	declaringModuleAddress string,
) iter.Seq[string] {
	if r.address == "" {
		return func(func(string) bool) {}
	}

	return func(yield func(string) bool) {
		// Terraform plan graph nodes name child-module outputs as
		// module.<name>.output.<output>, while references omit "output".
		if r.moduleOutputAddress != "" {
			address := qualifyConfigurationAddress(
				declaringModuleAddress, r.moduleOutputAddress,
			)
			yield(address)
			return
		}

		address := qualifyConfigurationAddress(declaringModuleAddress, r.address)
		prefixLength := 0
		if declaringModuleAddress != "" {
			prefixLength = len(declaringModuleAddress) + 1
		}
		for end := len(r.partEndByteOffsets) - 1; end >= 1; end-- {
			if !yield(address[:prefixLength+r.partEndByteOffsets[end]]) {
				return
			}
		}
	}
}

// ParseManagedResourceAddress parses an absolute managed-resource
// address. It accepts unindexed addresses and concrete count or
// for_each instance keys.
func ParseManagedResourceAddress(raw string) (ManagedResourceAddress, error) {
	traversal, err := parseAddressTraversal(raw)
	if err != nil {
		return ManagedResourceAddress{}, err
	}

	modulePath, position, err := parseModulePathPrefix(raw, traversal)
	if err != nil {
		return ManagedResourceAddress{}, err
	}

	if position+1 >= len(traversal) {
		return ManagedResourceAddress{}, xerrors.New("resource address must contain a resource type and name")
	}
	resourceType, ok := traversalName(traversal[position])
	if !ok {
		return ManagedResourceAddress{}, xerrors.New("resource address must contain a resource type")
	}
	resourceName, ok := traversal[position+1].(hcl.TraverseAttr)
	if !ok {
		return ManagedResourceAddress{}, xerrors.New("resource type must be followed by a resource name")
	}
	position += 2

	instanceKey := cty.NilVal
	if position < len(traversal) {
		if index, ok := traversal[position].(hcl.TraverseIndex); ok {
			instanceKey, err = parseInstanceKey(index.Key)
			if err != nil {
				return ManagedResourceAddress{}, xerrors.Errorf(
					"parse resource %q instance key: %w", resourceName.Name, err,
				)
			}
			position++
		}
	}
	if position != len(traversal) {
		return ManagedResourceAddress{}, xerrors.New("resource address contains unsupported traversal steps")
	}

	return ManagedResourceAddress{
		modulePath:   modulePath,
		resourceType: resourceType,
		resourceName: resourceName.Name,
		instanceKey:  instanceKey,
	}, nil
}

// ParseModulePath parses an absolute path containing only module
// calls. An empty path identifies the root module.
func ParseModulePath(raw string) (ModulePath, error) {
	if raw == "" {
		return ModulePath{}, nil
	}

	traversal, err := parseAddressTraversal(raw)
	if err != nil {
		return ModulePath{}, err
	}

	path, position, err := parseModulePathPrefix(raw, traversal)
	if err != nil {
		return ModulePath{}, err
	}
	if position != len(traversal) {
		return ModulePath{}, xerrors.New("module path must contain only module calls")
	}
	return path, nil
}

// ParseConfigurationReference parses a Terraform value reference for
// configuration-address lookup.
func ParseConfigurationReference(raw string) (ConfigurationReference, error) {
	traversal, err := parseAddressTraversal(raw)
	if err != nil {
		return ConfigurationReference{},
			xerrors.Errorf("parse Terraform reference: %w", err)
	}

	parts := make([]string, 0, len(traversal))
	for _, traverser := range traversal {
		name, ok := traversalName(traverser)
		if ok {
			parts = append(parts, name)
			continue
		}
		if _, ok := traverser.(hcl.TraverseIndex); ok {
			continue
		}
		return ConfigurationReference{},
			xerrors.New("reference contains unsupported traversal steps")
	}
	if len(parts) > maxConfigurationReferenceAddressParts {
		return ConfigurationReference{}, xerrors.Errorf(
			"reference contains more than %d address parts",
			maxConfigurationReferenceAddressParts,
		)
	}
	if len(parts) < 2 {
		return ConfigurationReference{}, nil
	}

	var address strings.Builder
	partEndByteOffsets := make([]int, 0, len(parts))
	for index, part := range parts {
		if index > 0 {
			_ = address.WriteByte('.')
		}
		_, _ = address.WriteString(part)
		partEndByteOffsets = append(partEndByteOffsets, address.Len())
	}

	var moduleOutputAddress string
	if len(parts) >= 3 && parts[0] == "module" {
		moduleOutputAddress = strings.Join(
			[]string{parts[0], parts[1], "output", parts[2]}, ".",
		)
	}
	return ConfigurationReference{
		address:             address.String(),
		partEndByteOffsets:  partEndByteOffsets,
		moduleOutputAddress: moduleOutputAddress,
	}, nil
}

// InstanceKeysEqual reports whether two parsed instance keys are equal.
func InstanceKeysEqual(left, right cty.Value) bool {
	// cty does not permit operations on NilVal, so compare with the
	// sentinel before calling RawEquals.
	if left == cty.NilVal || right == cty.NilVal {
		return left == cty.NilVal && right == cty.NilVal
	}
	return left.RawEquals(right)
}

func parseModulePathPrefix(
	raw string, traversal hcl.Traversal,
) (ModulePath, int, error) {
	var (
		pathEnd  int
		steps    []ModuleStep
		position int
	)
	for position < len(traversal) {
		name, ok := traversalName(traversal[position])
		if !ok || name != "module" {
			break
		}
		if position+1 >= len(traversal) {
			return ModulePath{}, 0, xerrors.New("module prefix must be followed by a module name")
		}

		moduleName, ok := traversal[position+1].(hcl.TraverseAttr)
		if !ok {
			return ModulePath{}, 0, xerrors.New("module prefix must be followed by a module name")
		}
		position += 2

		instanceKey := cty.NilVal
		if position < len(traversal) {
			if index, ok := traversal[position].(hcl.TraverseIndex); ok {
				parsedInstanceKey, err := parseInstanceKey(index.Key)
				if err != nil {
					return ModulePath{}, 0, xerrors.Errorf(
						"parse module %q instance key: %w", moduleName.Name, err,
					)
				}
				instanceKey = parsedInstanceKey
				position++
			}
		}

		steps = append(steps, ModuleStep{
			name:        moduleName.Name,
			instanceKey: instanceKey,
		})
		pathEnd = traversal[position-1].SourceRange().End.Byte
	}

	var path string
	if pathEnd > 0 {
		path = raw[:pathEnd]
	}
	return ModulePath{raw: path, steps: steps}, position, nil
}

func parseAddressTraversal(raw string) (hcl.Traversal, error) {
	traversal, diagnostics := hclsyntax.ParseTraversalAbs(
		[]byte(raw), "terraform-address", hcl.InitialPos,
	)
	if diagnostics.HasErrors() {
		return nil,
			xerrors.Errorf("invalid Terraform address syntax: %s", diagnostics.Error())
	}
	return traversal, nil
}

func traversalName(traverser hcl.Traverser) (string, bool) {
	switch traverser := traverser.(type) {
	case hcl.TraverseRoot:
		return traverser.Name, true
	case hcl.TraverseAttr:
		return traverser.Name, true
	default:
		return "", false
	}
}

func parseInstanceKey(key cty.Value) (cty.Value, error) {
	switch key.Type() {
	case cty.String:
		return key, nil
	case cty.Number:
		var index int
		if err := gocty.FromCtyValue(key, &index); err != nil {
			return cty.NilVal, xerrors.Errorf("instance key must be an integer: %w", err)
		}
		if index < 0 {
			return cty.NilVal, xerrors.New("instance key must not be negative")
		}
		// Normalize the validated integer because equivalent HCL
		// numbers can have different internal representations and
		// fail structural test comparisons.
		return cty.NumberIntVal(int64(index)), nil
	default:
		return cty.NilVal, xerrors.New("instance key must be a string or integer")
	}
}

func qualifyConfigurationAddress(prefix, address string) string {
	if prefix == "" {
		return address
	}
	return prefix + "." + address
}
