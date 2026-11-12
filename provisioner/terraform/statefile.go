package terraform

import (
	"cmp"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strconv"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2/hclwrite"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
)

// minStateFileTerraformVersion is the oldest Terraform whose state file
// readStateFile converts. Before 1.4, `terraform show -json` sorted resources
// by address, and before 1.3 it quoted instance keys with Go syntax. It can go
// once minTerraformVersion reaches it.
var minStateFileTerraformVersion = version.Must(version.NewVersion("1.4.0"))

// errStateFileTerraformVersion reports a state file from a Terraform version
// whose `terraform show -json` output readStateFile is not known to
// reproduce.
var errStateFileTerraformVersion = xerrors.New("unsupported terraform version")

// stateFile is the part of a Terraform state file that Coder reads. The
// format is version 4, which Terraform has written since 0.12.
type stateFile struct {
	Version          int                 `json:"version"`
	TerraformVersion string              `json:"terraform_version"`
	Resources        []stateFileResource `json:"resources"`
}

type stateFileResource struct {
	// Module is the address of the module instance that holds the resource,
	// empty for the root module.
	Module    string              `json:"module"`
	Mode      tfjson.ResourceMode `json:"mode"`
	Type      string              `json:"type"`
	Name      string              `json:"name"`
	Instances []stateFileInstance `json:"instances"`
}

type stateFileInstance struct {
	// IndexKey is a number for count, a string for for_each, and absent
	// otherwise.
	IndexKey   any            `json:"index_key"`
	Attributes map[string]any `json:"attributes"`
	// AttributesFlat is the encoding from before 0.12, which only Terraform
	// can interpret. Terraform replaces it on the first apply.
	AttributesFlat map[string]string `json:"attributes_flat"`
	Dependencies   []string          `json:"dependencies"`
	Status         string            `json:"status"`
	Deposed        string            `json:"deposed"`
}

// readStateFile converts the state file at path into what `terraform show
// -json` prints for it, without starting Terraform and the providers. The two
// agree when the providers that wrote the file are the ones show would start,
// as they are right after apply. Otherwise show would first upgrade the
// resources to those providers' schemas.
//
// It leaves out what Coder doesn't read: provider names, schema versions,
// sensitive value markers, resource identities, outputs, and checks.
// Attributes with a dynamic schema type, such as terraform_data.input, keep
// the {"type", "value"} wrapper the file stores them in, because only the
// provider schema identifies them. Coder reads no such attribute: the coder
// provider is built on plugin SDK v2, which has no dynamic types, and what
// Coder reads from other providers' resources are plain strings.
func readStateFile(path string) (*tfjson.State, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// No state file means an empty state.
		return &tfjson.State{FormatVersion: "1.0"}, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("read state file: %w", err)
	}
	return parseStateFile(content)
}

// parseStateFile is readStateFile for the content of a state file.
func parseStateFile(content []byte) (*tfjson.State, error) {
	var file stateFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, xerrors.Errorf("decode state file: %w", err)
	}
	if file.Version != 4 {
		return nil, xerrors.Errorf("unsupported state file version %d", file.Version)
	}
	tfVersion, err := version.NewVersion(file.TerraformVersion)
	if err != nil {
		return nil, xerrors.Errorf("parse terraform version %q: %w", file.TerraformVersion, err)
	}
	if !stateFileTerraformVersionSupported(tfVersion) {
		return nil, xerrors.Errorf("%w %s", errStateFileTerraformVersion, tfVersion)
	}
	if len(file.Resources) == 0 {
		// `terraform show -json` prints no version or values for an empty
		// state. For a state that only has outputs it prints an empty root
		// module, which Graph treats the same.
		return &tfjson.State{FormatVersion: "1.0"}, nil
	}

	root := &tfjson.StateModule{}
	modules := map[string]*tfjson.StateModule{"": root}
	// Within a module, the file lists resources in the order `terraform show
	// -json` does.
	for _, resource := range file.Resources {
		module, err := stateModule(modules, resource.Module)
		if err != nil {
			return nil, err
		}
		resources, err := resource.stateResources()
		if err != nil {
			return nil, err
		}
		module.Resources = append(module.Resources, resources...)
	}
	// Child modules are not in that order. `terraform show -json` sorts them
	// by address, as strings, so [10] comes before [2].
	for _, module := range modules {
		slices.SortFunc(module.ChildModules, func(a, b *tfjson.StateModule) int {
			return cmp.Compare(a.Address, b.Address)
		})
	}

	return &tfjson.State{
		FormatVersion:    "1.0",
		TerraformVersion: file.TerraformVersion,
		Values:           &tfjson.StateValues{RootModule: root},
	}, nil
}

