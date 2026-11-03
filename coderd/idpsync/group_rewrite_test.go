package idpsync_test

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

// This file holds the test vectors for the rewriter of group sync settings;
// see the relaxed-mode section of `SYNC_SETTINGS.md` in this directory. The
// tests that use these vectors follow them in this file.
//
// Each vector gives a settings text, the findings the validators report for
// it, and the outcome, settings, and warnings the rewriter must produce. The
// rewriter is given the findings in the vector, not the validators' output,
// so a failure here points at the rewriter alone. The findings agree with what
// the validators' vectors say for the same kind of input.
//
// Every vector is planned with the never-reject fixture action function from
// `group_policy_test.go`: rewritable findings are rewritten and the others are
// kept, so no vector is rejected. Rejection is the policy's concern, tested
// with the policy's vectors.
//
// Settings texts are compared as JSON token sequences: whitespace and the way
// strings are escaped are ignored, and member order is significant. See
// "Equivalence and canonical forms" in `SYNC_SETTINGS.md`. Long texts are
// written as multiline raw strings for readability.
//
// The UUIDs are those of the full validator's vectors:
//
//	99999999-9999-4999-8999-999999999999  organization O, the target, and
//	                                      the ID of O's Everyone group
//	11111111-1111-4111-8111-111111111111  group G, in O
//	22222222-2222-4222-8222-222222222222  group H, in O
//	33333333-3333-4333-8333-333333333333  group X, in another organization
//	44444444-4444-4444-8444-444444444444  M, which names no group

// rewriteVector is one test vector for the rewriter.
type rewriteVector struct {
	// name identifies the vector in test output. It is unique within the
	// table, and its first segment names the vector's group.
	name string
	// why says what the vector checks and why its outcome is the right one.
	why string
	// input is the settings text given to the rewriter, after decoding.
	input string
	// findings are the findings given to the rewriter, in the order the
	// validators report them.
	findings []rewriteFinding
	// wantOutcome is the outcome the rewriter must report.
	wantOutcome string
	// want is the settings the rewriter must return.
	want rewriteWant
	// wantWarnings are the warnings the rewriter must return: one for each
	// finding, in the same order.
	wantWarnings []rewriteWarning
}

// rewriteFinding is one finding given to the rewriter.
type rewriteFinding struct {
	// code names the condition violated.
	code codersdk.GroupSyncFindingCode
	// path is a JSON Pointer (RFC 6901) into the input. Array indexes refer to
	// positions in the input, not in the rewritten settings.
	path string
}

// rewriteWarning is one warning the rewriter returns.
type rewriteWarning struct {
	// code is the finding's code.
	code codersdk.GroupSyncFindingCode
	// path is the finding's path, into the input.
	path string
	// received is the JSON text of the element at path in the input.
	received string
	// accepted is the JSON text of the element in the settings the rewriter
	// returns. It is empty when the rewrite removed the element, and equal to
	// received when the outcome is not rewritten.
	accepted string
}

// Outcomes the rewriter reports: what it did with the settings as a whole.
const (
	// outcomeValid: the input had no findings; the rewriter returned it
	// unchanged, whether or not it was in canonical form.
	outcomeValid = "valid"
	// outcomeRewritten: the rewriter brought the input into valid canonical
	// form.
	outcomeRewritten = "rewritten"
	// outcomeNotRewritten: the rewriter returned the input unchanged.
	outcomeNotRewritten = "not_rewritten"
)

// rewriteWant is the settings a rewriter vector expects: either its input
// unchanged, or an explicit canonical text.
type rewriteWant interface{ isRewriteWant() }

// sameAsInput expects the rewriter to return its input unchanged: the
// settings it returns are equal, as a Go value, to the input as decoded.
type sameAsInput struct{}

// rewrittenTo expects the rewriter to return Text, which is in canonical form.
type rewrittenTo struct{ Text string }

func (sameAsInput) isRewriteWant() {}
func (rewrittenTo) isRewriteWant() {}

