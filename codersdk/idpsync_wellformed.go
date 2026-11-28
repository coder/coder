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

// GroupSyncFinding is one violation found in group sync settings. The
// validators produce findings and the server's rewriter consumes them. API
// responses carry a finding's path rendered as a JSON Pointer, in warnings and
// in rejections, never the finding itself.
//
// @typescript-ignore GroupSyncFinding
type GroupSyncFinding struct {
	// Code names the condition violated.
	Code GroupSyncFindingCode
	// Path locates the offending element of the settings text. Array indexes
	// refer to positions in the settings as given.
	Path GroupSyncPath
}

// GroupSyncPath locates an element of a group sync settings text that a
// finding can refer to. It holds the parts of a JSON Pointer (RFC 6901), so
// code that acts on a finding reads the parts and never parses a pointer;
// String renders the pointer for API output. The zero value refers to the
// whole text. A path is made only by the constructors below, so it always has
// one of the shapes GroupSyncPathKind lists.
//
// @typescript-ignore GroupSyncPath
type GroupSyncPath struct {
	kind GroupSyncPathKind
	// member is set for GroupSyncPathMember, and is `mapping` for the two
	// mapping kinds.
	member GroupSyncMember
	// key is set for GroupSyncPathMappingArray and GroupSyncPathMappingElement.
	key string
	// index is set for GroupSyncPathMappingElement.
	index int
}

// GroupSyncPathKind says which shape a GroupSyncPath has.
//
// @typescript-ignore GroupSyncPathKind
type GroupSyncPathKind int

const (
	// GroupSyncPathWhole refers to the whole text. It renders as "".
	GroupSyncPathWhole GroupSyncPathKind = iota
	// GroupSyncPathMember refers to a top-level member. It renders as
	// "/<member>".
	GroupSyncPathMember
	// GroupSyncPathMappingArray refers to the array under one key of
	// `mapping`. It renders as "/mapping/<key>", with the key escaped.
	GroupSyncPathMappingArray
	// GroupSyncPathMappingElement refers to one element of such an array. It
	// renders as "/mapping/<key>/<index>".
	GroupSyncPathMappingElement
)

// GroupSyncMember names a top-level member of group sync settings, from the
// list in W3.
//
// @typescript-ignore GroupSyncMember
type GroupSyncMember string

const (
	GroupSyncMemberField             GroupSyncMember = "field"
	GroupSyncMemberMapping           GroupSyncMember = "mapping"
	GroupSyncMemberRegexFilter       GroupSyncMember = "regex_filter"
	GroupSyncMemberAutoCreateMissing GroupSyncMember = "auto_create_missing_groups"
	GroupSyncMemberLegacyMapping     GroupSyncMember = "legacy_group_name_mapping"
)

// GroupSyncWholePath returns the path to the whole text.
func GroupSyncWholePath() GroupSyncPath {
	return GroupSyncPath{kind: GroupSyncPathWhole}
}

// GroupSyncMemberPath returns the path to top-level member m.
func GroupSyncMemberPath(m GroupSyncMember) GroupSyncPath {
	return GroupSyncPath{kind: GroupSyncPathMember, member: m}
}

// GroupSyncMappingArrayPath returns the path to the array under key in
// `mapping`.
func GroupSyncMappingArrayPath(key string) GroupSyncPath {
	return GroupSyncPath{kind: GroupSyncPathMappingArray, member: GroupSyncMemberMapping, key: key}
}

// GroupSyncMappingElementPath returns the path to element index of the array
// under key in `mapping`.
func GroupSyncMappingElementPath(key string, index int) GroupSyncPath {
	return GroupSyncPath{kind: GroupSyncPathMappingElement, member: GroupSyncMemberMapping, key: key, index: index}
}

// Kind returns the shape of p.
func (p GroupSyncPath) Kind() GroupSyncPathKind { return p.kind }

// Member returns the top-level member p is within: the member itself for
// GroupSyncPathMember, `mapping` for the mapping kinds, and "" for the whole
// text.
func (p GroupSyncPath) Member() GroupSyncMember { return p.member }

// Key returns the `mapping` key of a mapping kind, and "" otherwise.
func (p GroupSyncPath) Key() string { return p.key }

