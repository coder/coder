package idpsync_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

// This file holds the test vectors for the full validator of group sync
// settings, which checks the validity conditions V1 through V5 in
// `SYNC_SETTINGS.md` in this directory. V1 is well-formedness, so the full
// validator calls the syntax validator and reports its findings along with
// its own. The tests that use these vectors follow them in this file.
//
// Each vector is a JSON text that decodes into `codersdk.GroupSyncSettings`,
// a description of the state of the deployment, and the outcome the full
// validator must produce. Texts that fail to decode belong to the syntax
// validator's vectors in `codersdk/idpsync_wellformed_test.go`; the full
// validator never sees them.
//
// The validator does not read the database. Its caller asks the settings for
// the canonical list of UUIDs in `mapping`, reads a snapshot of the deployment
// for those UUIDs, and passes the snapshot in. A test builds that snapshot
// from the vector's snapshotState and the UUIDs it was read for, which are the
// vector's requested UUIDs unless snapshotCovers says otherwise.
//
// Whitespace between JSON tokens is insignificant (RFC 8259), so the layout of
// an input has no effect on how it decodes. Long inputs are written as
// multiline raw strings for readability, and short ones on a single line.
//
// These UUIDs recur in the vectors. Their values carry no meaning beyond
// being distinct and nonzero.
//
//	99999999-9999-4999-8999-999999999999  organization O, the target
//	88888888-8888-4888-8888-888888888888  organization P, another one
//	11111111-1111-4111-8111-111111111111  group G, in O
//	22222222-2222-4222-8222-222222222222  group H, in O
//	33333333-3333-4333-8333-333333333333  group X, in P
//	44444444-4444-4444-8444-444444444444  M, which names no group
//
// The Everyone group of an organization has the organization's ID, so O's ID
// is also the ID of a group in O.

// orgO is the target organization of every vector.
const orgO = "99999999-9999-4999-8999-999999999999"

// fullVector is one test vector for the full validator.
type fullVector struct {
	// name identifies the vector in test output. It is unique within the
	// table.
	name string
	// why says what the vector checks and why its outcome is the right one.
	why string
	// input is the JSON text given to the decoder. It must decode.
	input string
	// requested lists the IDs `CanonicalMappingUUIDs` must return for the input: every
	// distinct UUID in `mapping` other than the nil UUID, in ascending order.
	// It is empty when the settings name no UUIDs.
	requested []string
	// snapshot describes the state of the deployment the validator sees.
	snapshot snapshotState
	// snapshotCovers lists the IDs the snapshot was read for, when they differ
	// from requested. When nil, the snapshot was read for exactly requested.
	// Only the vectors that check coverage set it.
	snapshotCovers []string
	// want is the outcome the vector expects.
	want fullWant
}

// snapshotState describes the state of the deployment a snapshot reports.
// The organization is always O. A test gives the snapshot an entry for each
// ID it was read for: GroupInsideOrganization for an ID in inside,
// GroupOutsideOrganization for an ID in outside, and GroupDoesNotExist for
// any other. An ID the snapshot was not read for has no entry, even when it
// appears in inside or outside.
type snapshotState struct {
	// org is the state of O.
	org idpsync.OrganizationStatus
	// inside lists the IDs of groups that belong to O.
	inside []string
	// outside lists the IDs of groups that belong to another organization.
	outside []string
}

// The snapshot states the vectors use.
var (
	// snapshotTypical is the deployment described at the top of this file:
	// O is active, G, H and O's Everyone group are in O, X is in P, and M
	// names no group.
	snapshotTypical = snapshotState{
		org: idpsync.OrganizationActive,
		inside: []string{
			"11111111-1111-4111-8111-111111111111",
			"22222222-2222-4222-8222-222222222222",
			orgO,
		},
		outside: []string{"33333333-3333-4333-8333-333333333333"},
	}
	// snapshotDeleted is snapshotTypical with O soft-deleted. Deletion
	// requires that O have no groups but Everyone, so G and H name no group.
	snapshotDeleted = snapshotState{
		org:     idpsync.OrganizationDeleted,
		inside:  []string{orgO},
		outside: []string{"33333333-3333-4333-8333-333333333333"},
	}
	// snapshotAbsent has no organization with O's ID, and so no groups in it.
	snapshotAbsent = snapshotState{
		org:     idpsync.OrganizationAbsent,
		outside: []string{"33333333-3333-4333-8333-333333333333"},
	}
)

