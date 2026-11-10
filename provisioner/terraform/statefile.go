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

// The types below describe the subset of Terraform's state file format
// (version 4, in use since Terraform 0.12) that Coder needs. Terraform
// writes terraform.tfstate in this format at the end of apply, and
// `terraform show -json` merely reshapes it, at the cost of starting
// Terraform and every provider again to fetch schemas. Reading the file
// directly is equivalent as long as the schema versions in the file match
// the providers that just wrote it, which is always the case right after
// apply.

type rawState struct {
	Version          int           `json:"version"`
	TerraformVersion string        `json:"terraform_version"`
	Resources        []rawResource `json:"resources"`
}

type rawResource struct {
	// Module is the module instance address, empty for the root module.
	Module    string              `json:"module"`
	Mode      tfjson.ResourceMode `json:"mode"`
	Type      string              `json:"type"`
	Name      string              `json:"name"`
	Instances []rawInstance       `json:"instances"`
}

type rawInstance struct {
	// IndexKey is a number for count, a string for for_each, or absent.
	IndexKey   any            `json:"index_key"`
	Attributes map[string]any `json:"attributes"`
	// AttributesFlat is the pre-0.12 flatmap encoding. Terraform rewrites it
	// on the first apply, so it is not expected here, but its presence means
	// the file needs Terraform itself to interpret.
	AttributesFlat map[string]string `json:"attributes_flat"`
	Dependencies   []string          `json:"dependencies"`
	Status         string            `json:"status"`
	Deposed        string            `json:"deposed"`
}

// readStateFile converts a Terraform state file into the structure
// `terraform show -json` produces for it. It leaves out what Coder doesn't
// read: provider names, schema versions, sensitive value markers, resource
// identities, outputs, and checks.
//
// Attributes with a dynamic schema type, such as terraform_data.input, keep
// the {"type", "value"} wrapper the file stores them in, because only the
// provider schema identifies them. Coder doesn't read such attributes: the
// coder provider is built on plugin SDK v2, which has no dynamic types, and
// the attributes read from other providers' resources are plain strings.
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

func parseStateFile(content []byte) (*tfjson.State, error) {
	var raw rawState
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, xerrors.Errorf("decode state file: %w", err)
	}
	if raw.Version != 4 {
		return nil, xerrors.Errorf("unsupported state file version %d", raw.Version)
	}
	tfVersion, err := version.NewVersion(raw.TerraformVersion)
	if err != nil {
		return nil, xerrors.Errorf("parse terraform version %q: %w", raw.TerraformVersion, err)
	}
	if !stateFileTerraformVersionSupported(tfVersion) {
		return nil, xerrors.Errorf("%w %s", errStateFileTerraformVersion, tfVersion)
	}
	if len(raw.Resources) == 0 {
		// Terraform reports an empty state without a version or values.
		// Root outputs alone would add an empty root module, which Graph
		// treats the same.
		return &tfjson.State{FormatVersion: "1.0"}, nil
	}

	root := &tfjson.StateModule{}
	modules := map[string]*tfjson.StateModule{"": root}
	// module returns the entry for a module address. Like `terraform show
	// -json`, it gives every module on the path to one with resources an
	// entry, even when that module holds none itself.
	var module func(addr string) (*tfjson.StateModule, error)
	module = func(addr string) (*tfjson.StateModule, error) {
		if m, ok := modules[addr]; ok {
			return m, nil
		}
		path, err := tfaddr.ParseModulePath(addr)
		if err != nil {
			return nil, xerrors.Errorf("parse module address %q: %w", addr, err)
		}
		parent, err := module(path.Parent().String())
		if err != nil {
			return nil, err
		}
		m := &tfjson.StateModule{Address: addr}
		modules[addr] = m
		parent.ChildModules = append(parent.ChildModules, m)
		return m, nil
	}

	// Terraform writes resources in the order `terraform show -json` lists
	// them within a module, so they keep the file's order.
	for _, r := range raw.Resources {
		address := r.Type + "." + r.Name
		switch r.Mode {
		case tfjson.ManagedResourceMode:
		case tfjson.DataResourceMode:
			address = "data." + address
		default:
			return nil, xerrors.Errorf("resource %s: unsupported mode %q", address, r.Mode)
		}
		mod, err := module(r.Module)
		if err != nil {
			return nil, err
		}
		if r.Module != "" {
			address = r.Module + "." + address
		}
		for _, inst := range r.Instances {
			if inst.AttributesFlat != nil {
				return nil, xerrors.Errorf("resource %s uses legacy flatmap attributes", address)
			}
			key, err := instanceKey(inst.IndexKey)
			if err != nil {
				return nil, xerrors.Errorf("resource %s: %w", address, err)
			}
			mod.Resources = append(mod.Resources, &tfjson.StateResource{
				Address:         address + key,
				Mode:            r.Mode,
				Type:            r.Type,
				Name:            r.Name,
				Index:           inst.IndexKey,
				AttributeValues: inst.Attributes,
				DependsOn:       inst.Dependencies,
				Tainted:         inst.Status == "tainted",
				DeposedKey:      inst.Deposed,
			})
		}
	}

	// `terraform show -json` orders child modules by address string, so [10]
	// sorts before [2]. The file's order can differ: a module without
	// resources of its own first appears with a descendant, and
	// module.a.module.b follows module.a-c.
	for _, m := range modules {
		slices.SortFunc(m.ChildModules, func(a, b *tfjson.StateModule) int {
			return cmp.Compare(a.Address, b.Address)
		})
	}

	return &tfjson.State{
		FormatVersion:    "1.0",
		TerraformVersion: raw.TerraformVersion,
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
