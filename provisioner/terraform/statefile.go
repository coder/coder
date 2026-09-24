package terraform

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

// The types below describe the subset of Terraform's state file format
// (version 4, unchanged since Terraform 0.12) that Coder needs. Terraform
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
	Module    string        `json:"module"`
	Mode      string        `json:"mode"`
	Type      string        `json:"type"`
	Name      string        `json:"name"`
	Provider  string        `json:"provider"`
	Instances []rawInstance `json:"instances"`
}

type rawInstance struct {
	IndexKey      json.RawMessage            `json:"index_key"`
	SchemaVersion uint64                     `json:"schema_version"`
	Attributes    map[string]json.RawMessage `json:"attributes"`
	// AttributesFlat is the pre-0.12 flatmap encoding. Terraform rewrites it
	// on the first apply, so it is not expected here, but its presence means
	// the file needs Terraform itself to interpret.
	AttributesFlat map[string]string `json:"attributes_flat"`
	Dependencies   []string          `json:"dependencies"`
	Status         string            `json:"status"`
	Deposed        string            `json:"deposed"`
}

// readStateFile converts a Terraform state file into the structure that
// `terraform show -json` would produce for it. Only the fields Coder reads
// are populated; sensitive value markers and resource identities are
// omitted.
//
// One representational difference remains: attributes whose schema type is
// dynamic (cty.DynamicPseudoType, used for example by terraform_data.input
// and kubernetes_manifest.manifest) are stored in the file as a
// {"type": ..., "value": ...} wrapper that only the provider schema can
// identify, so they are returned in that wrapped form. Coder never reads
// such attributes: it decodes attributes of coder_* resources, which come
// from an SDKv2 provider without dynamic types, and a handful of plain
// string attributes on cloud instance resources.
func readStateFile(path string) (*tfjson.State, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// No state file means an empty state, which is what `terraform show
		// -json` reports as well.
		return &tfjson.State{FormatVersion: "1.0"}, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("read state file: %w", err)
	}
	return convertRawState(content)
}

func convertRawState(content []byte) (*tfjson.State, error) {
	var raw rawState
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, xerrors.Errorf("decode state file: %w", err)
	}
	if raw.Version != 4 {
		return nil, xerrors.Errorf("unsupported state file version %d", raw.Version)
	}

	state := &tfjson.State{
		FormatVersion:    "1.0",
		TerraformVersion: raw.TerraformVersion,
	}
	if len(raw.Resources) == 0 {
		// Terraform omits "values" entirely for an empty state.
		return state, nil
	}

	modules := map[string]*tfjson.StateModule{}
	module := func(addr string) *tfjson.StateModule {
		if m, ok := modules[addr]; ok {
			return m
		}
		m := &tfjson.StateModule{Address: addr}
		modules[addr] = m
		return m
	}
	root := module("")

	for _, r := range raw.Resources {
		mode, err := convertResourceMode(r.Mode)
		if err != nil {
			return nil, err
		}
		providerName, err := convertProviderName(r.Provider)
		if err != nil {
			return nil, xerrors.Errorf("resource %s.%s: %w", r.Type, r.Name, err)
		}
		mod := module(r.Module)
		for _, inst := range r.Instances {
			if inst.AttributesFlat != nil {
				return nil, xerrors.Errorf("resource %s.%s uses legacy flatmap attributes", r.Type, r.Name)
			}
			index, indexSuffix, err := convertIndexKey(inst.IndexKey)
			if err != nil {
				return nil, xerrors.Errorf("resource %s.%s: %w", r.Type, r.Name, err)
			}
			address := r.Type + "." + r.Name + indexSuffix
			if mode == tfjson.DataResourceMode {
				address = "data." + address
			}
			if r.Module != "" {
				address = r.Module + "." + address
			}
			values := make(map[string]interface{}, len(inst.Attributes))
			for k, v := range inst.Attributes {
				var decoded interface{}
				if err := json.Unmarshal(v, &decoded); err != nil {
					return nil, xerrors.Errorf("resource %s attribute %q: %w", address, k, err)
				}
				values[k] = decoded
			}
			mod.Resources = append(mod.Resources, &tfjson.StateResource{
				Address:         address,
				Mode:            mode,
				Type:            r.Type,
				Name:            r.Name,
				Index:           index,
				ProviderName:    providerName,
				SchemaVersion:   inst.SchemaVersion,
				AttributeValues: values,
				DependsOn:       inst.Dependencies,
				Tainted:         inst.Status == "tainted",
				DeposedKey:      inst.Deposed,
			})
		}
	}

	// Terraform creates an entry for every module on the path to a module
	// that holds resources, even when the intermediate module holds none,
	// and orders resources the way `terraform show -json` does.
	for addr := range modules {
		for parent := parentModule(addr); parent != ""; parent = parentModule(parent) {
			module(parent)
		}
	}
	for addr, m := range modules {
		sort.SliceStable(m.Resources, func(i, j int) bool {
			return resourceLess(m.Resources[i], m.Resources[j])
		})
		if addr == "" {
			continue
		}
		parent := module(parentModule(addr))
		parent.ChildModules = append(parent.ChildModules, m)
	}
	for _, m := range modules {
		sort.Slice(m.ChildModules, func(i, j int) bool {
			return m.ChildModules[i].Address < m.ChildModules[j].Address
		})
	}

	state.Values = &tfjson.StateValues{RootModule: root}
	return state, nil
}

