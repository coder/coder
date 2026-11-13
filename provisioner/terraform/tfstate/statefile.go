// Package tfstate reads Terraform state files without starting Terraform and
// the providers.
package tfstate

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

// minTerraformVersion is the oldest Terraform whose state file Read converts.
// Before 1.4, `terraform show -json` sorted resources by address, and before
// 1.3 it quoted instance keys with Go syntax. It can go once the provisioner
// requires that version.
var minTerraformVersion = version.Must(version.NewVersion("1.4.0"))

// stateFileFormatVersion is the format Terraform has written since 0.12.
const stateFileFormatVersion = 4

// stateFile is the part of a Terraform state file that Coder reads.
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

// Read returns the root module that `terraform show -json` reports for the
// state file at path, without starting Terraform and the providers. It is
// the only element of the result, which is empty for a state without
// resources. The two agree when the providers that wrote the file are the
// ones show would start, as they are right after apply. Otherwise show would
// first upgrade the resources to those providers' schemas.
//
// Read returns an error for a state file it can't convert, such as one
// written by a Terraform version that SupportsTerraformVersion rejects.
//
// Resources lack what Coder doesn't read: provider names, schema versions,
// sensitive value markers, and identities. Attributes with a dynamic schema
// type, such as terraform_data.input, keep the {"type", "value"} wrapper the
// file stores them in, because only the provider schema identifies them.
// Coder reads no such attribute: the coder provider is built on plugin SDK
// v2, which has no dynamic types, and what Coder reads from other providers'
// resources are plain strings.
func Read(path string, maxVersion *version.Version) ([]*tfjson.StateModule, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// `terraform show -json` also reports a missing state file as an
		// empty state.
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("read state file: %w", err)
	}
	return parseStateFile(content, maxVersion)
}

// parseStateFile is Read for the content of a state file.
func parseStateFile(content []byte, maxVersion *version.Version) ([]*tfjson.StateModule, error) {
	var file stateFile
	if err := json.Unmarshal(content, &file); err != nil {
		return nil, xerrors.Errorf("decode state file: %w", err)
	}
	if file.Version != stateFileFormatVersion {
		return nil, xerrors.Errorf("unsupported state file version %d", file.Version)
	}
	tfVersion, err := version.NewVersion(file.TerraformVersion)
	if err != nil {
		return nil, xerrors.Errorf("parse terraform version %q: %w", file.TerraformVersion, err)
	}
	if !SupportsTerraformVersion(tfVersion, maxVersion) {
		return nil, xerrors.Errorf("unsupported state file: written by terraform %s", tfVersion)
	}
	if len(file.Resources) == 0 {
		return nil, nil
	}

	root := &tfjson.StateModule{}
	modules := map[string]*tfjson.StateModule{"": root}
	// Within a module, the file lists resources in the order `terraform show
	// -json` does.
	for _, resource := range file.Resources {
		path, err := tfaddr.ParseModulePath(resource.Module)
		if err != nil {
			return nil, xerrors.Errorf("parse module address %q: %w", resource.Module, err)
		}
		resources, err := resource.stateResources()
		if err != nil {
			return nil, err
		}
		module := stateModule(modules, path)
		module.Resources = append(module.Resources, resources...)
	}
	// Child modules are not in that order. `terraform show -json` sorts them
	// by address, as strings, so [10] comes before [2].
	for _, module := range modules {
		slices.SortFunc(module.ChildModules, func(a, b *tfjson.StateModule) int {
			return cmp.Compare(a.Address, b.Address)
		})
	}
	return []*tfjson.StateModule{root}, nil
}

// SupportsTerraformVersion reports whether Read converts state written by
// Terraform version v. Versions from maxVersion on are left to `terraform
// show`, because nothing has compared their output with Read's yet.
func SupportsTerraformVersion(v, maxVersion *version.Version) bool {
	return v.GreaterThanOrEqual(minTerraformVersion) && v.LessThan(maxVersion)
}

// stateModule returns the module at path, adding it to modules if it is
// missing. Like `terraform show -json`, it also adds every module on the way
// there from the root, whether or not that module holds resources itself.
func stateModule(modules map[string]*tfjson.StateModule, path tfaddr.ModulePath) *tfjson.StateModule {
	address := path.String()
	if module, ok := modules[address]; ok {
		return module
	}
	module := &tfjson.StateModule{Address: address}
	modules[address] = module
	parent := stateModule(modules, path.Parent())
	parent.ChildModules = append(parent.ChildModules, module)
	return module
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
		suffix, err := instanceKeySuffix(instance.IndexKey)
		if err != nil {
			return nil, xerrors.Errorf("resource %s: %w", address, err)
		}
		resources = append(resources, &tfjson.StateResource{
			Address:         address + suffix,
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

// instanceKeySuffix returns what an instance's index key adds to its
// resource address.
func instanceKeySuffix(key any) (string, error) {
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
