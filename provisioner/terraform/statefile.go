package terraform

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisioner/terraform/tfaddr"
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
	ensureModule := func(addr string) *tfjson.StateModule {
		if m, ok := modules[addr]; ok {
			return m
		}
		m := &tfjson.StateModule{Address: addr}
		modules[addr] = m
		return m
	}
	root := ensureModule("")

	for _, r := range raw.Resources {
		mode, err := convertResourceMode(r.Mode)
		if err != nil {
			return nil, xerrors.Errorf("resource %s.%s: %w", r.Type, r.Name, err)
		}
		providerName, err := convertProviderName(r.Provider)
		if err != nil {
			return nil, xerrors.Errorf("resource %s.%s: %w", r.Type, r.Name, err)
		}
		mod := ensureModule(r.Module)
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
			values := make(map[string]any, len(inst.Attributes))
			for k, v := range inst.Attributes {
				var decoded any
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

	// Like `terraform show -json`, create an entry for every module on the
	// path to one with resources, even when it holds none itself, and sort
	// resources within each module.
	parents := make(map[string]string, len(modules))
	pending := slices.Collect(maps.Keys(modules))
	for len(pending) > 0 {
		addr := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if addr == "" {
			continue
		}
		parent, err := parentModule(addr)
		if err != nil {
			return nil, err
		}
		parents[addr] = parent
		if _, ok := modules[parent]; !ok {
			ensureModule(parent)
			pending = append(pending, parent)
		}
	}
	for addr, m := range modules {
		slices.SortStableFunc(m.Resources, compareResources)
		if addr == "" {
			continue
		}
		parent := modules[parents[addr]]
		parent.ChildModules = append(parent.ChildModules, m)
	}
	for _, m := range modules {
		slices.SortFunc(m.ChildModules, func(a, b *tfjson.StateModule) int {
			return cmp.Compare(a.Address, b.Address)
		})
	}

	state.Values = &tfjson.StateValues{RootModule: root}
	return state, nil
}

// compareResources mirrors the ordering `terraform show -json` uses within
// a module: data sources before managed resources, then by type and name,
// then instances without a key, integer keys, and string keys, with a
// deposed object following the current object of the same instance.
func compareResources(a, b *tfjson.StateResource) int {
	return cmp.Or(
		cmp.Compare(modeRank(a.Mode), modeRank(b.Mode)),
		cmp.Compare(a.Type, b.Type),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(indexKind(a.Index), indexKind(b.Index)),
		compareIndex(a.Index, b.Index),
		cmp.Compare(a.DeposedKey, b.DeposedKey),
	)
}

func modeRank(mode tfjson.ResourceMode) int {
	if mode == tfjson.DataResourceMode {
		return 0
	}
	return 1
}

func indexKind(index any) int {
	switch index.(type) {
	case nil:
		return 0
	case float64:
		return 1
	default:
		return 2
	}
}

// compareIndex compares two index keys of the same kind.
func compareIndex(a, b any) int {
	switch av := a.(type) {
	case float64:
		bv, _ := b.(float64)
		return cmp.Compare(av, bv)
	case string:
		bv, _ := b.(string)
		return cmp.Compare(av, bv)
	default:
		return 0
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
	_, rest, ok := strings.Cut(ref, `provider["`)
	if !ok {
		return "", xerrors.Errorf("unsupported provider reference %q", ref)
	}
	name, _, ok := strings.Cut(rest, `"]`)
	if !ok {
		return "", xerrors.Errorf("unsupported provider reference %q", ref)
	}
	return name, nil
}

// convertIndexKey returns the index value as `terraform show -json` reports
// it and the suffix it adds to the resource address.
func convertIndexKey(key json.RawMessage) (any, string, error) {
	if len(key) == 0 || string(key) == "null" {
		return nil, "", nil
	}
	var index any
	if err := json.Unmarshal(key, &index); err != nil {
		return nil, "", xerrors.Errorf("decode index key %s: %w", key, err)
	}
	switch v := index.(type) {
	case string:
		return v, "[" + hclQuotedString(v) + "]", nil
	case float64:
		return v, "[" + strconv.FormatInt(int64(v), 10) + "]", nil
	default:
		return nil, "", xerrors.Errorf("unsupported index key %s", key)
	}
}

// hclQuotedString quotes s the way Terraform writes string instance keys in
// resource addresses, which differs from Go quoting: template introducers are
// doubled and other non-printable characters use \u escapes. It mirrors
// toHCLQuotedString in Terraform's internal/addrs package.
func hclQuotedString(s string) string {
	var buf strings.Builder
	_ = buf.WriteByte('"')
	for i, r := range s {
		switch r {
		case '\n':
			_, _ = buf.WriteString(`\n`)
		case '\r':
			_, _ = buf.WriteString(`\r`)
		case '\t':
			_, _ = buf.WriteString(`\t`)
		case '"':
			_, _ = buf.WriteString(`\"`)
		case '\\':
			_, _ = buf.WriteString(`\\`)
		case '$', '%':
			_, _ = buf.WriteRune(r)
			if strings.HasPrefix(s[i+1:], "{") {
				// Double the template introducer to escape it.
				_, _ = buf.WriteRune(r)
			}
		default:
			switch {
			case unicode.IsPrint(r):
				_, _ = buf.WriteRune(r)
			case r < 0x10000:
				_, _ = fmt.Fprintf(&buf, "\\u%04x", r)
			default:
				_, _ = fmt.Fprintf(&buf, "\\U%08x", r)
			}
		}
	}
	_ = buf.WriteByte('"')
	return buf.String()
}

// parentModule returns the address of the module containing the given
// module instance, or "" for a child of the root module.
func parentModule(addr string) (string, error) {
	path, err := tfaddr.ParseModulePath(addr)
	if err != nil {
		return "", xerrors.Errorf("parse module address %q: %w", addr, err)
	}
	return path.Parent().String(), nil
}