// Index returns the array index of GroupSyncPathMappingElement, and 0
// otherwise.
func (p GroupSyncPath) Index() int { return p.index }

// String renders p as a JSON Pointer (RFC 6901).
func (p GroupSyncPath) String() string {
	switch p.kind {
	case GroupSyncPathMember:
		return "/" + EscapeJSONPointerToken(string(p.member))
	case GroupSyncPathMappingArray:
		return "/mapping/" + EscapeJSONPointerToken(p.key)
	case GroupSyncPathMappingElement:
		return "/mapping/" + EscapeJSONPointerToken(p.key) + "/" + strconv.Itoa(p.index)
	default:
		return ""
	}
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
	var found []GroupSyncFinding

	regexSet := s.RegexFilter != nil && s.RegexFilter.String() != ""

	// W4: when `field` is empty, every other member must be in an empty form.
	// `regex_filter` as the empty string is an empty form.
	if s.Field == "" &&
		(len(s.Mapping) > 0 || regexSet || s.AutoCreateMissing || len(s.LegacyNameMapping) > 0) {
		found = append(found, GroupSyncFinding{Code: GroupSyncFindingFieldEmpty, Path: GroupSyncMemberPath(GroupSyncMemberField)})
	}

	// W8: the legacy mapping must be empty.
	if len(s.LegacyNameMapping) > 0 {
		found = append(found, GroupSyncFinding{Code: GroupSyncFindingLegacyMapping, Path: GroupSyncMemberPath(GroupSyncMemberLegacyMapping)})
	}

	// W5 and W6, member by member. Keys are visited in sorted order so the
	// findings do not depend on map iteration order, though they are sorted
	// again below.
	for _, key := range slices.Sorted(maps.Keys(s.Mapping)) {
		ids := s.Mapping[key]
		array := GroupSyncMappingArrayPath(key)

		if key == "" {
			found = append(found, GroupSyncFinding{Code: GroupSyncFindingEmptyMappingKey, Path: array})
		}
		// The filter matches by unanchored search, as sync applies it.
		if regexSet && !s.RegexFilter.MatchString(key) {
			found = append(found, GroupSyncFinding{Code: GroupSyncFindingFilterExcludedKey, Path: array})
		}
		// JSON `null` decodes to a nil slice, and `[]` to an empty one.
		if ids == nil {
			found = append(found, GroupSyncFinding{Code: GroupSyncFindingNullArray, Path: array})
		}

		seen := make(map[uuid.UUID]bool, len(ids))
		for i, id := range ids {
			element := GroupSyncMappingElementPath(key, i)
			switch {
			case id == uuid.Nil:
				// The nil UUID is reported at every position and never as
				// a repeat.
				found = append(found, GroupSyncFinding{Code: GroupSyncFindingNilUUID, Path: element})
			case seen[id]:
				found = append(found, GroupSyncFinding{Code: GroupSyncFindingRepeatedUUID, Path: element})
			default:
				seen[id] = true
			}
		}
	}

	slices.SortFunc(found, CompareGroupSyncFindings)
	if found == nil {
		found = []GroupSyncFinding{}
	}
	return found
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
	return cmp.Or(
		CompareGroupSyncPaths(a.Path, b.Path),
		strings.Compare(string(a.Code), string(b.Code)),
	)
}

// CompareGroupSyncPaths orders paths token by token: member names by code
// point, then `mapping` keys by code point, then array indexes as numbers. A
// path with fewer tokens comes before a longer path that extends it, so the
// whole text comes first and a `mapping` key comes before its elements.
func CompareGroupSyncPaths(a, b GroupSyncPath) int {
	aHasKey := a.kind == GroupSyncPathMappingArray || a.kind == GroupSyncPathMappingElement
	bHasKey := b.kind == GroupSyncPathMappingArray || b.kind == GroupSyncPathMappingElement
	return cmp.Or(
		strings.Compare(string(a.member), string(b.member)),
		compareBool(aHasKey, bHasKey),
		strings.Compare(a.key, b.key),
		compareBool(a.kind == GroupSyncPathMappingElement, b.kind == GroupSyncPathMappingElement),
		cmp.Compare(a.index, b.index),
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
