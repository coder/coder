package idpsync_test

// This file holds the test vectors for the full validator of group sync
// settings, which checks the validity conditions V1 through V5 in
// `SYNC_SETTINGS.md` in this directory. V1 is well-formedness, so the full
// validator calls the syntax validator and reports its findings along with
// its own. The tests that use these vectors follow them in this file.
//
// Each vector is a JSON text that decodes into `codersdk.GroupSyncSettings`,
// the organization the settings are for, a description of the state of the
// deployment, and the outcome the full validator must produce. Texts that fail
// to decode belong to the syntax validator's vectors in
// `codersdk/idpsync_wellformed_test.go`; the full validator never sees them.
//
// The state of the deployment is given as a snapshotState. A test turns it
// into a fixture of the same function type as the production snapshot
// function, so the validator under test cannot tell the fixture from the
// database. The validator calls the snapshot function exactly once per
// validation, even when the settings name no UUIDs, because V2 needs the
// organization's state.
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
	// snapshot describes the state of the deployment the validator sees.
	snapshot snapshotState
	// requested lists the UUIDs the validator must ask the snapshot function
	// about: every distinct UUID in `mapping` other than the nil UUID, in
	// ascending order. It is empty when the settings name no UUIDs.
	requested []string
	// want is the outcome the vector expects.
	want fullWant
}

// orgState is the state of the target organization in a snapshot.
type orgState int

const (
	// orgActive: the organization exists and is not deleted.
	orgActive orgState = iota
	// orgDeleted: the organization exists and is soft-deleted.
	orgDeleted
	// orgAbsent: no organization has the target ID.
	orgAbsent
)

// snapshotState describes the state of the deployment a snapshot reports.
// A requested UUID that appears in neither inOrg nor outside names no group.
type snapshotState struct {
	// org is the state of the target organization.
	org orgState
	// inOrg lists the UUIDs of groups that belong to the target organization.
	inOrg []string
	// outside lists the UUIDs of groups that belong to another organization.
	outside []string
	// fails, when true, makes the snapshot function return an error instead
	// of a snapshot.
	fails bool
}

// The snapshot states the vectors use.
var (
	// snapshotTypical is the deployment described at the top of this file:
	// O is active, G, H and O's Everyone group are in O, X is in P, and M
	// names no group.
	snapshotTypical = snapshotState{
		org: orgActive,
		inOrg: []string{
			"11111111-1111-4111-8111-111111111111",
			"22222222-2222-4222-8222-222222222222",
			orgO,
		},
		outside: []string{"33333333-3333-4333-8333-333333333333"},
	}
	// snapshotDeleted is snapshotTypical with O soft-deleted. Deletion
	// requires that O have no groups but Everyone, so G and H name no group.
	snapshotDeleted = snapshotState{
		org:     orgDeleted,
		inOrg:   []string{orgO},
		outside: []string{"33333333-3333-4333-8333-333333333333"},
	}
	// snapshotAbsent has no organization with O's ID, and so no groups in it.
	snapshotAbsent = snapshotState{
		org:     orgAbsent,
		outside: []string{"33333333-3333-4333-8333-333333333333"},
	}
	// snapshotFails makes the snapshot function return an error.
	snapshotFails = snapshotState{fails: true}
)

// fullWant is the outcome a full vector expects: either the snapshot function
// fails and the validator returns its error, or the validator reports exactly
// the listed findings.
type fullWant interface{ isFullWant() }

// snapshotError expects the validator to return the snapshot function's error
// and no findings.
type snapshotError struct{}

// fullFindings expects the validator to report exactly these findings, in
// this order. An empty list means the settings are valid. Findings are
// ordered as the syntax validator orders them: by path, then by code, with
// paths compared token by token and array indexes compared as numbers.
type fullFindings []fullFinding

// fullFinding is one finding the full validator reports.
type fullFinding struct {
	// code names the condition violated.
	code string
	// path is a JSON Pointer (RFC 6901) to the offending element of the input.
	// A finding about the organization, which is not in the text, has the
	// empty path, which refers to the whole text.
	path string
}

func (snapshotError) isFullWant() {}
func (fullFindings) isFullWant()  {}

// Finding codes reported by the full validator. Each array element of
// `mapping` has at most one finding: an element the syntax validator reports
// as the nil UUID or as a repeat gets no V3, V4, or V5 finding.
const (
	// codeOrganizationDeleted: the target organization is soft-deleted (V2).
	codeOrganizationDeleted = "organization_deleted"
	// codeOrganizationAbsent: no organization has the target ID (V2).
	codeOrganizationAbsent = "organization_absent"
	// codeMissingGroup: a `mapping` array contains a UUID that names no group
	// (V3).
	codeMissingGroup = "missing_group"
	// codeOutsideGroup: a `mapping` array contains a UUID that names a group
	// in another organization (V4).
	codeOutsideGroup = "outside_group"
	// codeEveryoneGroup: a `mapping` array contains the target organization's
	// Everyone group (V5).
	codeEveryoneGroup = "everyone_group"
)