// stateFileTerraformVersionSupported reports whether readStateFile converts
// state written by the given Terraform version. Versions from
// maxTerraformVersion on are left to `terraform show`, because nothing has
// compared their output with readStateFile's yet.
func stateFileTerraformVersionSupported(v *version.Version) bool {
	return v.GreaterThanOrEqual(minStateFileTerraformVersion) && v.LessThan(maxTerraformVersion)
}

// stateModule returns the module at address, adding it to modules if it is
// missing. Like `terraform show -json`, it also adds every module on the way
// there from the root, whether or not that module holds resources itself.
func stateModule(modules map[string]*tfjson.StateModule, address string) (*tfjson.StateModule, error) {
	if module, ok := modules[address]; ok {
		return module, nil
	}
	path, err := tfaddr.ParseModulePath(address)
	if err != nil {
		return nil, xerrors.Errorf("parse module address %q: %w", address, err)
	}
	parent, err := stateModule(modules, path.Parent().String())
	if err != nil {
		return nil, err
	}
	module := &tfjson.StateModule{Address: address}
	modules[address] = module
	parent.ChildModules = append(parent.ChildModules, module)
	return module, nil
}

// address returns the resource's absolute address, without an instance key.
func (r stateFileResource) address() (string, error) {
	address := r.Type + "." + r.Name
	switch r.Mode {
	case tfjson.ManagedResourceMode:
	case tfjson.DataResourceMode:
		address = "data." + address
	default:
		return "", xerrors.Errorf("resource %s: unsupported mode %q", address, r.Mode)
	}
	if r.Module != "" {
		address = r.Module + "." + address
	}
	return address, nil
}

// stateResources returns what `terraform show -json` lists for the resource:
// one entry for each of its instances, including deposed ones.
func (r stateFileResource) stateResources() ([]*tfjson.StateResource, error) {
	address, err := r.address()
	if err != nil {
		return nil, err
	}
	resources := make([]*tfjson.StateResource, 0, len(r.Instances))
	for _, instance := range r.Instances {
		if instance.AttributesFlat != nil {
			return nil, xerrors.Errorf("resource %s uses legacy flatmap attributes", address)
		}
		key, err := instanceKey(instance.IndexKey)
		if err != nil {
			return nil, xerrors.Errorf("resource %s: %w", address, err)
		}
		resources = append(resources, &tfjson.StateResource{
			Address:         address + key,
			Mode:            r.Mode,
			Type:            r.Type,
			Name:            r.Name,
			Index:           instance.IndexKey,
			AttributeValues: instance.Attributes,
			DependsOn:       instance.Dependencies,
			Tainted:         instance.Status == "tainted",
			DeposedKey:      instance.Deposed,
		})
	}
	return resources, nil
}

// instanceKey returns the suffix that an instance's index key adds to its
// resource address.
func instanceKey(key any) (string, error) {
	switch key := key.(type) {
	case nil:
		return "", nil
	case float64:
		return "[" + strconv.FormatInt(int64(key), 10) + "]", nil
	case string:
		// Terraform quotes string keys the way hclwrite does.
		return "[" + string(hclwrite.TokensForValue(cty.StringVal(key)).Bytes()) + "]", nil
	default:
		return "", xerrors.Errorf("unsupported index key %v", key)
	}
}
