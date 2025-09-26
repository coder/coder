package rbac

import (
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// ParseAllowListEntry parses a single allow-list entry string in the form
// "*:*", "<resource_type>:*", or "<resource_type>:<uuid>" into an
// AllowListElement with validation.
func ParseAllowListEntry(s string) (AllowListElement, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	res, id, ok := ParseResourceAction(s)
	if !ok {
		return AllowListElement{}, xerrors.Errorf("invalid allow_list entry %q: want <type>:<id>", s)
	}

	return NewAllowListElement(res, id)
}

func NewAllowListElement(resourceType string, id string) (AllowListElement, error) {
	if resourceType != policy.WildcardSymbol {
		if _, ok := policy.RBACPermissions[resourceType]; !ok {
			return AllowListElement{}, xerrors.Errorf("unknown resource type %q", resourceType)
		}
	}
	if id != policy.WildcardSymbol {
		if _, err := uuid.Parse(id); err != nil {
			return AllowListElement{}, xerrors.Errorf("invalid %s ID (must be UUID): %q", resourceType, id)
		}
	}

	return AllowListElement{Type: resourceType, ID: id}, nil
}

// ParseAllowList parses, validates, normalizes, and deduplicates a list of
// allow-list entries. If max is <=0, a default cap of 128 is applied.
func ParseAllowList(inputs []string, maxEntries int) ([]AllowListElement, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if maxEntries <= 0 {
		maxEntries = 128
	}
	if len(inputs) > maxEntries {
		return nil, xerrors.Errorf("allow_list has %d entries; max allowed is %d", len(inputs), maxEntries)
	}

	elems := make([]AllowListElement, 0, len(inputs))
	for _, s := range inputs {
		e, err := ParseAllowListEntry(s)
		if err != nil {
			return nil, err
		}
		// Global wildcard short-circuits
		if e.Type == policy.WildcardSymbol && e.ID == policy.WildcardSymbol {
			return []AllowListElement{AllowListAll()}, nil
		}
		elems = append(elems, e)
	}

	return ProcessAllowList(elems, maxEntries)
}

func ProcessAllowList(inputs []AllowListElement, maxEntries int) ([]AllowListElement, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if maxEntries <= 0 {
		maxEntries = 128
	}
	if len(inputs) > maxEntries {
		return nil, xerrors.Errorf("allow_list has %d entries; max allowed is %d", len(inputs), maxEntries)
	}

	// Collapse typed wildcards and drop shadowed IDs
	typedWildcard := map[string]struct{}{}
	idsByType := map[string]map[string]struct{}{}
	for _, e := range inputs {
		// Global wildcard short-circuits
		if e.Type == policy.WildcardSymbol && e.ID == policy.WildcardSymbol {
			return []AllowListElement{AllowListAll()}, nil
		}

		if e.ID == policy.WildcardSymbol {
			typedWildcard[e.Type] = struct{}{}
			continue
		}
		if idsByType[e.Type] == nil {
			idsByType[e.Type] = map[string]struct{}{}
		}
		idsByType[e.Type][e.ID] = struct{}{}
	}

	out := make([]AllowListElement, 0)
	for t, ids := range idsByType {
		if _, ok := typedWildcard[t]; ok {
			out = append(out, AllowListElement{Type: t, ID: policy.WildcardSymbol})
			continue
		}
		for id := range ids {
			out = append(out, AllowListElement{Type: t, ID: id})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].ID < out[j].ID
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}