// Finding codes reported by the syntax validator and passed through by the
// full validator. They are defined in `codersdk/idpsync_wellformed_test.go`;
// these vectors use the ones they need.
const (
	syntaxCodeFieldEmpty         = "field_empty"
	syntaxCodeNilUUID            = "nil_uuid"
	syntaxCodeRepeatedUUID       = "repeated_uuid"
	syntaxCodeFilterExcludedKey  = "filter_excluded_key"
	syntaxCodeEmptyMappingKey    = "empty_mapping_key"
	syntaxCodeLegacyMappingFound = "legacy_mapping"
)

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
//  7. A failing snapshot function.
//  8. Settings with several findings, which check the order of findings.
var fullVectors = []fullVector{
	// Group 1: valid settings. Each is well-formed and satisfies V2 through
	// V5 against its snapshot.
	{
		name:      "valid/empty_object",
		why:       "Settings that name no UUIDs are valid for an active organization. The snapshot is still consulted, for V2.",
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
		want:      fullFindings{{code: codeOrganizationDeleted, path: ""}},
	},
	{
		name:      "v2/organization_absent",
		why:       "Settings for an organization ID that names no organization violate V2.",
		input:     `{"field":"groups"}`,
		snapshot:  snapshotAbsent,
		requested: []string{},
		want:      fullFindings{{code: codeOrganizationAbsent, path: ""}},
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
			{code: codeOrganizationDeleted, path: ""},
			{code: codeMissingGroup, path: "/mapping/eng/0"},
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
		want:      fullFindings{{code: codeMissingGroup, path: "/mapping/eng/0"}},
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
		want: fullFindings{{code: codeMissingGroup, path: "/mapping/eng/1"}},
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
			{code: codeMissingGroup, path: "/mapping/eng/0"},
			{code: codeMissingGroup, path: "/mapping/ops/0"},
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
		want:      fullFindings{{code: codeOutsideGroup, path: "/mapping/eng/0"}},
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
			org:     orgActive,
			inOrg:   []string{orgO},
			outside: []string{"88888888-8888-4888-8888-888888888888"},
		},
		requested: []string{"88888888-8888-4888-8888-888888888888"},
		want:      fullFindings{{code: codeOutsideGroup, path: "/mapping/eng/0"}},
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
		want:      fullFindings{{code: codeEveryoneGroup, path: "/mapping/all/0"}},
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
			{code: codeOrganizationDeleted, path: ""},
			{code: codeEveryoneGroup, path: "/mapping/all/0"},
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
		want:      fullFindings{{code: syntaxCodeLegacyMappingFound, path: "/legacy_group_name_mapping"}},
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
		want:      fullFindings{{code: syntaxCodeNilUUID, path: "/mapping/eng/0"}},
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
			{code: codeMissingGroup, path: "/mapping/eng/0"},
			{code: syntaxCodeRepeatedUUID, path: "/mapping/eng/1"},
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
			{code: syntaxCodeFilterExcludedKey, path: "/mapping/ops"},
			{code: codeOutsideGroup, path: "/mapping/ops/0"},
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
			{code: syntaxCodeEmptyMappingKey, path: "/mapping/"},
			{code: codeMissingGroup, path: "/mapping//0"},
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
			{code: syntaxCodeFieldEmpty, path: "/field"},
			{code: codeMissingGroup, path: "/mapping/eng/0"},
		},
	},

	// Group 7: a failing snapshot function. The validator returns the error
	// and no findings, even when the syntax validator has findings to report.
	{
		name:      "snapshot_fails/no_uuids",
		why:       "The snapshot is consulted even with no UUIDs, so its failure is seen.",
		input:     `{"field":"groups"}`,
		snapshot:  snapshotFails,
		requested: []string{},
		want:      snapshotError{},
	},
	{
		name: "snapshot_fails/with_syntax_findings",
		why:  "A failure discards the syntax findings too, so a caller never acts on a partial result.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"00000000-0000-0000-0000-000000000000",
					"11111111-1111-4111-8111-111111111111"
				]
			}
		}`,
		snapshot:  snapshotFails,
		requested: []string{"11111111-1111-4111-8111-111111111111"},
		want:      snapshotError{},
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
			{code: codeMissingGroup, path: "/mapping/eng/0"},
			{code: codeEveryoneGroup, path: "/mapping/eng/2"},
			{code: codeOutsideGroup, path: "/mapping/ops/0"},
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
			{code: codeOutsideGroup, path: "/mapping/eng/0"},
			{code: syntaxCodeNilUUID, path: "/mapping/eng/1"},
			{code: codeMissingGroup, path: "/mapping/eng/2"},
			{code: syntaxCodeRepeatedUUID, path: "/mapping/eng/3"},
		},
	},
}
