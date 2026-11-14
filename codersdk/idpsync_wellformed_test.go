package codersdk_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

// This file holds the test vectors for the syntax validator of group sync
// settings, which checks the well-formedness conditions W1 through W8 in
// `coderd/idpsync/SYNC_SETTINGS.md`. The tests that use these vectors follow
// them in this file.
//
// Each vector is a JSON text, as a client would send it, and the outcome the
// syntax validator must produce for it. The text is first decoded into
// `codersdk.GroupSyncSettings` the way `httpapi.Read` decodes a request body:
// one JSON value is read, and anything after it is ignored. The validator sees
// only the decoded value.
//
// Whitespace between JSON tokens is insignificant (RFC 8259), so the layout of
// an input has no effect on how it decodes. Long inputs are written as
// multiline raw strings for readability, and short ones on a single line; the
// newlines and tabs inside a raw string are part of the text but change
// nothing. A raw string cannot contain a backtick, so an input that needs one
// must be built by concatenation.
//
// Three UUIDs recur in the vectors. Their values carry no meaning beyond being
// distinct and nonzero, except that C is written with letters so that vectors
// can spell it in upper case.
//
//	11111111-1111-4111-8111-111111111111  ("A")
//	22222222-2222-4222-8222-222222222222  ("B")
//	aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa  ("C")

// syntaxVector is one test vector for the syntax validator.
type syntaxVector struct {
	// name identifies the vector in test output. It is unique within the
	// table.
	name string
	// why says what the vector checks and why its outcome is the right one.
	why string
	// input is the JSON text given to the decoder.
	input string
	// want is the outcome the vector expects.
	want syntaxWant
}

// syntaxWant is the outcome a syntax vector expects: either the text fails to
// decode, or it decodes and the validator reports exactly the listed findings.
type syntaxWant interface{ isSyntaxWant() }

// decodeFails expects the text to be rejected while decoding into
// `codersdk.GroupSyncSettings`. The validator never sees such a text. These
// are rejections the current code already makes.
type decodeFails struct{}

// findings expects the text to decode, and the validator to report exactly
// these findings, in this order. An empty list means the text is well-formed.
// Findings are ordered by path, then by code. Paths compare token by token,
// with member names compared by code point and array indexes compared as
// numbers, so `/mapping/eng/2` precedes `/mapping/eng/10`.
type findings []syntaxFinding

// syntaxFinding is one finding the validator reports.
type syntaxFinding struct {
	// code names the condition violated.
	code codersdk.GroupSyncFindingCode
	// path is a JSON Pointer (RFC 6901) to the offending element of the input.
	// Array indexes refer to positions in the input as written.
	path string
}

func (decodeFails) isSyntaxWant() {}
func (findings) isSyntaxWant()    {}

