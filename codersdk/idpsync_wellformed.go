package codersdk

import (
	"bytes"
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// GroupSyncFindingCode names a condition that group sync settings violate.
// The codes are stable: they appear in API responses and may later be stored
// as part of a policy.
type GroupSyncFindingCode string

// Finding codes reported by the syntax validator. Each names a violation of
// one of the well-formedness conditions W1 through W8 in
// `coderd/idpsync/SYNC_SETTINGS.md`.
const (
	// GroupSyncFindingFieldEmpty: `field` is empty while another member is
	// nonempty (W4).
	GroupSyncFindingFieldEmpty GroupSyncFindingCode = "field_empty"
	// GroupSyncFindingEmptyMappingKey: a member name of `mapping` is the
	// empty string (W5).
	GroupSyncFindingEmptyMappingKey GroupSyncFindingCode = "empty_mapping_key"
	// GroupSyncFindingNullArray: a member of `mapping` has the value `null`
	// (W5).
	GroupSyncFindingNullArray GroupSyncFindingCode = "null_array"
	// GroupSyncFindingNilUUID: a `mapping` array contains the nil UUID (W5).
	GroupSyncFindingNilUUID GroupSyncFindingCode = "nil_uuid"
	// GroupSyncFindingRepeatedUUID: a `mapping` array contains a UUID that
	// already appears earlier in the same array (W5). The nil UUID is
	// reported as GroupSyncFindingNilUUID at every position, never as a
	// repeat.
	GroupSyncFindingRepeatedUUID GroupSyncFindingCode = "repeated_uuid"
	// GroupSyncFindingFilterExcludedKey: `regex_filter` is a nonempty string
	// that does not match a member name of `mapping` (W6).
	GroupSyncFindingFilterExcludedKey GroupSyncFindingCode = "filter_excluded_key"
	// GroupSyncFindingLegacyMapping: `legacy_group_name_mapping` is nonempty
	// (W8).
	GroupSyncFindingLegacyMapping GroupSyncFindingCode = "legacy_mapping"
)

// GroupSyncFinding is one violation found in group sync settings.
type GroupSyncFinding struct {
	// Code names the condition violated.
	Code GroupSyncFindingCode `json:"code"`
	// Path is a JSON Pointer (RFC 6901) to the offending element of the
	// settings text. Array indexes refer to positions in the settings as
	// given. The empty path refers to the whole text.
	Path string `json:"path"`
}

// WellFormedFindings reports every violation of the well-formedness
// conditions W1 through W8 that is visible in s. It returns an empty slice
// when s is well-formed.
//
// The findings are ordered by path, then by code. Paths compare token by
// token, with member names compared by code point and array indexes compared
// as numbers.
//
// Violations that decoding resolves before s exists cannot be seen here:
// duplicate and unknown member names, member names that differ only in case,
// a top-level `null`, data after the JSON value, and UUIDs spelled in a
// form other than canonical text.
func (s GroupSyncSettings) WellFormedFindings() []GroupSyncFinding {
	var found []syntaxFinding

	regexSet := s.RegexFilter != nil && s.RegexFilter.String() != ""

	// W4: when `field` is empty, every other member must be in an empty form.
	// `regex_filter` as the empty string is an empty form.
	if s.Field == "" &&
		(len(s.Mapping) > 0 || regexSet || s.AutoCreateMissing || len(s.LegacyNameMapping) > 0) {
		found = append(found, syntaxFinding{member: "field", code: GroupSyncFindingFieldEmpty})
	}

	// W8: the legacy mapping must be empty.
	if len(s.LegacyNameMapping) > 0 {
		found = append(found, syntaxFinding{member: "legacy_group_name_mapping", code: GroupSyncFindingLegacyMapping})
	}

	// W5 and W6, member by member. Keys are visited in sorted order so the
	// findings do not depend on map iteration order, though they are sorted
	// again below.
	for _, key := range slices.Sorted(maps.Keys(s.Mapping)) {
		ids := s.Mapping[key]
		member := syntaxFinding{member: "mapping", key: key, hasKey: true}

		if key == "" {
			member.code = GroupSyncFindingEmptyMappingKey
			found = append(found, member)
		}
		// The filter matches by unanchored search, as sync applies it.
		if regexSet && !s.RegexFilter.MatchString(key) {
			member.code = GroupSyncFindingFilterExcludedKey
			found = append(found, member)
		}
		// JSON `null` decodes to a nil slice, and `[]` to an empty one.
		if ids == nil {
			member.code = GroupSyncFindingNullArray
			found = append(found, member)
		}

		seen := make(map[uuid.UUID]bool, len(ids))
		for i, id := range ids {
			element := syntaxFinding{member: "mapping", key: key, hasKey: true, index: i, hasIndex: true}
			switch {
			case id == uuid.Nil:
				// The nil UUID is reported at every position and never as
				// a repeat.
				element.code = GroupSyncFindingNilUUID
				found = append(found, element)
			case seen[id]:
				element.code = GroupSyncFindingRepeatedUUID
				found = append(found, element)
			default:
				seen[id] = true
			}
		}
	}

	slices.SortFunc(found, compareSyntaxFinding)

	out := make([]GroupSyncFinding, 0, len(found))
	for _, f := range found {
		out = append(out, GroupSyncFinding{Code: f.code, Path: f.path()})
	}
	return out
}

// CanonicalMappingUUIDs returns the UUIDs that appear in the arrays of
// `mapping`, as a canonical list: each UUID once, the nil UUID left out, in
// ascending order. It returns an empty slice when there are none.
//
// Nothing here establishes that the UUIDs name groups. The server reads the
// state of exactly these UUIDs to check the settings' validity, and the
// canonical form gives that read a stable input.
func (s GroupSyncSettings) CanonicalMappingUUIDs() []uuid.UUID {
	distinct := make(map[uuid.UUID]struct{})
	for _, ids := range s.Mapping {
		for _, id := range ids {
			if id != uuid.Nil {
				distinct[id] = struct{}{}
			}
		}
	}
	// Byte order of a UUID is the order of its canonical text, which is
	// lowercase hexadecimal.
	out := slices.SortedFunc(maps.Keys(distinct), func(a, b uuid.UUID) int {
		return bytes.Compare(a[:], b[:])
	})
	if out == nil {
		out = []uuid.UUID{}
	}
	return out
}

// CompareGroupSyncFindings orders two findings the way WellFormedFindings
// orders its result: by path, then by code. Member names compare by code
// point and array indexes as numbers, and a finding about a `mapping` member
// comes before the findings about its array elements. The empty path, which
// refers to the whole text, comes first.
//
// It is the one definition of that order. The server's full validator merges
// its own findings into the syntax validator's and sorts the result with this
// function, so the merged list is ordered exactly as WellFormedFindings
// orders its own. A comparison written out again at the point of use could
// drift from this one, and the two validators would then disagree on order.
func CompareGroupSyncFindings(a, b GroupSyncFinding) int {
	return compareSyntaxFinding(parseFindingPath(a), parseFindingPath(b))
}

// parseFindingPath recovers the parts of a finding's path. The paths a
// validator produces are the empty path, `/member`, `/mapping/key`, and
// `/mapping/key/index`, with the key escaped as in a JSON Pointer.
func parseFindingPath(f GroupSyncFinding) syntaxFinding {
	parsed := syntaxFinding{code: f.Code}
	if f.Path == "" {
		return parsed
	}
	tokens := strings.Split(strings.TrimPrefix(f.Path, "/"), "/")
	parsed.member = UnescapeJSONPointerToken(tokens[0])
	if len(tokens) > 1 {
		parsed.key, parsed.hasKey = UnescapeJSONPointerToken(tokens[1]), true
	}
	if len(tokens) > 2 {
		// A path this package produced always has a numeric index here.
		parsed.index, _ = strconv.Atoi(tokens[2])
		parsed.hasIndex = true
	}
	return parsed
}

// syntaxFinding is a finding before its path is rendered. It keeps the parts
// of the path separately, so findings sort by the documented order without
// parsing JSON Pointers back apart.
type syntaxFinding struct {
	// member is the top-level member the finding is about.
	member string
	// key is the `mapping` member name, when hasKey is true.
	key    string
	hasKey bool
	// index is the position in the `mapping` array, when hasIndex is true.
	index    int
	hasIndex bool
	code     GroupSyncFindingCode
}

// path renders the finding's location as a JSON Pointer (RFC 6901).
func (f syntaxFinding) path() string {
	p := "/" + f.member
	if f.hasKey {
		p += "/" + EscapeJSONPointerToken(f.key)
	}
	if f.hasIndex {
		p += "/" + strconv.Itoa(f.index)
	}
	return p
}

// compareSyntaxFinding orders findings by path, then by code. Member names
// compare by code point and array indexes as numbers. A finding about a
// `mapping` member comes before the findings about its array elements.
func compareSyntaxFinding(a, b syntaxFinding) int {
	return cmp.Or(
		strings.Compare(a.member, b.member),
		compareBool(a.hasKey, b.hasKey),
		strings.Compare(a.key, b.key),
		compareBool(a.hasIndex, b.hasIndex),
		cmp.Compare(a.index, b.index),
		strings.Compare(string(a.code), string(b.code)),
	)
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// EscapeJSONPointerToken escapes a reference token for a JSON Pointer: `~`
// becomes `~0` and `/` becomes `~1`, in that order (RFC 6901, section 3).
func EscapeJSONPointerToken(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

// UnescapeJSONPointerToken reverses EscapeJSONPointerToken: `~1` becomes `/`
// and `~0` becomes `~`, in that order (RFC 6901, section 4).
func UnescapeJSONPointerToken(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
}