// rewriteVectors is the table of test vectors for the rewriter.
//
// The vectors fall into three groups, named by the first segment of each
// vector's name:
//
//  1. valid: valid input, with no findings. The rewriter returns it
//     unchanged, whether or not it is in canonical form, with outcome valid
//     and no warnings. Settings are rewritten only when they need a rewrite
//     rule.
//  2. rewritable: input whose findings all have rewrite rules. The rewriter
//     brings it into valid canonical form, with outcome rewritten and one
//     warning per finding.
//  3. not_rewritten: input with at least one finding the rewriter cannot
//     rewrite. It is returned unchanged, even when other findings could be
//     rewritten, with outcome not rewritten and one warning per finding.
//
// A test checks the table itself before the rewriter runs: that sameAsInput
// appears only with outcomes valid and not rewritten, that rewrittenTo
// appears only with outcome rewritten, that a valid outcome has no findings,
// and that there is one warning per finding, in the same order.
var rewriteVectors = []rewriteVector{
	// Group 1: valid. The input is returned unchanged, in canonical form or
	// not.
	{
		name:         "valid/canonical_empty",
		why:          "The canonical empty form of every member.",
		input:        `{"field":"","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "valid/typical",
		why:  "A configured organization, with members in canonical order and `mapping` keys sorted.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["22222222-2222-4222-8222-222222222222"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": true
		}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "valid/empty_array_and_filter",
		why:  "An empty array, kept because it suppresses that IdP group, and a filter every key matches.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng-a": [],
				"eng-b": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": "^eng-",
			"auto_create_missing_groups": false
		}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "valid/array_order_kept",
		why:  "Array elements keep their order, so H before G is not reordered.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"22222222-2222-4222-8222-222222222222",
					"11111111-1111-4111-8111-111111111111"
				]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "valid/empty_object",
		why:          "Absent members are empty forms, not violations, so the input is not rewritten into canonical form.",
		input:        `{}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "valid/field_only",
		why:          "The other members are absent, which is an empty form of each.",
		input:        `{"field":"groups"}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "valid/mapping_null",
		why:          "`null` is an empty form of `mapping`, though not the canonical one; it is kept.",
		input:        `{"field":"groups","mapping":null,"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "valid/regex_filter_empty_string",
		why:          "The empty string is an empty form of `regex_filter`, though not the canonical one; it is kept. This is what the UI sends when no filter is set.",
		input:        `{"field":"groups","mapping":{},"regex_filter":"","auto_create_missing_groups":false}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "valid/legacy_empty",
		why:          "An empty legacy mapping is permissible; it is kept, though its canonical form is an absent member.",
		input:        `{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false,"legacy_group_name_mapping":{}}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "valid/member_and_key_order",
		why:  "Members out of canonical order and `mapping` keys unsorted. The order of members carries no meaning once decoded, so the input is returned unchanged.",
		input: `{
			"auto_create_missing_groups": true,
			"mapping": {
				"ops": ["22222222-2222-4222-8222-222222222222"],
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"field": "groups"
		}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},

	// Group 2: rewritable. Rewrite rules R1 through R7, one at a time, then
	// together.
	{
		name: "rewritable/r1_nil_uuid",
		why:  "R1 removes the nil UUID and keeps G.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"00000000-0000-0000-0000-000000000000",
					"11111111-1111-4111-8111-111111111111"
				]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0"}},
		wantOutcome: outcomeRewritten,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0", received: `"00000000-0000-0000-0000-000000000000"`, accepted: ""},
		},
	},
	{
		name: "rewritable/r1_only_element",
		why:  "Removing the only element leaves the member with `[]`, which still keeps the name out of N.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["00000000-0000-0000-0000-000000000000"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0"}},
		wantOutcome: outcomeRewritten,
		want:        rewrittenTo{`{"field":"groups","mapping":{"eng":[]},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0", received: `"00000000-0000-0000-0000-000000000000"`, accepted: ""},
		},
	},
	{
		name: "rewritable/r2_repeated_uuid",
		why:  "R2 keeps the first copy of G and removes the repeat, leaving H between them in place.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"22222222-2222-4222-8222-222222222222",
					"11111111-1111-4111-8111-111111111111"
				]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/2"}},
		wantOutcome: outcomeRewritten,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"22222222-2222-4222-8222-222222222222"
				]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/2", received: `"11111111-1111-4111-8111-111111111111"`, accepted: ""},
		},
	},
	{
		name:        "rewritable/r3_null_array",
		why:         "R3 replaces `null` with `[]`. The element is modified, not removed, so accepted is `[]`.",
		input:       `{"field":"groups","mapping":{"eng":null},"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/eng"}},
		wantOutcome: outcomeRewritten,
		want:        rewrittenTo{`{"field":"groups","mapping":{"eng":[]},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/eng", received: `null`, accepted: `[]`},
		},
	},
	{
		name: "rewritable/r4_filter_excluded_key",
		why:  "R4 removes the member the filter excludes, whole.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng-a": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["22222222-2222-4222-8222-222222222222"]
			},
			"regex_filter": "^eng-",
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops"}},
		wantOutcome: outcomeRewritten,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"eng-a": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": "^eng-",
			"auto_create_missing_groups": false
		}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops", received: `["22222222-2222-4222-8222-222222222222"]`, accepted: ""},
		},
	},
	{
		name: "rewritable/r4_subsumes_element_finding",
		why:  "R4 removes the member, which also removes X under it. The V4 finding on X is still reported, with nothing accepted.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng-a": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["33333333-3333-4333-8333-333333333333"]
			},
			"regex_filter": "^eng-",
			"auto_create_missing_groups": false
		}`,
		findings: []rewriteFinding{
			{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops"},
			{code: codersdk.GroupSyncFindingOutsideGroup, path: "/mapping/ops/0"},
		},
		wantOutcome: outcomeRewritten,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"eng-a": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": "^eng-",
			"auto_create_missing_groups": false
		}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops", received: `["33333333-3333-4333-8333-333333333333"]`, accepted: ""},
			{code: codersdk.GroupSyncFindingOutsideGroup, path: "/mapping/ops/0", received: `"33333333-3333-4333-8333-333333333333"`, accepted: ""},
		},
	},
	{
		name: "rewritable/r5_everyone_group",
		why:  "R5 removes O's Everyone group, which T always contains.",
		input: `{
			"field": "groups",
			"mapping": {
				"all": [
					"99999999-9999-4999-8999-999999999999",
					"11111111-1111-4111-8111-111111111111"
				]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingEveryoneGroup, path: "/mapping/all/0"}},
		wantOutcome: outcomeRewritten,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"all": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingEveryoneGroup, path: "/mapping/all/0", received: `"99999999-9999-4999-8999-999999999999"`, accepted: ""},
		},
	},
	{
		name: "rewritable/r6_missing_group",
		why:  "R6 removes M, which names no group. With the never-reject action function it applies on any route.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"44444444-4444-4444-8444-444444444444"
				]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingMissingGroup, path: "/mapping/eng/1"}},
		wantOutcome: outcomeRewritten,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingMissingGroup, path: "/mapping/eng/1", received: `"44444444-4444-4444-8444-444444444444"`, accepted: ""},
		},
	},
	{
		name: "rewritable/r7_outside_group",
		why:  "R7 removes X, a group in another organization.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["33333333-3333-4333-8333-333333333333"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingOutsideGroup, path: "/mapping/eng/0"}},
		wantOutcome: outcomeRewritten,
		want:        rewrittenTo{`{"field":"groups","mapping":{"eng":[]},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingOutsideGroup, path: "/mapping/eng/0", received: `"33333333-3333-4333-8333-333333333333"`, accepted: ""},
		},
	},
	{
		name: "rewritable/several_rules_and_form",
		why:  "R1, R2, R3, R6, and R7 at once, with members out of order and an empty filter. Paths refer to input positions, though removals shift the rest.",
		input: `{
			"regex_filter": "",
			"mapping": {
				"ops": null,
				"eng": [
					"33333333-3333-4333-8333-333333333333",
					"00000000-0000-0000-0000-000000000000",
					"11111111-1111-4111-8111-111111111111",
					"44444444-4444-4444-8444-444444444444",
					"11111111-1111-4111-8111-111111111111"
				]
			},
			"field": "groups"
		}`,
		findings: []rewriteFinding{
			{code: codersdk.GroupSyncFindingOutsideGroup, path: "/mapping/eng/0"},
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/1"},
			{code: codersdk.GroupSyncFindingMissingGroup, path: "/mapping/eng/3"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/4"},
			{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/ops"},
		},
		wantOutcome: outcomeRewritten,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"],
				"ops": []
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingOutsideGroup, path: "/mapping/eng/0", received: `"33333333-3333-4333-8333-333333333333"`, accepted: ""},
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/1", received: `"00000000-0000-0000-0000-000000000000"`, accepted: ""},
			{code: codersdk.GroupSyncFindingMissingGroup, path: "/mapping/eng/3", received: `"44444444-4444-4444-8444-444444444444"`, accepted: ""},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/4", received: `"11111111-1111-4111-8111-111111111111"`, accepted: ""},
			{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/ops", received: `null`, accepted: `[]`},
		},
	},

	// Group 3: not rewritten. One finding without a rewrite rule leaves the
	// whole input unchanged, and every warning has accepted equal to
	// received.
	{
		name: "not_rewritten/legacy_mapping",
		why:  "A legacy mapping has no rewrite rule.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false,
			"legacy_group_name_mapping": {"idp-eng": "eng"}
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingLegacyMapping, path: "/legacy_group_name_mapping"}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingLegacyMapping, path: "/legacy_group_name_mapping", received: `{"idp-eng":"eng"}`, accepted: `{"idp-eng":"eng"}`},
		},
	},
	{
		name: "not_rewritten/field_empty",
		why:  "An empty `field` with a nonempty `mapping` is kept as received by policy.",
		input: `{
			"field": "",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field"}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field", received: `""`, accepted: `""`},
		},
	},
	{
		name: "not_rewritten/empty_mapping_key",
		why:  "An empty key may match an IdP claim, so removing it could change behavior.",
		input: `{
			"field": "groups",
			"mapping": {
				"": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingEmptyMappingKey, path: "/mapping/"}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingEmptyMappingKey, path: "/mapping/", received: `["11111111-1111-4111-8111-111111111111"]`, accepted: `["11111111-1111-4111-8111-111111111111"]`},
		},
	},
	{
		name:        "not_rewritten/organization_deleted",
		why:         "A deleted organization's stored settings have no repair. The finding's path is empty, so received is the whole text.",
		input:       `{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:    []rewriteFinding{{code: codersdk.GroupSyncFindingOrganizationDeleted, path: ""}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{
				code:     codersdk.GroupSyncFindingOrganizationDeleted,
				path:     "",
				received: `{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`,
				accepted: `{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`,
			},
		},
	},
	{
		name: "not_rewritten/no_partial_rewrite",
		why:  "A nil UUID R1 could remove, alongside a legacy mapping with no rule. Nothing is rewritten, not even the nil UUID.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"00000000-0000-0000-0000-000000000000",
					"11111111-1111-4111-8111-111111111111"
				]
			},
			"regex_filter": null,
			"auto_create_missing_groups": false,
			"legacy_group_name_mapping": {"idp-eng": "eng"}
		}`,
		findings: []rewriteFinding{
			{code: codersdk.GroupSyncFindingLegacyMapping, path: "/legacy_group_name_mapping"},
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0"},
		},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: codersdk.GroupSyncFindingLegacyMapping, path: "/legacy_group_name_mapping", received: `{"idp-eng":"eng"}`, accepted: `{"idp-eng":"eng"}`},
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0", received: `"00000000-0000-0000-0000-000000000000"`, accepted: `"00000000-0000-0000-0000-000000000000"`},
		},
	},
}

// jsonTokens returns the JSON token sequence of text, with numbers kept as
// text. Two texts are textually equal, in the sense of `SYNC_SETTINGS.md`,
// when their token sequences are equal: whitespace and the way strings are
// escaped do not appear in the sequence, and member order does.
func jsonTokens(t *testing.T, text string) []json.Token {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var tokens []json.Token
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return tokens
		}
		require.NoError(t, err, "text is not JSON: %s", text)
		tokens = append(tokens, tok)
	}
}

// requireTextuallyEqual fails unless want and got are textually equal.
func requireTextuallyEqual(t *testing.T, want, got string, msgAndArgs ...any) {
	t.Helper()
	require.Equal(t, jsonTokens(t, want), jsonTokens(t, got), msgAndArgs...)
}

// encodeSettings encodes settings with Go's encoder, which writes the
// members in the order the canonical form requires.
func encodeSettings(t *testing.T, settings codersdk.GroupSyncSettings) string {
	t.Helper()
	b, err := json.Marshal(settings)
	require.NoError(t, err)
	return string(b)
}

// elementAt returns the JSON text of the element a JSON Pointer refers to in
// text, re-encoded so that two elements compare by value. The empty pointer
// refers to the whole text.
func elementAt(t *testing.T, text, pointer string) string {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(text), &v))
	if pointer != "" {
		for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
			token = codersdk.UnescapeJSONPointerToken(token)
			switch node := v.(type) {
			case map[string]any:
				v = node[token]
			case []any:
				i, err := strconv.Atoi(token)
				require.NoError(t, err, "pointer %q indexes an array with %q", pointer, token)
				v = node[i]
			default:
				require.Failf(t, "bad pointer", "pointer %q goes through a scalar", pointer)
			}
		}
	}
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// normalizedJSON re-encodes text so that two texts compare by value.
func normalizedJSON(t *testing.T, text string) string {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(text), &v), "not JSON: %s", text)
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// rewriteFindings converts a vector's findings to the validators' type.
func rewriteFindings(v rewriteVector) []codersdk.GroupSyncFinding {
	out := make([]codersdk.GroupSyncFinding, 0, len(v.findings))
	for _, f := range v.findings {
		out = append(out, codersdk.GroupSyncFinding{Code: f.code, Path: f.path})
	}
	return out
}

// snapshotForTypical builds a snapshot of the typical deployment described in
// `group_validity_test.go`, read for exactly the UUIDs in settings.
func snapshotForTypical(settings codersdk.GroupSyncSettings) idpsync.GroupSnapshot {
	requested := []string{}
	for _, id := range settings.CanonicalMappingUUIDs() {
		requested = append(requested, id.String())
	}
	return buildSnapshot(fullVector{requested: requested, snapshot: snapshotTypical})
}

// TestRewriteResultTable checks the table of vectors itself: names are
// unique and begin with their group, expected settings and outcomes go
// together as documented, there is one warning per finding with the same
// code and path, each warning's received value is the element at its path in
// the input, a not-rewritten warning accepts what it received, and every
// expected text is in canonical form. A defect found here is in the table,
// not in the code under test.
func TestRewriteResultTable(t *testing.T) {
	t.Parallel()

	groups := map[string]string{
		outcomeValid:        "valid/",
		outcomeRewritten:    "rewritable/",
		outcomeNotRewritten: "not_rewritten/",
	}

	names := make(map[string]bool, len(rewriteVectors))
	for _, v := range rewriteVectors {
		require.False(t, names[v.name], "vector name %q is not unique", v.name)
		names[v.name] = true
		require.NotEmpty(t, v.why, "vector %q has no why", v.name)

		prefix, ok := groups[v.wantOutcome]
		require.True(t, ok, "vector %q: unknown outcome %q", v.name, v.wantOutcome)
		require.True(t, strings.HasPrefix(v.name, prefix), "vector %q: outcome %q belongs to group %q", v.name, v.wantOutcome, prefix)

		_, err := decodeFullInput(v.input)
		require.NoError(t, err, "vector %q: input must decode", v.name)

		switch want := v.want.(type) {
		case sameAsInput:
			require.NotEqual(t, outcomeRewritten, v.wantOutcome, "vector %q: sameAsInput with outcome rewritten", v.name)
		case rewrittenTo:
			require.Equal(t, outcomeRewritten, v.wantOutcome, "vector %q: rewrittenTo needs outcome rewritten", v.name)
			decoded, err := decodeFullInput(want.Text)
			require.NoError(t, err, "vector %q: expected text must decode", v.name)
			requireTextuallyEqual(t, encodeSettings(t, decoded), want.Text, "vector %q: expected text is not in canonical form", v.name)
		default:
			require.Failf(t, "malformed vector", "vector %q: want must be sameAsInput or rewrittenTo, got %T", v.name, v.want)
		}

		require.Equal(t, v.wantOutcome == outcomeValid, len(v.findings) == 0,
			"vector %q: the outcome is valid exactly when there are no findings", v.name)
		require.Len(t, v.wantWarnings, len(v.findings), "vector %q: one warning per finding", v.name)
		for i, f := range v.findings {
			w := v.wantWarnings[i]
			require.Equal(t, f.code, w.code, "vector %q: warning %d has the wrong code", v.name, i)
			require.Equal(t, f.path, w.path, "vector %q: warning %d has the wrong path", v.name, i)
			require.Equal(t, elementAt(t, v.input, f.path), normalizedJSON(t, w.received),
				"vector %q: warning %d received a value that is not in the input", v.name, i)
			if v.wantOutcome == outcomeNotRewritten {
				require.Equal(t, w.received, w.accepted, "vector %q: warning %d must accept what it received", v.name, i)
			}
		}
	}
}

// TestRewriteResult plans every vector with the never-reject fixture action
// function and carries the plan out. The outcome, settings, and warnings must
// be exactly those the vector expects. A rewritten result must also be valid:
// it has no syntax findings, and no validity findings against the typical
// deployment, which the vectors' UUIDs are drawn from.
func TestRewriteResult(t *testing.T) {
	t.Parallel()

	for _, v := range rewriteVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			settings, err := decodeFullInput(v.input)
			require.NoError(t, err)
			findings := rewriteFindings(v)
			plan := idpsync.RewritePlan(fixtureActions(t, actionsNeverReject), idpsync.GroupSyncRouteFetch, findings)

			result, err := idpsync.RewriteResult(settings, plan)
			require.NoError(t, err, v.why)
			require.Equal(t, idpsync.GroupSyncRewriteOutcome(v.wantOutcome), result.Outcome, v.why)

			switch want := v.want.(type) {
			case sameAsInput:
				require.Equal(t, settings, result.Settings, "the settings must be returned unchanged: %s", v.why)
			case rewrittenTo:
				requireTextuallyEqual(t, want.Text, encodeSettings(t, result.Settings), v.why)
			default:
				require.Failf(t, "malformed vector", "want must be sameAsInput or rewrittenTo, got %T", v.want)
			}

			require.Len(t, result.Warnings, len(v.wantWarnings), v.why)
			for i, want := range v.wantWarnings {
				got := result.Warnings[i]
				require.Equal(t, want.code, got.Code, "warning %d: %s", i, v.why)
				require.Equal(t, want.path, got.Path, "warning %d: %s", i, v.why)
				requireTextuallyEqual(t, want.received, string(got.Received), "warning %d received: %s", i, v.why)
				if want.accepted == "" {
					require.Empty(t, got.Accepted, "warning %d: the element was removed, so nothing is accepted: %s", i, v.why)
				} else {
					requireTextuallyEqual(t, want.accepted, string(got.Accepted), "warning %d accepted: %s", i, v.why)
				}
			}

			if result.Outcome == idpsync.GroupSyncRewriteOutcomeRewritten {
				require.Empty(t, result.Settings.WellFormedFindings(), "a rewrite must be well-formed")
				validity, err := idpsync.ValidityFindings(result.Settings, snapshotForTypical(result.Settings))
				require.NoError(t, err)
				require.Empty(t, validity, "a rewrite must be valid")
			}
		})
	}
}

// TestRewriteResultCallerErrors checks the plans RewriteResult refuses to
// carry out: one that says reject, and one with an instruction that is not an
// instruction at all. Each is a mistake by the caller, so each returns an
// error. A plan cannot pair an action with the wrong finding, since it holds
// them together.
func TestRewriteResultCallerErrors(t *testing.T) {
	t.Parallel()

	settings, err := decodeFullInput(`{"field":"groups","mapping":{"eng":["00000000-0000-0000-0000-000000000000"]}}`)
	require.NoError(t, err)
	finding := codersdk.GroupSyncFinding{Code: codersdk.GroupSyncFindingNilUUID, Path: "/mapping/eng/0"}

	cases := []struct {
		name string
		why  string
		plan idpsync.GroupSyncRewritePlan
	}{
		{
			name: "reject",
			why:  "A plan that says reject leaves nothing to carry out.",
			plan: idpsync.GroupSyncRewritePlan{
				Instruction: idpsync.GroupSyncInstructionReject,
				Actions:     []idpsync.GroupSyncPlannedAction{{Finding: finding, Action: idpsync.GroupSyncActionReject}},
			},
		},
		{
			name: "unknown_instruction",
			why:  "An instruction RewritePlan never gives cannot be carried out.",
			plan: idpsync.GroupSyncRewritePlan{
				Instruction: "unknown_instruction",
				Actions:     []idpsync.GroupSyncPlannedAction{{Finding: finding, Action: idpsync.GroupSyncActionRewrite}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := idpsync.RewriteResult(settings, c.plan)
			require.Error(t, err, c.why)
		})
	}
}