// syntaxVectors is the table of test vectors for the syntax validator.
//
// The vectors are grouped in this order:
//
//  1. Well-formed texts, which produce no findings.
//  2. Texts that fail to decode. These are the rejections the current code
//     makes before any validation.
//  3. Violations the decoder resolves silently. They break a W condition but
//     cannot be seen in the decoded value, so the validator reports nothing.
//  4. Violations of W4 (`field`).
//  5. Violations of W5 (`mapping`).
//  6. Violations of W6 (`regex_filter`).
//  7. Violations of W8 (`legacy_group_name_mapping`).
//  8. Texts with several findings, which check the order of findings.
var syntaxVectors = []syntaxVector{
	// Group 1: well-formed texts. Each satisfies W1 through W8, so the
	// validator reports nothing. Canonical form is not required.
	{
		name:  "well_formed/empty_object",
		why:   "Every member may be absent, and an absent member is an empty form.",
		input: `{}`,
		want:  findings{},
	},
	{
		name: "well_formed/canonical_empty",
		why:  "The canonical empty form of every member, written out in full.",
		input: `{
			"field": "",
			"mapping": {},
			"regex_filter": null,
			"auto_create_missing_groups": false
		}`,
		want: findings{},
	},
	{
		name: "well_formed/typical",
		why:  "A configured organization: a claim, two mapped IdP groups, a filter both keys match, and auto-create on.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng-a": ["11111111-1111-4111-8111-111111111111"],
				"eng-b": ["22222222-2222-4222-8222-222222222222"]
			},
			"regex_filter": "^eng-",
			"auto_create_missing_groups": true
		}`,
		want: findings{},
	},
	{
		name:  "well_formed/empty_array",
		why:   "W5 allows an array of zero UUIDs. It suppresses that IdP group, which is distinct from an absent member.",
		input: `{"field":"groups","mapping":{"eng":[]}}`,
		want:  findings{},
	},
	{
		name: "well_formed/same_uuid_under_two_keys",
		why:  "W5 requires distinct UUIDs within one array, not across arrays. Two IdP groups may map to the same Coder group.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		want: findings{},
	},
	{
		name:  "well_formed/mapping_null",
		why:   "W5 allows `mapping` to be `null`. It is an empty form, though not the canonical one.",
		input: `{"field":"groups","mapping":null}`,
		want:  findings{},
	},
	{
		name: "well_formed/regex_filter_empty_string",
		why:  "W6 allows any string. The empty string is an empty form, though not the canonical one, and W6's coherence condition applies only to a nonempty string.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": ""
		}`,
		want: findings{},
	},
	{
		name: "well_formed/regex_filter_substring_match",
		why:  "The filter matches by unanchored search, so `eng` matches the key `team-eng`.",
		input: `{
			"field": "groups",
			"mapping": {
				"team-eng": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": "eng"
		}`,
		want: findings{},
	},
	{
		name:  "well_formed/legacy_null",
		why:   "W8 allows `legacy_group_name_mapping` to be `null`.",
		input: `{"field":"groups","legacy_group_name_mapping":null}`,
		want:  findings{},
	},
	{
		name:  "well_formed/legacy_empty_object",
		why:   "W8 allows `legacy_group_name_mapping` to be an empty object.",
		input: `{"field":"groups","legacy_group_name_mapping":{}}`,
		want:  findings{},
	},
	{
		name: "well_formed/field_empty_with_empty_forms",
		why:  "W4 forbids other nonempty members when `field` is empty. Empty forms of the others are allowed, including `regex_filter` as the empty string.",
		input: `{
			"field": "",
			"mapping": null,
			"regex_filter": "",
			"auto_create_missing_groups": false,
			"legacy_group_name_mapping": {}
		}`,
		want: findings{},
	},

	// Group 2: texts that fail to decode. The current code rejects each of
	// these, and relaxed mode is never less strict, so they stay rejected.
	{
		name:  "decode_fails/not_json",
		why:   "W1: the text is not a JSON text.",
		input: `{"field":`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/top_level_array",
		why:   "W1: the value is an array, not an object.",
		input: `[]`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/top_level_string",
		why:   "W1: the value is a string, not an object.",
		input: `"groups"`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/field_number",
		why:   "W4: `field` must be a string.",
		input: `{"field":1}`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/mapping_array",
		why:   "W5: `mapping` must be `null` or an object.",
		input: `{"field":"groups","mapping":[]}`,
		want:  decodeFails{},
	},
	{
		name: "decode_fails/mapping_value_string",
		why:  "W5: a member of `mapping` must be an array.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": "11111111-1111-4111-8111-111111111111"
			}
		}`,
		want: decodeFails{},
	},
	{
		name:  "decode_fails/uuid_unparseable",
		why:   "W5: an array element must be a UUID.",
		input: `{"field":"groups","mapping":{"eng":["not-a-uuid"]}}`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/uuid_empty_string",
		why:   "W5: the empty string is not a UUID.",
		input: `{"field":"groups","mapping":{"eng":[""]}}`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/regex_filter_invalid",
		why:   "W6: a nonempty `regex_filter` must compile as RE2.",
		input: `{"field":"groups","regex_filter":"("}`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/regex_filter_object",
		why:   "W6: `regex_filter` must be `null` or a string.",
		input: `{"field":"groups","regex_filter":{}}`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/auto_create_string",
		why:   "W7: `auto_create_missing_groups` must be `true` or `false`.",
		input: `{"field":"groups","auto_create_missing_groups":"true"}`,
		want:  decodeFails{},
	},
	{
		name:  "decode_fails/legacy_value_number",
		why:   "W8: the legacy mapping's values must be strings, so a number fails to decode before W8 is considered.",
		input: `{"field":"groups","legacy_group_name_mapping":{"a":1}}`,
		want:  decodeFails{},
	},

	// Group 3: violations the decoder resolves silently. Each breaks a W
	// condition, but the decoded value is indistinguishable from a
	// well-formed one, so the validator reports nothing. These vectors pin
	// down that behavior so a change to decoding is noticed.
	{
		name:  "decoder_resolves/top_level_null",
		why:   "W1: `null` is not an object, but it decodes to the zero value without error.",
		input: `null`,
		want:  findings{},
	},
	{
		name:  "decoder_resolves/duplicate_member",
		why:   "W2: a repeated member name decodes with the last value winning. Not reported, per the relaxed-mode section.",
		input: `{"field":"a","field":"groups"}`,
		want:  findings{},
	},
	{
		name: "decoder_resolves/duplicate_mapping_key",
		why:  "W2 applies to every object, `mapping` included. The last array wins, so no repeated UUID is seen.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"],
				"eng": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		want: findings{},
	},
	{
		name:  "decoder_resolves/unknown_member",
		why:   "W3: an unknown member is ignored by the decoder. Not reported, per the relaxed-mode section.",
		input: `{"field":"groups","extra":1}`,
		want:  findings{},
	},
	{
		name:  "decoder_resolves/member_name_case",
		why:   "W3: member names are case-sensitive, but the decoder matches `Field` to `field`.",
		input: `{"Field":"groups"}`,
		want:  findings{},
	},
	{
		name: "decoder_resolves/uuid_uppercase",
		why:  "W5: canonical text form is lowercase, but the decoder parses C in upper case into the same UUID.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"]
			}
		}`,
		want: findings{},
	},
	{
		name: "decoder_resolves/uuid_without_hyphens",
		why:  "W5: canonical text form has hyphens, but the decoder accepts 32 hexadecimal digits.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["11111111111141118111111111111111"]
			}
		}`,
		want: findings{},
	},
	{
		name: "decoder_resolves/uuid_braces",
		why:  "W5: the decoder accepts a UUID in braces.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["{11111111-1111-4111-8111-111111111111}"]
			}
		}`,
		want: findings{},
	},
	{
		name: "decoder_resolves/uuid_urn",
		why:  "W5: the decoder accepts a UUID with a `urn:uuid:` prefix.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["urn:uuid:11111111-1111-4111-8111-111111111111"]
			}
		}`,
		want: findings{},
	},
	{
		name:  "decoder_resolves/trailing_data",
		why:   "W1: the text is more than one JSON value, but the request decoder reads one value and ignores the rest.",
		input: `{"field":"groups"} {`,
		want:  findings{},
	},

	// Group 4: violations of W4. When `field` is empty, every other member
	// must be in an empty form.
	{
		name: "w4/field_empty_mapping_nonempty",
		why:  "A nonempty `mapping` with an empty `field`.",
		input: `{
			"field": "",
			"mapping": {
				"eng": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		want: findings{{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field"}},
	},
	{
		name:  "w4/field_empty_mapping_key_with_empty_array",
		why:   "A `mapping` with one key is nonempty even when the key's array is empty.",
		input: `{"field":"","mapping":{"eng":[]}}`,
		want:  findings{{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field"}},
	},
	{
		name:  "w4/field_absent_regex_filter_set",
		why:   "An absent `field` is empty. A nonempty `regex_filter` with it violates W4.",
		input: `{"regex_filter":"^eng"}`,
		want:  findings{{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field"}},
	},
	{
		name:  "w4/field_absent_auto_create_true",
		why:   "`true` is not an empty form of `auto_create_missing_groups`.",
		input: `{"auto_create_missing_groups":true}`,
		want:  findings{{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field"}},
	},

	// Group 5: violations of W5.
	{
		name: "w5/empty_mapping_key",
		why:  "A member name of `mapping` must be nonempty. The JSON Pointer to the empty key ends in a bare slash.",
		input: `{
			"field": "groups",
			"mapping": {
				"": ["11111111-1111-4111-8111-111111111111"]
			}
		}`,
		want: findings{{code: codersdk.GroupSyncFindingEmptyMappingKey, path: "/mapping/"}},
	},
	{
		name:  "w5/null_array",
		why:   "A member of `mapping` must be an array; `null` decodes without error but is not one.",
		input: `{"field":"groups","mapping":{"eng":null}}`,
		want:  findings{{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/eng"}},
	},
	{
		name: "w5/nil_uuid",
		why:  "The nil UUID is excluded. This is what a mapping-diff pair with no `Gets` produces.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": ["00000000-0000-0000-0000-000000000000"]
			}
		}`,
		want: findings{{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0"}},
	},
	{
		name: "w5/nil_uuid_after_valid",
		why:  "The path gives the nil UUID's position in the input array.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"00000000-0000-0000-0000-000000000000"
				]
			}
		}`,
		want: findings{{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/1"}},
	},
	{
		name: "w5/nil_uuid_repeated",
		why:  "A repeated nil UUID is reported as the nil UUID at each position, never as a repeat.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"00000000-0000-0000-0000-000000000000",
					"00000000-0000-0000-0000-000000000000"
				]
			}
		}`,
		want: findings{
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/0"},
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng/1"},
		},
	},
	{
		name: "w5/repeated_uuid",
		why:  "The second occurrence is the repeat. The first stays as the one copy.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"22222222-2222-4222-8222-222222222222",
					"11111111-1111-4111-8111-111111111111"
				]
			}
		}`,
		want: findings{{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/2"}},
	},
	{
		name: "w5/repeated_uuid_three_times",
		why:  "Every occurrence after the first is a repeat.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111"
				]
			}
		}`,
		want: findings{
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/1"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/2"},
		},
	},
	{
		name: "w5/repeated_uuid_different_spelling",
		why:  "Repeats are judged on the decoded UUID, so C in upper case repeats C in lower case.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
					"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
				]
			}
		}`,
		want: findings{{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/1"}},
	},
	{
		name:  "w5/pointer_escaping",
		why:   "RFC 6901 escapes `~` as `~0` and `/` as `~1` in a JSON Pointer.",
		input: `{"field":"groups","mapping":{"a/b~c":null}}`,
		want:  findings{{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/a~1b~0c"}},
	},

	// Group 6: violations of W6. A nonempty `regex_filter` must match every
	// member name of `mapping`.
	{
		name: "w6/filter_excludes_key",
		why:  "`ops` does not match `^eng-`, so that member can never apply.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng-a": ["11111111-1111-4111-8111-111111111111"],
				"ops": ["22222222-2222-4222-8222-222222222222"]
			},
			"regex_filter": "^eng-"
		}`,
		want: findings{{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops"}},
	},
	{
		name: "w6/filter_excludes_empty_key",
		why:  "A filter that requires a character excludes the empty key, which is also a W5 violation. Both are reported, ordered by code.",
		input: `{
			"field": "groups",
			"mapping": {
				"": ["11111111-1111-4111-8111-111111111111"]
			},
			"regex_filter": "."
		}`,
		want: findings{
			{code: codersdk.GroupSyncFindingEmptyMappingKey, path: "/mapping/"},
			{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/"},
		},
	},

	// Group 7: violations of W8.
	{
		name: "w8/legacy_mapping",
		why:  "A nonempty legacy mapping is not well-formed.",
		input: `{
			"field": "groups",
			"legacy_group_name_mapping": {
				"idp-eng": "eng"
			}
		}`,
		want: findings{{code: codersdk.GroupSyncFindingLegacyMapping, path: "/legacy_group_name_mapping"}},
	},

	// Group 8: texts with several findings. Findings are ordered by path, then
	// by code, regardless of the order of members in the input or the order
	// in which a Go map yields its keys.
	{
		name: "several/two_keys_in_reverse_order",
		why:  "`b` precedes `a` in the input, but `/mapping/a/0` sorts before `/mapping/b`.",
		input: `{
			"field": "groups",
			"mapping": {
				"b": null,
				"a": ["00000000-0000-0000-0000-000000000000"]
			}
		}`,
		want: findings{
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/a/0"},
			{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/b"},
		},
	},
	{
		name: "several/array_indexes_compare_as_numbers",
		why:  "Eleven copies of A give repeats at indexes 1 through 10. Index 10 sorts after 9, not after 1.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng": [
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111"
				]
			}
		}`,
		want: findings{
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/1"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/2"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/3"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/4"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/5"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/6"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/7"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/8"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/9"},
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng/10"},
		},
	},
	{
		name: "several/same_path_two_codes",
		why:  "A null array whose key the filter excludes: two findings at one path, ordered by code.",
		input: `{
			"field": "groups",
			"mapping": {
				"ops": null
			},
			"regex_filter": "^eng"
		}`,
		want: findings{
			{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops"},
			{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/ops"},
		},
	},
	{
		name:  "several/field_empty_and_legacy",
		why:   "An empty `field` with a legacy mapping violates both W4 and W8.",
		input: `{"field":"","legacy_group_name_mapping":{"idp-eng":"eng"}}`,
		want: findings{
			{code: codersdk.GroupSyncFindingFieldEmpty, path: "/field"},
			{code: codersdk.GroupSyncFindingLegacyMapping, path: "/legacy_group_name_mapping"},
		},
	},
	{
		name: "several/one_of_each_mapping_violation",
		why:  "Each W5 and W6 violation at once, to check the validator reports all of them and orders them by path.",
		input: `{
			"field": "groups",
			"mapping": {
				"eng-a": [
					"11111111-1111-4111-8111-111111111111",
					"11111111-1111-4111-8111-111111111111"
				],
				"eng-b": null,
				"eng-c": ["00000000-0000-0000-0000-000000000000"],
				"ops": ["22222222-2222-4222-8222-222222222222"]
			},
			"regex_filter": "^eng-"
		}`,
		want: findings{
			{code: codersdk.GroupSyncFindingRepeatedUUID, path: "/mapping/eng-a/1"},
			{code: codersdk.GroupSyncFindingNullArray, path: "/mapping/eng-b"},
			{code: codersdk.GroupSyncFindingNilUUID, path: "/mapping/eng-c/0"},
			{code: codersdk.GroupSyncFindingFilterExcludedKey, path: "/mapping/ops"},
		},
	},
}

// allSyntaxCodes lists every finding code the syntax validator exports. Go
// cannot enumerate a type's constants, so this list is kept by hand;
// TestWellFormedFindingsCodes checks that every code in it is exercised and
// that the vectors use no code outside it.
var allSyntaxCodes = []codersdk.GroupSyncFindingCode{
	codersdk.GroupSyncFindingFieldEmpty,
	codersdk.GroupSyncFindingEmptyMappingKey,
	codersdk.GroupSyncFindingNullArray,
	codersdk.GroupSyncFindingNilUUID,
	codersdk.GroupSyncFindingRepeatedUUID,
	codersdk.GroupSyncFindingFilterExcludedKey,
	codersdk.GroupSyncFindingLegacyMapping,
}

// decodeSyntaxInput decodes a vector's input the way `httpapi.Read` decodes a
// request body: it reads one JSON value and ignores anything after it.
func decodeSyntaxInput(input string) (codersdk.GroupSyncSettings, error) {
	var s codersdk.GroupSyncSettings
	err := json.NewDecoder(strings.NewReader(input)).Decode(&s)
	return s, err
}

// asSyntaxFindings converts the validator's findings to the vectors'
// representation. The result is never nil, so that it compares equal to an
// empty findings literal.
func asSyntaxFindings(got []codersdk.GroupSyncFinding) findings {
	out := findings{}
	for _, f := range got {
		out = append(out, syntaxFinding{code: f.Code, path: f.Path})
	}
	return out
}

// comparePointers orders two JSON Pointers token by token. Two tokens that
// are both array indexes compare as numbers; otherwise they compare by code
// point. A pointer that is a prefix of another comes first.
func comparePointers(a, b string) int {
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

// compareSyntaxFindings orders findings by path, then by code.
func compareSyntaxFindings(a, b syntaxFinding) int {
	if c := comparePointers(a.path, b.path); c != 0 {
		return c
	}
	return strings.Compare(string(a.code), string(b.code))
}

// TestWellFormedFindingsTable checks the table of vectors itself: names are
// unique, every vector has an expected outcome, and every list of expected
// findings is in the documented order. A defect found here is in the table,
// not in the validator.
func TestWellFormedFindingsTable(t *testing.T) {
	t.Parallel()

	names := make(map[string]bool, len(syntaxVectors))
	for _, v := range syntaxVectors {
		require.False(t, names[v.name], "vector name %q is not unique", v.name)
		names[v.name] = true
		require.NotEmpty(t, v.why, "vector %q has no why", v.name)

		switch want := v.want.(type) {
		case decodeFails:
		case findings:
			require.True(t, slices.IsSortedFunc(want, compareSyntaxFindings),
				"vector %q lists its findings out of order", v.name)
		default:
			require.Failf(t, "malformed vector", "vector %q: want must be decodeFails or findings, got %T", v.name, v.want)
		}
	}
}

// TestWellFormedFindingsCodes checks that the vectors use only the codes the
// validator exports, and that every exported code is expected by at least one
// vector.
func TestWellFormedFindingsCodes(t *testing.T) {
	t.Parallel()

	known := make(map[codersdk.GroupSyncFindingCode]bool, len(allSyntaxCodes))
	for _, code := range allSyntaxCodes {
		require.False(t, known[code], "code %q is listed twice", code)
		known[code] = true
	}

	used := make(map[codersdk.GroupSyncFindingCode]bool)
	for _, v := range syntaxVectors {
		want, ok := v.want.(findings)
		if !ok {
			continue
		}
		for _, f := range want {
			require.True(t, known[f.code], "vector %q uses unknown code %q", v.name, f.code)
			used[f.code] = true
		}
	}
	for code := range known {
		require.True(t, used[code], "no vector expects code %q", code)
	}
}

// TestWellFormedFindings runs every vector through decoding and the syntax
// validator. A vector that expects decodeFails must fail to decode. Any other
// vector must decode, and the validator must report exactly its expected
// findings, in order, on every call.
func TestWellFormedFindings(t *testing.T) {
	t.Parallel()

	// repeats is how many times each vector is validated. `mapping` is a Go
	// map, whose iteration order is randomized, so a validator that depends
	// on it can pass one call by luck: about one time in k for findings under
	// k keys. Across the two vectors with findings under several keys, three
	// calls make a lucky pass roughly one in five hundred per test run.
	const repeats = 3

	for _, v := range syntaxVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			settings, err := decodeSyntaxInput(v.input)
			switch want := v.want.(type) {
			case decodeFails:
				require.Error(t, err, "input decoded, but the vector expects it not to: %s", v.why)
			case findings:
				require.NoError(t, err, "input failed to decode: %s", v.why)
				for range repeats {
					got := asSyntaxFindings(settings.WellFormedFindings())
					require.Equal(t, want, got, v.why)
				}
			default:
				require.Failf(t, "malformed vector", "want must be decodeFails or findings, got %T", v.want)
			}
		})
	}
}