// resourceLess mirrors the ordering `terraform show -json` uses within a
// module: data sources before managed resources, then by type and name,
// then instances without a key, integer keys, and string keys, with a
// deposed object following the current object of the same instance.
func resourceLess(a, b *tfjson.StateResource) bool {
	if a.Mode != b.Mode {
		return a.Mode == tfjson.DataResourceMode
	}
	if a.Type != b.Type {
		return a.Type < b.Type
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if ak, bk := indexKind(a.Index), indexKind(b.Index); ak != bk {
		return ak < bk
	}
	switch ai := a.Index.(type) {
	case float64:
		if bi, _ := b.Index.(float64); ai != bi {
			return ai < bi
		}
	case string:
		if bi, _ := b.Index.(string); ai != bi {
			return ai < bi
		}
	}
	if (a.DeposedKey == "") != (b.DeposedKey == "") {
		return a.DeposedKey == ""
	}
	return a.DeposedKey < b.DeposedKey
}

func indexKind(index interface{}) int {
	switch index.(type) {
	case nil:
		return 0
	case float64:
		return 1
	default:
		return 2
	}
}

func convertResourceMode(mode string) (tfjson.ResourceMode, error) {
	switch mode {
	case "managed":
		return tfjson.ManagedResourceMode, nil
	case "data":
		return tfjson.DataResourceMode, nil
	default:
		return "", xerrors.Errorf("unsupported resource mode %q", mode)
	}
}

// convertProviderName extracts the provider source address from a state
// file provider reference such as
// `provider["registry.terraform.io/coder/coder"]` or
// `module.m.provider["registry.terraform.io/coder/coder"].alias`.
func convertProviderName(ref string) (string, error) {
	const open = `provider["`
	start := strings.Index(ref, open)
	if start < 0 {
		return "", xerrors.Errorf("unsupported provider reference %q", ref)
	}
	rest := ref[start+len(open):]
	end := strings.Index(rest, `"]`)
	if end < 0 {
		return "", xerrors.Errorf("unsupported provider reference %q", ref)
	}
	return rest[:end], nil
}

// convertIndexKey returns the index value as `terraform show -json` reports
// it and the suffix it adds to the resource address.
func convertIndexKey(key json.RawMessage) (interface{}, string, error) {
	if len(key) == 0 || string(key) == "null" {
		return nil, "", nil
	}
	var index interface{}
	if err := json.Unmarshal(key, &index); err != nil {
		return nil, "", xerrors.Errorf("decode index key %s: %w", key, err)
	}
	switch v := index.(type) {
	case string:
		return v, "[" + strconv.Quote(v) + "]", nil
	case float64:
		return v, "[" + strconv.FormatInt(int64(v), 10) + "]", nil
	default:
		return nil, "", xerrors.Errorf("unsupported index key %s", key)
	}
}

// parentModule returns the address of the module containing the given
// module instance, or "" for a child of the root module. Module instance
// keys may contain arbitrary strings, so the address is scanned rather than
// split on ".module.".
func parentModule(addr string) string {
	inQuote := false
	last := -1
	for i := 0; i < len(addr); i++ {
		switch addr[i] {
		case '\\':
			if inQuote {
				i++
			}
		case '"':
			inQuote = !inQuote
		case '.':
			if !inQuote && strings.HasPrefix(addr[i+1:], "module.") {
				last = i
			}
		}
	}
	if last < 0 {
		return ""
	}
	return addr[:last]
}
