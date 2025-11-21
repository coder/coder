package rbac

import (
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// maxAllowListEntries caps normalized allow lists to a manageable size. This
// limit is intentionally arbitrary—just high enough for current use cases—so we
// can revisit it without implying any semantic contract.
const maxAllowListEntries = 128

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

	return NormalizeAllowList(elems)
}

// NormalizeAllowList enforces max entry limits, collapses typed wildcards, and
// produces a deterministic, deduplicated allow list. A global wildcard returns
// early with a single `[*:*]` entry, typed wildcards shadow specific IDs, and
// the final slice is sorted to keep downstream comparisons stable. When the
// input is empty we return an empty (non-nil) slice so callers can differentiate
// between "no restriction" and "not provided" cases.
func NormalizeAllowList(inputs []AllowListElement) ([]AllowListElement, error) {
	if len(inputs) == 0 {
		return []AllowListElement{}, nil
	}
	if len(inputs) > maxAllowListEntries {
		return nil, xerrors.Errorf("allow_list has %d entries; max allowed is %d", len(inputs), maxAllowListEntries)
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
	for t := range typedWildcard {
		out = append(out, AllowListElement{Type: t, ID: policy.WildcardSymbol})
	}
	for t, ids := range idsByType {
		if _, ok := typedWildcard[t]; ok {
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

// UnionAllowLists merges multiple allow lists, returning the set of resources
// permitted by any input. A global wildcard short-circuits the merge. When no
// entries are present across all inputs, the result is an empty allow list.
func UnionAllowLists(lists ...[]AllowListElement) ([]AllowListElement, error) {
	union := make([]AllowListElement, 0)
	seen := make(map[string]struct{})

	for _, list := range lists {
		for _, elem := range list {
			if elem.Type == policy.WildcardSymbol && elem.ID == policy.WildcardSymbol {
				return []AllowListElement{AllowListAll()}, nil
			}
			key := elem.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			union = append(union, elem)
		}
	}

	return NormalizeAllowList(union)
}

// IntersectAllowLists combines the allow list produced by RBAC expansion with the
// API key's stored allow list. The result enforces both constraints: any
// resource must be allowed by the scope *and* the database filter. Wildcards in
// either list are respected and short-circuit appropriately.
//
// Intuition: scope definitions provide the *ceiling* of what a key could touch,
// while the DB allow list can narrow that set. Technically, since this is
// an intersection, both can narrow each other.
//
// A few illustrative cases:
//
//	| Scope AllowList   | DB AllowList                          | Result            |
//	| ----------------- | ------------------------------------- | ----------------- |
//	| `[*:*]`           | `[workspace:A]`                       | `[workspace:A]`   |
//	| `[workspace:*]`   | `[workspace:A, workspace:B]`          | `[workspace:A, workspace:B]` |
//	| `[workspace:A]`   | `[workspace:A, workspace:B]`          | `[workspace:A]`   |
//	| `[]`              | `[workspace:A]`                       | `[workspace:A]`   |
//
// Today most API key scopes expand with an empty allow list (meaning "no
// scope-level restriction"), so the merge simply mirrors what the database
// stored. Only scopes that intentionally embed resource filters would trim the
// DB entries.
func IntersectAllowLists(scopeList []AllowListElement, dbList []AllowListElement) []AllowListElement {
	// Empty DB list means no additional restriction.
	if len(dbList) == 0 {
		// Defensive: API keys should always persist a non-empty allow list, but
		// we cannot have an empty allow list, thus we fail close.
		return nil
	}

	// If scope already allows everything, the db list is authoritative.
	scopeAll := allowListContainsAll(scopeList)
	dbAll := allowListContainsAll(dbList)

	switch {
	case scopeAll && dbAll:
		return []AllowListElement{AllowListAll()}
	case scopeAll:
		return dbList
	case dbAll:
		return scopeList
	}

	// Otherwise compute intersection.
	resultSet := make(map[string]AllowListElement)
	for _, scopeElem := range scopeList {
		matching := intersectAllow(scopeElem, dbList)
		for _, elem := range matching {
			resultSet[elem.String()] = elem
		}
	}

	if len(resultSet) == 0 {
		return []AllowListElement{}
	}

	result := make([]AllowListElement, 0, len(resultSet))
	for _, elem := range resultSet {
		result = append(result, elem)
	}

	slices.SortFunc(result, func(a, b AllowListElement) int {
		if a.Type == b.Type {
			return strings.Compare(a.ID, b.ID)
		}
		return strings.Compare(a.Type, b.Type)
	})

	normalized, err := NormalizeAllowList(result)
	if err != nil {
		return result
	}
	if normalized == nil {
		return []AllowListElement{}
	}
	return normalized
}

func allowListContainsAll(elements []AllowListElement) bool {
	if len(elements) == 0 {
		return false
	}
	for _, e := range elements {
		if e.Type == policy.WildcardSymbol && e.ID == policy.WildcardSymbol {
			return true
		}
	}
	return false
}

// intersectAllow returns the set of permit entries that satisfy both the scope
// element and the database allow list.
func intersectAllow(scopeElem AllowListElement, dbList []AllowListElement) []AllowListElement {
	// Scope element is wildcard -> intersection is db list.
	if scopeElem.Type == policy.WildcardSymbol && scopeElem.ID == policy.WildcardSymbol {
		return dbList
	}

	result := make([]AllowListElement, 0)
	for _, dbElem := range dbList {
		// DB entry wildcard -> keep scope element.
		if dbElem.Type == policy.WildcardSymbol && dbElem.ID == policy.WildcardSymbol {
			result = append(result, scopeElem)
			continue
		}

		if !typeMatches(scopeElem.Type, dbElem.Type) {
			continue
		}

		if !idMatches(scopeElem.ID, dbElem.ID) {
			continue
		}

		result = append(result, AllowListElement{
			Type: intersectType(scopeElem.Type, dbElem.Type),
			ID:   intersectID(scopeElem.ID, dbElem.ID),
		})
	}
	return result
}

func typeMatches(scopeType, dbType string) bool {
	return scopeType == dbType || scopeType == policy.WildcardSymbol || dbType == policy.WildcardSymbol
}

func idMatches(scopeID, dbID string) bool {
	return scopeID == dbID || scopeID == policy.WildcardSymbol || dbID == policy.WildcardSymbol
}

func intersectType(scopeType, dbType string) string {
	if scopeType == dbType {
		return scopeType
	}
	if scopeType == policy.WildcardSymbol {
		return dbType
	}
	return scopeType
}

func intersectID(scopeID, dbID string) string {
	switch {
	case scopeID == dbID:
		return scopeID
	case scopeID == policy.WildcardSymbol:
		return dbID
	case dbID == policy.WildcardSymbol:
		return scopeID
	default:
		// Should not happen when intersecting with matching IDs; fallback to scope ID.
		return scopeID
	}
}
