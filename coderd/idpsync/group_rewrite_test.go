package idpsync_test

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
// Every vector uses the never-reject fixture policy from
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
	code string
	// path is a JSON Pointer (RFC 6901) into the input. Array indexes refer to
	// positions in the input, not in the rewritten settings.
	path string
}

// rewriteWarning is one warning the rewriter returns.
type rewriteWarning struct {
	// code is the finding's code.
	code string
	// path is the finding's path, into the input.
	path string
	// received is the JSON text of the element at path in the input.
	received string
	// accepted is the JSON text of the element in the settings the rewriter
	// returns. It is empty when the rewrite removed the element, and equal to
	// received when the outcome is not rewritten.
	accepted string
}

// rewriteWant is the settings a rewriter vector expects: either its input
// unchanged, or an explicit canonical text.
type rewriteWant interface{ isRewriteWant() }

// sameAsInput expects the rewriter to return its input unchanged. The input
// of such a vector must be written as the encoder writes it, so that the
// returned settings, once encoded, compare equal to the input as written; a
// test checks this of every such vector before using it.
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
//  1. fixed_point: input in strict form and in canonical form. The rewriter
//     returns it unchanged, with outcome strictly valid and no warnings.
//  2. rewritable: input the rewriter brings into canonical strict form. With
//     no findings, the input was strictly valid and only its form changes, so
//     the outcome is strictly valid with no warnings. With findings, the
//     outcome is rewritten, with one warning per finding.
//  3. not_rewritten: input with at least one finding the rewriter cannot
//     rewrite. It is returned unchanged, even when other findings could be
//     rewritten, with outcome not rewritten and one warning per finding.
//
// A test checks the table itself before the rewriter runs: that sameAsInput
// appears only with outcomes strictly valid and not rewritten, that
// rewrittenTo appears only with outcomes strictly valid and rewritten, that a
// strictly valid outcome has no findings, and that there is one warning per
// finding, in the same order.
var rewriteVectors = []rewriteVector{
	// Group 1: fixed points.
	{
		name:         "fixed_point/canonical_empty",
		why:          "The canonical empty form of every member.",
		input:        `{"field":"","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeStrictlyValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "fixed_point/typical",
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
		wantOutcome:  outcomeStrictlyValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "fixed_point/empty_array_and_filter",
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
		wantOutcome:  outcomeStrictlyValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "fixed_point/array_order_kept",
		why:  "Canonical form keeps array elements in their original order, so H before G is not reordered.",
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
		wantOutcome:  outcomeStrictlyValid,
		want:         sameAsInput{},
		wantWarnings: []rewriteWarning{},
	},

	// Group 2, first part: strictly valid input in a form that is not
	// canonical. Only the form changes, so there are no warnings.
	{
		name:         "rewritable/empty_object",
		why:          "Absent members become their canonical empty forms; `legacy_group_name_mapping` stays absent.",
		input:        `{}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeStrictlyValid,
		want:         rewrittenTo{`{"field":"","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "rewritable/field_only",
		why:          "The other members are absent and become their canonical empty forms.",
		input:        `{"field":"groups"}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeStrictlyValid,
		want:         rewrittenTo{`{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "rewritable/mapping_null",
		why:          "`null` is an empty form of `mapping`; the canonical one is `{}`.",
		input:        `{"field":"groups","mapping":null,"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeStrictlyValid,
		want:         rewrittenTo{`{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "rewritable/regex_filter_empty_string",
		why:          "The empty string is an empty form of `regex_filter`; the canonical one is `null`. This is what the UI sends when no filter is set.",
		input:        `{"field":"groups","mapping":{},"regex_filter":"","auto_create_missing_groups":false}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeStrictlyValid,
		want:         rewrittenTo{`{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{},
	},
	{
		name:         "rewritable/legacy_empty",
		why:          "An empty legacy mapping is permissible, and its canonical form is an absent member.",
		input:        `{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false,"legacy_group_name_mapping":{}}`,
		findings:     []rewriteFinding{},
		wantOutcome:  outcomeStrictlyValid,
		want:         rewrittenTo{`{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{},
	},
	{
		name: "rewritable/member_and_key_order",
		why:  "Members are put in canonical order and `mapping` keys are sorted by code point.",
		input: `{
			"auto_create_missing_groups": true,
			"mapping": {
				"ops": ["22222222-2222-4222-8222-222222222222"],
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"field": "groups"
		}`,
		findings:    []rewriteFinding{},
		wantOutcome: outcomeStrictlyValid,
		want: rewrittenTo{`{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["22222222-2222-4222-8222-222222222222"]
			},
			"regex_filter": null,
			"auto_create_missing_groups": true
		}`},
		wantWarnings: []rewriteWarning{},
	},

	// Group 2, second part: rewrite rules R1 through R7, one at a time, then
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
		findings:    []rewriteFinding{{code: syntaxCodeNilUUID, path: "/mapping/eng/0"}},
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
			{code: syntaxCodeNilUUID, path: "/mapping/eng/0", received: `"00000000-0000-0000-0000-000000000000"`, accepted: ""},
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
		findings:    []rewriteFinding{{code: syntaxCodeNilUUID, path: "/mapping/eng/0"}},
		wantOutcome: outcomeRewritten,
		want:        rewrittenTo{`{"field":"groups","mapping":{"eng":[]},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{
			{code: syntaxCodeNilUUID, path: "/mapping/eng/0", received: `"00000000-0000-0000-0000-000000000000"`, accepted: ""},
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
		findings:    []rewriteFinding{{code: syntaxCodeRepeatedUUID, path: "/mapping/eng/2"}},
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
			{code: syntaxCodeRepeatedUUID, path: "/mapping/eng/2", received: `"11111111-1111-4111-8111-111111111111"`, accepted: ""},
		},
	},
	{
		name:        "rewritable/r3_null_array",
		why:         "R3 replaces `null` with `[]`. The element is modified, not removed, so accepted is `[]`.",
		input:       `{"field":"groups","mapping":{"eng":null},"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:    []rewriteFinding{{code: syntaxCodeNullArray, path: "/mapping/eng"}},
		wantOutcome: outcomeRewritten,
		want:        rewrittenTo{`{"field":"groups","mapping":{"eng":[]},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{
			{code: syntaxCodeNullArray, path: "/mapping/eng", received: `null`, accepted: `[]`},
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
		findings:    []rewriteFinding{{code: syntaxCodeFilterExcludedKey, path: "/mapping/ops"}},
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
			{code: syntaxCodeFilterExcludedKey, path: "/mapping/ops", received: `["22222222-2222-4222-8222-222222222222"]`, accepted: ""},
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
			{code: syntaxCodeFilterExcludedKey, path: "/mapping/ops"},
			{code: codeOutsideGroup, path: "/mapping/ops/0"},
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
			{code: syntaxCodeFilterExcludedKey, path: "/mapping/ops", received: `["33333333-3333-4333-8333-333333333333"]`, accepted: ""},
			{code: codeOutsideGroup, path: "/mapping/ops/0", received: `"33333333-3333-4333-8333-333333333333"`, accepted: ""},
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
		findings:    []rewriteFinding{{code: codeEveryoneGroup, path: "/mapping/all/0"}},
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
			{code: codeEveryoneGroup, path: "/mapping/all/0", received: `"99999999-9999-4999-8999-999999999999"`, accepted: ""},
		},
	},
	{
		name: "rewritable/r6_missing_group",
		why:  "R6 removes M, which names no group. With the never-reject policy it applies on any route.",
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
		findings:    []rewriteFinding{{code: codeMissingGroup, path: "/mapping/eng/1"}},
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
			{code: codeMissingGroup, path: "/mapping/eng/1", received: `"44444444-4444-4444-8444-444444444444"`, accepted: ""},
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
		findings:    []rewriteFinding{{code: codeOutsideGroup, path: "/mapping/eng/0"}},
		wantOutcome: outcomeRewritten,
		want:        rewrittenTo{`{"field":"groups","mapping":{"eng":[]},"regex_filter":null,"auto_create_missing_groups":false}`},
		wantWarnings: []rewriteWarning{
			{code: codeOutsideGroup, path: "/mapping/eng/0", received: `"33333333-3333-4333-8333-333333333333"`, accepted: ""},
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
			{code: codeOutsideGroup, path: "/mapping/eng/0"},
			{code: syntaxCodeNilUUID, path: "/mapping/eng/1"},
			{code: codeMissingGroup, path: "/mapping/eng/3"},
			{code: syntaxCodeRepeatedUUID, path: "/mapping/eng/4"},
			{code: syntaxCodeNullArray, path: "/mapping/ops"},
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
			{code: codeOutsideGroup, path: "/mapping/eng/0", received: `"33333333-3333-4333-8333-333333333333"`, accepted: ""},
			{code: syntaxCodeNilUUID, path: "/mapping/eng/1", received: `"00000000-0000-0000-0000-000000000000"`, accepted: ""},
			{code: codeMissingGroup, path: "/mapping/eng/3", received: `"44444444-4444-4444-8444-444444444444"`, accepted: ""},
			{code: syntaxCodeRepeatedUUID, path: "/mapping/eng/4", received: `"11111111-1111-4111-8111-111111111111"`, accepted: ""},
			{code: syntaxCodeNullArray, path: "/mapping/ops", received: `null`, accepted: `[]`},
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
		findings:    []rewriteFinding{{code: syntaxCodeLegacyMappingFound, path: "/legacy_group_name_mapping"}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: syntaxCodeLegacyMappingFound, path: "/legacy_group_name_mapping", received: `{"idp-eng":"eng"}`, accepted: `{"idp-eng":"eng"}`},
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
		findings:    []rewriteFinding{{code: syntaxCodeFieldEmpty, path: "/field"}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: syntaxCodeFieldEmpty, path: "/field", received: `""`, accepted: `""`},
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
		findings:    []rewriteFinding{{code: syntaxCodeEmptyMappingKey, path: "/mapping/"}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: syntaxCodeEmptyMappingKey, path: "/mapping/", received: `["11111111-1111-4111-8111-111111111111"]`, accepted: `["11111111-1111-4111-8111-111111111111"]`},
		},
	},
	{
		name:        "not_rewritten/organization_deleted",
		why:         "A deleted organization's stored settings have no repair. The finding's path is empty, so received is the whole text.",
		input:       `{"field":"groups","mapping":{},"regex_filter":null,"auto_create_missing_groups":false}`,
		findings:    []rewriteFinding{{code: codeOrganizationDeleted, path: ""}},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{
				code:     codeOrganizationDeleted,
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
			{code: syntaxCodeLegacyMappingFound, path: "/legacy_group_name_mapping"},
			{code: syntaxCodeNilUUID, path: "/mapping/eng/0"},
		},
		wantOutcome: outcomeNotRewritten,
		want:        sameAsInput{},
		wantWarnings: []rewriteWarning{
			{code: syntaxCodeLegacyMappingFound, path: "/legacy_group_name_mapping", received: `{"idp-eng":"eng"}`, accepted: `{"idp-eng":"eng"}`},
			{code: syntaxCodeNilUUID, path: "/mapping/eng/0", received: `"00000000-0000-0000-0000-000000000000"`, accepted: `"00000000-0000-0000-0000-000000000000"`},
		},
	},
}