// fullWant is the outcome a full vector expects: either the snapshot does not
// cover the settings and the validator returns an error, or the validator
// reports exactly the listed findings.
type fullWant interface{ isFullWant() }

// coverageError expects the validator to return an error and no findings,
// because the snapshot was not read for every ID in the settings.
type coverageError struct{}

// fullFindings expects the validator to report exactly these findings, in
// this order. An empty list means the settings are valid. Findings are
// ordered as the syntax validator orders them: by path, then by code, with
// paths compared token by token and array indexes compared as numbers.
//
// Each array element of `mapping` has at most one finding: an element the
// syntax validator reports as the nil UUID or as a repeat gets no V3, V4, or
// V5 finding.
type fullFindings []fullFinding

// fullFinding is one finding the full validator reports.
type fullFinding struct {
	// code names the condition violated.
	code codersdk.GroupSyncFindingCode
	// path is a JSON Pointer (RFC 6901) to the offending element of the input.
	// A finding about the organization, which is not in the text, has the
	// empty path, which refers to the whole text.
	path string
}

func (coverageError) isFullWant() {}
func (fullFindings) isFullWant()  {}

// fullVectors is the table of test vectors for the full validator.
//
// The vectors are grouped in this order:
//
//  1. Valid settings, which produce no findings.
//  2. Violations of V2 (the organization).
//  3. Violations of V3 (a UUID names no group).
//  4. Violations of V4 (a group in another organization).
//  5. Violations of V5 (the Everyone group).
//  6. Interaction with the syntax validator's findings (V1).
//  7. Coverage: a snapshot read for fewer IDs than the settings name.
//  8. Settings with several findings, which check the order of findings.
var fullVectors = []fullVector{
	// Group 1: valid settings. Each is well-formed and satisfies V2 through
	// V5 against its snapshot.
	{
		name:      "valid/empty_object",
		why:       "Settings that name no UUIDs are valid for an active organization. V2 still applies, so the snapshot still carries the organization's state.",
		input:     `{}`,
		snapshot:  snapshotTypical,
		requested: []string{},
		want:      fullFindings{},
	},
	{
		name: "valid/typical",
		why:  "Two mapped IdP groups, each naming a group in O.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["22222222-2222-4222-8222-222222222222"]
			}
		}`,
		snapshot: snapshotTypical,
		requested: []string{
			"11111111-1111-4111-8111-111111111111",
			"22222222-2222-4222-8222-222222222222",
		},
		want: fullFindings{},
	},
	{
		name: "valid/same_group_under_two_keys",
		why:  "G appears under two keys. It is requested once, and both occurrences are valid.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"11111111-1111-4111-8111-111111111111"},
		want:      fullFindings{},
	},
	{
		name:      "valid/empty_array",
		why:       "An empty array names no UUIDs, so there is nothing for V3 through V5 to check.",
		input:     `{"field":"groups","mapping":{"eng":[]}}`,
		snapshot:  snapshotTypical,
		requested: []string{},
		want:      fullFindings{},
	},

	// Group 2: violations of V2. The organization is not in the text, so the
	// finding has the empty path.
	{
		name:      "v2/organization_deleted",
		why:       "Settings for a soft-deleted organization violate V2, even when they name no UUIDs.",
		input:     `{"field":"groups"}`,
		snapshot:  snapshotDeleted,
		requested: []string{},
		want:      fullFindings{{code: idpsync.GroupSyncFindingOrganizationDeleted, path: ""}},
	},
	{
		name:      "v2/organization_absent",
		why:       "Settings for an organization ID that names no organization violate V2.",
		input:     `{"field":"groups"}`,
		snapshot:  snapshotAbsent,
		requested: []string{},
		want:      fullFindings{{code: idpsync.GroupSyncFindingOrganizationAbsent, path: ""}},
	},
	{
		name: "v2/organization_deleted_with_groups",
		why:  "V2 does not stop the other checks. G no longer names a group once O is deleted, so V3 is violated too.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		snapshot:  snapshotDeleted,
		requested: []string{"11111111-1111-4111-8111-111111111111"},
		want: fullFindings{
			{code: idpsync.GroupSyncFindingOrganizationDeleted, path: ""},
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/0"},
		},
	},

	// Group 3: violations of V3. M names no group.
	{
		name: "v3/missing_group",
		why:  "M names no group.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["44444444-4444-4444-8444-444444444444"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"44444444-4444-4444-8444-444444444444"},
		want:      fullFindings{{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/0"}},
	},
	{
		name: "v3/missing_group_after_valid",
		why:  "The path gives M's position in the input array; G before it is valid.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"44444444-4444-4444-8444-444444444444"
				]
			}
		}`,
		snapshot: snapshotTypical,
		requested: []string{
			"11111111-1111-4111-8111-111111111111",
			"44444444-4444-4444-8444-444444444444",
		},
		want: fullFindings{{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/1"}},
	},
	{
		name: "v3/missing_group_under_two_keys",
		why:  "M is requested once but reported at each position where it appears.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["44444444-4444-4444-8444-444444444444"],
				"ops": ["44444444-4444-4444-8444-444444444444"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"44444444-4444-4444-8444-444444444444"},
		want: fullFindings{
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/0"},
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/ops/0"},
		},
	},

	// Group 4: violations of V4. X is a group in P.
	{
		name: "v4/outside_group",
		why:  "X belongs to P, not to the target organization O.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["33333333-3333-4333-8333-333333333333"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"33333333-3333-4333-8333-333333333333"},
		want:      fullFindings{{code: idpsync.GroupSyncFindingOutsideGroup, path: "/mapping/eng/0"}},
	},
	{
		name: "v4/other_organizations_everyone_group",
		why:  "P's Everyone group has P's ID and belongs to P, so it is a group in another organization, not O's Everyone group.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["88888888-8888-4888-8888-888888888888"]
			}
		}`,
		snapshot: snapshotState{
			org:     idpsync.OrganizationActive,
			inside:  []string{orgO},
			outside: []string{"88888888-8888-4888-8888-888888888888"},
		},
		requested: []string{"88888888-8888-4888-8888-888888888888"},
		want:      fullFindings{{code: idpsync.GroupSyncFindingOutsideGroup, path: "/mapping/eng/0"}},
	},

	// Group 5: violations of V5. O's ID is also the ID of O's Everyone group.
	{
		name: "v5/everyone_group",
		why:  "O's Everyone group exists and belongs to O, so V3 and V4 hold, but V5 excludes it.",
		input: `{
			"field": "groups",
			"mapping": {
				"all": ["99999999-9999-4999-8999-999999999999"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{orgO},
		want:      fullFindings{{code: idpsync.GroupSyncFindingEveryoneGroup, path: "/mapping/all/0"}},
	},
	{
		name: "v5/everyone_group_of_deleted_organization",
		why:  "A deleted organization keeps its Everyone group, so the UUID still names O's Everyone group, alongside the V2 violation.",
		input: `{
			"field": "groups",
			"mapping": {
				"all": ["99999999-9999-4999-8999-999999999999"]
			}
		}`,
		snapshot:  snapshotDeleted,
		requested: []string{orgO},
		want: fullFindings{
			{code: idpsync.GroupSyncFindingOrganizationDeleted, path: ""},
			{code: idpsync.GroupSyncFindingEveryoneGroup, path: "/mapping/all/0"},
		},
	},

	// Group 6: interaction with the syntax validator's findings. The full
	// validator reports them unchanged, and gives no V3, V4, or V5 finding
	// to an element the syntax validator already reports.
	{
		name: "v1/syntax_finding_passed_through",
		why:  "A legacy mapping is a syntax finding. The full validator reports it unchanged alongside valid groups.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"legacy_group_name_mapping": {"idp-eng": "eng"}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"11111111-1111-4111-8111-111111111111"},
		want:      fullFindings{{code: codersdk.GroupSyncFindingLegacyMapping, path: "/legacy_group_name_mapping"}},
	},
	{
		name: "v1/nil_uuid_not_requested",
		why:  "The nil UUID is a syntax finding. It is not requested from the snapshot and gets no V3 finding.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["00000000-0000-0000-0000-000000000000"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{},
		want:      fullFindings{{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0"}},
	},
	{
		name: "v1/repeated_missing_group",
		why:  "M twice: the first occurrence gets the V3 finding, and the second is reported only as a repeat.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"44444444-4444-4444-8444-444444444444",
					"44444444-4444-4444-8444-444444444444"
				]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"44444444-4444-4444-8444-444444444444"},
		want: fullFindings{
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/0"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/1"},
		},
	},
	{
		name: "v1/filter_excluded_key_with_outside_group",
		why:  "A key the filter excludes is a syntax finding on the member. Its array element is still checked, and X gets a V4 finding at its own path.",
		input: `{
			"field": "groups",
			"mapping": {
				"ops": ["33333333-3333-4333-8333-333333333333"]
			},
			"regex_filter": "^eng"
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"33333333-3333-4333-8333-333333333333"},
		want: fullFindings{
			{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops"},
			{code: idpsync.GroupSyncFindingOutsideGroup, path: "/mapping/ops/0"},
		},
	},
	{
		name: "v1/empty_key_with_missing_group",
		why:  "An empty key is a syntax finding on the member. M under it gets a V3 finding at `/mapping//0`.",
		input: `{
			"field": "groups",
			"mapping": {
				"": ["44444444-4444-4444-8444-444444444444"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"44444444-4444-4444-8444-444444444444"},
		want: fullFindings{
			{code: codersdk.GroupSyncFindingEmptyMappingKey, path: "/mapping/"},
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping//0"},
		},
	},
	{
		name: "v1/field_empty_with_missing_group",
		why:  "An empty `field` does not stop the V checks; the settings are dormant, not exempt.",
		input: `{
			"field": "",
			"mapping": {
				"eng": ["44444444-4444-4444-8444-444444444444"]
			}
		}`,
		snapshot:  snapshotTypical,
		requested: []string{"44444444-4444-4444-8444-444444444444"},
		want: fullFindings{
			{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field"},
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/0"},
		},
	},

	// Group 7: coverage. A snapshot must have been read for every ID in the
	// settings. When it was not, the validator returns an error and no
	// findings, rather than treat an unread ID as naming no group; on the
	// fetch route that mistake would remove a correct mapping.
	{
		name: "coverage/none",
		why:  "The snapshot was read for no IDs, but the settings name G.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		requested:      []string{"11111111-1111-4111-8111-111111111111"},
		snapshot:       snapshotTypical,
		snapshotCovers: []string{},
		want:           coverageError{},
	},
	{
		name: "coverage/partial_with_syntax_findings",
		why:  "The snapshot covers G but not M. The error discards the syntax finding on the nil UUID too, so a caller never acts on a partial result.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"00000000-0000-0000-0000-000000000000",
					"11111111-1111-4111-8111-111111111111",
					"44444444-4444-4444-8444-444444444444"
				]
			}
		}`,
		requested: []string{
			"11111111-1111-4111-8111-111111111111",
			"44444444-4444-4444-8444-444444444444",
		},
		snapshot:       snapshotTypical,
		snapshotCovers: []string{"11111111-1111-4111-8111-111111111111"},
		want:           coverageError{},
	},
	{
		name: "coverage/superset",
		why:  "A snapshot read for more IDs than the settings name is fine; the extra IDs are ignored.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		requested: []string{"11111111-1111-4111-8111-111111111111"},
		snapshot:  snapshotTypical,
		snapshotCovers: []string{
			"11111111-1111-4111-8111-111111111111",
			"22222222-2222-4222-8222-222222222222",
			"33333333-3333-4333-8333-333333333333",
		},
		want: fullFindings{},
	},

	// Group 8: settings with several findings. Syntax and V findings are
	// merged and ordered by path, then by code.
	{
		name: "several/one_of_each_group_violation",
		why:  "V3, V4, and V5 at once, with a valid group among them, in input order that differs from the order of findings.",
		input: `{
			"field": "groups",
			"mapping": {
				"ops": ["33333333-3333-4333-8333-333333333333"],
				"eng": [
					"44444444-4444-4444-8444-444444444444",
					"11111111-1111-4111-8111-111111111111",
					"99999999-9999-4999-8999-999999999999"
				]
			}
		}`,
		snapshot: snapshotTypical,
		requested: []string{
			"11111111-1111-4111-8111-111111111111",
			"33333333-3333-4333-8333-333333333333",
			"44444444-4444-4444-8444-444444444444",
			orgO,
		},
		want: fullFindings{
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/0"},
			{code: idpsync.GroupSyncFindingEveryoneGroup, path: "/mapping/eng/2"},
			{code: idpsync.GroupSyncFindingOutsideGroup, path: "/mapping/ops/0"},
		},
	},
	{
		name: "several/syntax_and_validity_interleaved",
		why:  "Syntax findings and V findings in one array, merged by numeric index.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"33333333-3333-4333-8333-333333333333",
					"00000000-0000-0000-0000-000000000000",
					"44444444-4444-4444-8444-444444444444",
					"33333333-3333-4333-8333-333333333333"
				]
			}
		}`,
		snapshot: snapshotTypical,
		requested: []string{
			"33333333-3333-4333-8333-333333333333",
			"44444444-4444-4444-8444-444444444444",
		},
		want: fullFindings{
			{code: idpsync.GroupSyncFindingOutsideGroup, path: "/mapping/eng/0"},
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/1"},
			{code: idpsync.GroupSyncFindingMissingGroup, path: "/mapping/eng/2"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/3"},
		},
	},
}

// allValidityCodes lists every finding code the full validator exports. Go
// cannot enumerate a type's constants, so this list is kept by hand;
// TestValidityFindingsCodes checks that every code in it is exercised.
var allValidityCodes = []codersdk.GroupSyncFindingCode{
	idpsync.GroupSyncFindingOrganizationDeleted,
	idpsync.GroupSyncFindingOrganizationAbsent,
	idpsync.GroupSyncFindingMissingGroup,
	idpsync.GroupSyncFindingOutsideGroup,
	idpsync.GroupSyncFindingEveryoneGroup,
}

// syntaxCodesPassedThrough lists the syntax validator's codes. The full
// validator passes them through, so the vectors may use any of them.
var syntaxCodesPassedThrough = []codersdk.GroupSyncFindingCode{
	codersdk.GroupSyncFindingFieldEmpty,
	codersdk.GroupSyncFindingEmptyMappingKey,
	codersdk.GroupSyncFindingNullArray,
	codersdk.GroupSyncFindingNilUUID,
	codersdk.GroupSyncFindingRepeatedUUID,
	codersdk.GroupSyncFindingFilterExcludedKey,
	codersdk.GroupSyncFindingLegacyMapping,
}

// decodeFullInput decodes a vector's input the way `httpapi.Read` decodes a
// request body: it reads one JSON value and ignores anything after it.
func decodeFullInput(input string) (codersdk.GroupSyncSettings, error) {
	var s codersdk.GroupSyncSettings
	err := json.NewDecoder(strings.NewReader(input)).Decode(&s)
	return s, err
}

// buildSnapshot builds the snapshot a vector describes. The snapshot was read
// for v.snapshotCovers, or for v.requested when snapshotCovers is nil, and has
// an entry for each of those IDs and no others.
func buildSnapshot(v fullVector) idpsync.GroupSnapshot {
	covers := v.requested
	if v.snapshotCovers != nil {
		covers = v.snapshotCovers
	}
	groups := make(map[uuid.UUID]idpsync.GroupStatus, len(covers))
	for _, id := range covers {
		status := idpsync.GroupDoesNotExist
		switch {
		case slices.Contains(v.snapshot.inside, id):
			status = idpsync.GroupInsideOrganization
		case slices.Contains(v.snapshot.outside, id):
			status = idpsync.GroupOutsideOrganization
		}
		groups[uuid.MustParse(id)] = status
	}
	return idpsync.GroupSnapshot{
		OrganizationID: uuid.MustParse(orgO),
		Organization:   v.snapshot.org,
		Groups:         groups,
	}
}

// asFullFindings converts the validator's findings to the vectors'
// representation. The result is never nil, so that it compares equal to an
// empty findings literal.
func asFullFindings(got []codersdk.GroupSyncFinding) fullFindings {
	out := fullFindings{}
	for _, f := range got {
		out = append(out, fullFinding{code: f.Code, path: f.Path})
	}
	return out
}

// compareFullPointers orders two JSON Pointers token by token. Two tokens
// that are both array indexes compare as numbers; otherwise they compare by
// code point. A pointer that is a prefix of another comes first.
func compareFullPointers(a, b string) int {
	at, bt := strings.Split(a, "/"), strings.Split(b, "/")
	for i := range min(len(at), len(bt)) {
		ai, aErr := strconv.Atoi(at[i])
		bi, bErr := strconv.Atoi(bt[i])
		if aErr == nil && bErr == nil {
			if ai != bi {
				return ai - bi
			}
			continue
		}
		if c := strings.Compare(at[i], bt[i]); c != 0 {
			return c
		}
	}
	return len(at) - len(bt)
}

// compareFullFindings orders findings by path, then by code.
func compareFullFindings(a, b fullFinding) int {
	if c := compareFullPointers(a.path, b.path); c != 0 {
		return c
	}
	return strings.Compare(string(a.code), string(b.code))
}

// TestValidityFindingsTable checks the table of vectors itself: names are
// unique, inputs decode, requested lists are sorted and distinct, only the
// coverage vectors set snapshotCovers, and every list of expected findings
// is in the documented order. A defect found here is in the table, not in
// the code under test.
func TestValidityFindingsTable(t *testing.T) {
	t.Parallel()

	names := make(map[string]bool, len(fullVectors))
	for _, v := range fullVectors {
		require.False(t, names[v.name], "vector name %q is not unique", v.name)
		names[v.name] = true
		require.NotEmpty(t, v.why, "vector %q has no why", v.name)

		_, err := decodeFullInput(v.input)
		require.NoError(t, err, "vector %q: input must decode", v.name)

		require.NotNil(t, v.requested, "vector %q: requested must be set, if only to an empty list", v.name)
		require.True(t, slices.IsSorted(v.requested), "vector %q: requested is not sorted", v.name)
		require.Len(t, slices.Compact(slices.Clone(v.requested)), len(v.requested),
			"vector %q: requested has a repeated ID", v.name)

		isCoverage := strings.HasPrefix(v.name, "coverage/")
		require.Equal(t, isCoverage, v.snapshotCovers != nil,
			"vector %q: snapshotCovers must be set exactly on coverage vectors", v.name)

		switch want := v.want.(type) {
		case coverageError:
			require.True(t, isCoverage, "vector %q: only coverage vectors expect a coverage error", v.name)
		case fullFindings:
			require.True(t, slices.IsSortedFunc(want, compareFullFindings),
				"vector %q lists its findings out of order", v.name)
		default:
			require.Failf(t, "malformed vector", "vector %q: want must be coverageError or fullFindings, got %T", v.name, v.want)
		}
	}
}

// TestValidityFindingsCodes checks that the vectors use only known codes, and
// that every code the full validator exports is expected by at least one
// vector.
func TestValidityFindingsCodes(t *testing.T) {
	t.Parallel()

	known := make(map[codersdk.GroupSyncFindingCode]bool)
	for _, code := range slices.Concat(allValidityCodes, syntaxCodesPassedThrough) {
		require.False(t, known[code], "code %q is listed twice", code)
		known[code] = true
	}

	used := make(map[codersdk.GroupSyncFindingCode]bool)
	for _, v := range fullVectors {
		want, ok := v.want.(fullFindings)
		if !ok {
			continue
		}
		for _, f := range want {
			require.True(t, known[f.code], "vector %q uses unknown code %q", v.name, f.code)
			used[f.code] = true
		}
	}
	for _, code := range allValidityCodes {
		require.True(t, used[code], "no vector expects code %q", code)
	}
}

// TestCanonicalMappingUUIDs checks CanonicalMappingUUIDs against every vector's requested list.
func TestCanonicalMappingUUIDs(t *testing.T) {
	t.Parallel()

	for _, v := range fullVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			settings, err := decodeFullInput(v.input)
			require.NoError(t, err)

			got := []string{}
			for _, id := range settings.CanonicalMappingUUIDs() {
				got = append(got, id.String())
			}
			require.Equal(t, v.requested, got, v.why)
		})
	}
}

// TestValidityFindings runs every vector through the full validator, with
// the snapshot the vector describes. A vector that expects a coverage error
// must get an error and no findings. Any other vector must get exactly its
// expected findings, in order, on every call, and those findings must include
// everything the syntax validator reports.
func TestValidityFindings(t *testing.T) {
	t.Parallel()

	// repeats is how many times each vector is validated. `mapping` is a Go
	// map, whose iteration order is randomized, so a validator that depends
	// on it can pass one call by luck. Three calls make that unlikely; see the
	// same constant in `codersdk/idpsync_wellformed_test.go`.
	const repeats = 3

	for _, v := range fullVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			settings, err := decodeFullInput(v.input)
			require.NoError(t, err)
			snapshot := buildSnapshot(v)

			switch want := v.want.(type) {
			case coverageError:
				got, err := idpsync.ValidityFindings(settings, snapshot)
				require.Error(t, err, v.why)
				require.Empty(t, got, "findings returned alongside a coverage error")
			case fullFindings:
				syntax := asFullFindings(settings.WellFormedFindings())
				for range repeats {
					got, err := idpsync.ValidityFindings(settings, snapshot)
					require.NoError(t, err, v.why)
					require.Equal(t, want, asFullFindings(got), v.why)
					for _, f := range syntax {
						require.Contains(t, asFullFindings(got), f,
							"the syntax finding %v is missing from the full validator's findings", f)
					}
				}
			default:
				require.Failf(t, "malformed vector", "want must be coverageError or fullFindings, got %T", v.want)
			}
		})
	}
}
