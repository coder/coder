package terraform

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"golang.org/x/xerrors"
)

// Terraform Core's address parser is internal, while go-terraform-address
// does not support all valid HCL identifiers, including Unicode. These helpers
// implement the address forms currently needed by Coder's Terraform
// provisioner: managed-resource addresses and module-instance paths, including
// count and for_each instance keys.
type terraformModulePath struct {
	raw   string
	steps []terraformModulePathStep
}

func (p terraformModulePath) String() string {
	return p.raw
}

type terraformModulePathStep struct {
	name        string
	instanceKey cty.Value
}

type terraformManagedResourceAddress struct {
	modulePath   terraformModulePath
	resourceType string
	resourceName string
	instanceKey  cty.Value
}

// parseTerraformManagedResourceAddress parses the absolute address of a
// managed resource. It accepts unindexed addresses and concrete count or
// for_each instance keys.
func parseTerraformManagedResourceAddress(raw string) (terraformManagedResourceAddress, error) {
	traversal, err := parseTerraformAddressTraversal(raw)
	if err != nil {
		return terraformManagedResourceAddress{}, err
	}

	modulePath, position, err := parseTerraformModulePathPrefix(raw, traversal)
	if err != nil {
		return terraformManagedResourceAddress{}, err
	}

	if position+1 >= len(traversal) {
		return terraformManagedResourceAddress{}, xerrors.New("resource address must contain a resource type and name")
	}
	resourceType, ok := terraformTraversalName(traversal[position])
	if !ok {
		return terraformManagedResourceAddress{}, xerrors.New("resource address must contain a resource type")
	}
	resourceName, ok := traversal[position+1].(hcl.TraverseAttr)
	if !ok {
		return terraformManagedResourceAddress{}, xerrors.New("resource type must be followed by a resource name")
	}
	position += 2

	instanceKey := cty.NilVal
	if position < len(traversal) {
		if index, ok := traversal[position].(hcl.TraverseIndex); ok {
			instanceKey, err = parseTerraformInstanceKey(index.Key)
			if err != nil {
				return terraformManagedResourceAddress{}, xerrors.Errorf(
					"parse resource %q instance key: %w", resourceName.Name, err,
				)
			}
			position++
		}
	}
	if position != len(traversal) {
		return terraformManagedResourceAddress{}, xerrors.New("resource address contains unsupported traversal steps")
	}

	return terraformManagedResourceAddress{
		modulePath:   modulePath,
		resourceType: resourceType,
		resourceName: resourceName.Name,
		instanceKey:  instanceKey,
	}, nil
}

// parseTerraformModulePath parses an absolute path containing only module
// calls. An empty path identifies the root module.
func parseTerraformModulePath(raw string) (terraformModulePath, error) {
	if raw == "" {
		return terraformModulePath{}, nil
	}

	traversal, err := parseTerraformAddressTraversal(raw)
	if err != nil {
		return terraformModulePath{}, err
	}

	path, position, err := parseTerraformModulePathPrefix(raw, traversal)
	if err != nil {
		return terraformModulePath{}, err
	}
	if position != len(traversal) {
		return terraformModulePath{}, xerrors.New("module path must contain only module calls")
	}
	return path, nil
}

func parseTerraformModulePathPrefix(
	raw string, traversal hcl.Traversal,
) (terraformModulePath, int, error) {
	var (
		pathEnd  int
		steps    []terraformModulePathStep
		position int
	)
	for position < len(traversal) {
		name, ok := terraformTraversalName(traversal[position])
		if !ok || name != "module" {
			break
		}
		if position+1 >= len(traversal) {
			return terraformModulePath{}, 0, xerrors.New("module prefix must be followed by a module name")
		}

		moduleName, ok := traversal[position+1].(hcl.TraverseAttr)
		if !ok {
			return terraformModulePath{}, 0, xerrors.New("module prefix must be followed by a module name")
		}
		position += 2

		instanceKey := cty.NilVal
		if position < len(traversal) {
			if index, ok := traversal[position].(hcl.TraverseIndex); ok {
				parsedInstanceKey, err := parseTerraformInstanceKey(index.Key)
				if err != nil {
					return terraformModulePath{}, 0, xerrors.Errorf("parse module %q instance key: %w", moduleName.Name, err)
				}
				instanceKey = parsedInstanceKey
				position++
			}
		}

		steps = append(steps, terraformModulePathStep{
			name:        moduleName.Name,
			instanceKey: instanceKey,
		})
		pathEnd = traversal[position-1].SourceRange().End.Byte
	}

	var path string
	if pathEnd > 0 {
		path = raw[:pathEnd]
	}
	return terraformModulePath{raw: path, steps: steps}, position, nil
}

func parseTerraformAddressTraversal(raw string) (hcl.Traversal, error) {
	traversal, diagnostics := hclsyntax.ParseTraversalAbs(
		[]byte(raw), "terraform-address", hcl.InitialPos,
	)
	if diagnostics.HasErrors() {
		return nil,
			xerrors.Errorf("invalid Terraform address syntax: %s", diagnostics.Error())
	}
	return traversal, nil
}

func terraformTraversalName(traverser hcl.Traverser) (string, bool) {
	switch traverser := traverser.(type) {
	case hcl.TraverseRoot:
		return traverser.Name, true
	case hcl.TraverseAttr:
		return traverser.Name, true
	default:
		return "", false
	}
}

func parseTerraformInstanceKey(key cty.Value) (cty.Value, error) {
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

func terraformInstanceKeysEqual(left, right cty.Value) bool {
	// cty does not permit operations on NilVal, so compare with the
	// sentinel before calling RawEquals.
	if left == cty.NilVal || right == cty.NilVal {
		return left == cty.NilVal && right == cty.NilVal
	}
	return left.RawEquals(right)
}
