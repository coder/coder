package idpsync_test

import (
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

// This file holds the test vectors for the policy that sits between the
// validators and the rewriter of group sync settings. The policy decides, for
// each finding, whether the settings are rewritten, kept as received, or
// rejected; see the relaxed-mode section of `SYNC_SETTINGS.md` in this
// directory. The tests that use these vectors follow them in this file.
//
// The vectors come in three tables:
//
//  1. rewritableVectors: for each finding code, whether the rewriter has a
//     rewrite rule for it. This is a fixed property of the code, not a choice
//     of policy. Decide rejects any finding a policy asks to rewrite when no
//     rule exists.
//  2. relaxedPolicyVectors: the action RelaxedPolicy takes for every finding
//     code on every route.
//  3. decideVectors: lists of findings, with a fixture policy and a route,
//     and the outcome and actions Decide must produce.
//
// Routes, actions, and outcomes have stable string names, so that a policy
// can later be stored, sent over the API, and chosen per deployment.
//
// The finding codes are the exported constants: the syntax validator's in
// codersdk, and the full validator's in this package.

// Routes by which settings reach the rewriter.
const (
	// routeFullReplace is the full-replace endpoint, which receives the whole
	// settings text.
	routeFullReplace = "full_replace"
	// routeMerge is the config and mapping-diff endpoints, which merge a
	// request into the stored settings.
	routeMerge = "merge"
	// routeFetch is a stored row, read by GET or by a PATCH endpoint before
	// merging.
	routeFetch = "fetch"
)

// Actions a policy takes on one finding.
const (
	// actionRewrite applies the finding's rewrite rule.
	actionRewrite = "rewrite"
	// actionKeep keeps the settings as received.
	actionKeep = "keep"
	// actionReject rejects the settings.
	actionReject = "reject"
)

// Outcomes for the settings as a whole.
const (
	outcomeStrictlyValid = "strictly_valid"
	outcomeRewritten     = "rewritten"
	outcomeNotRewritten  = "not_rewritten"
	outcomeRejected      = "rejected"
)

// codeUnknown is a finding code that no validator produces. It stands for a
// code added to a validator without a policy entry or a rewritability entry.
const codeUnknown codersdk.GroupSyncFindingCode = "unknown_code"

// rewritableVector is one test vector for the rewritability of a finding
// code.
type rewritableVector struct {
	// code is the finding code.
	code codersdk.GroupSyncFindingCode
	// rewritable is true when the rewriter has a rewrite rule for the code.
	rewritable bool
	// why names the rewrite rule, or says why there is none.
	why string
}

// rewritableVectors lists every finding code and whether the rewriter can
// rewrite it. A test also checks that every code the validators define
// appears here exactly once.
var rewritableVectors = []rewritableVector{
	{code: codersdk.GroupSyncFindingNilUUID, rewritable: true, why: "R1 removes the nil UUID."},
	{code: codersdk.GroupSyncFindingRepeatedUUID, rewritable: true, why: "R2 keeps one copy of the UUID."},
	{code: codersdk.GroupSyncFindingNullArray, rewritable: true, why: "R3 replaces `null` with `[]`."},
	{code: codersdk.GroupSyncFindingFilterExcludedKey, rewritable: true, why: "R4 removes the member."},
	{code: idpsync.GroupSyncFindingEveryoneGroup, rewritable: true, why: "R5 removes the Everyone group."},
	{code: idpsync.GroupSyncFindingMissingGroup, rewritable: true, why: "R6 removes the UUID."},
	{code: idpsync.GroupSyncFindingOutsideGroup, rewritable: true, why: "R7 removes the UUID."},
	{code: codersdk.GroupSyncFindingFieldEmpty, rewritable: false, why: "No rule is defined. The settings are kept as received by policy."},
	{code: codersdk.GroupSyncFindingLegacyMapping, rewritable: false, why: "A rewrite into strict form is not feasible in general."},
	{code: codersdk.GroupSyncFindingEmptyMappingKey, rewritable: false, why: "An IdP claim can contain the empty string, so removing the key could change behavior."},
	{code: idpsync.GroupSyncFindingOrganizationDeleted, rewritable: false, why: "No omission makes a deleted organization's settings valid."},
	{code: idpsync.GroupSyncFindingOrganizationAbsent, rewritable: false, why: "No omission makes settings for an absent organization valid."},
	{code: codeUnknown, rewritable: false, why: "An unknown code has no rule, so it is never rewritten."},
}

// relaxedPolicyVector is one test vector for RelaxedPolicy.
type relaxedPolicyVector struct {
	// code is the finding code.
	code codersdk.GroupSyncFindingCode
	// route is the route the finding arrives by.
	route string
	// want is the action RelaxedPolicy must return.
	want string
	// why says why the action keeps the current behavior, or why the current
	// behavior is deliberately changed.
	why string
}

// relaxedPolicyVectors gives RelaxedPolicy's action for every finding code on
// every route, in the order of the rewrite rules, then the codes that are not
// rewritten, then the unknown code. A test checks that the table covers every
// pair of code and route exactly once.
//
// Relaxed mode keeps the current behavior: a finding the current code accepts
// is rewritten when a rule exists and kept otherwise, and a finding the
// current code rejects stays rejected. The one deliberate change is that V2,
// V3, and V4 violations are rejected on the request routes.
var relaxedPolicyVectors = []relaxedPolicyVector{
	// R1: the nil UUID. Accepted today, and removing it changes nothing.
	{code: codersdk.GroupSyncFindingNilUUID, route: routeFullReplace, want: actionRewrite, why: "Accepted today; R1 applies."},
	{code: codersdk.GroupSyncFindingNilUUID, route: routeMerge, want: actionRewrite, why: "Accepted today, including a mapping-diff pair with no `Gets`; R1 applies."},
	{code: codersdk.GroupSyncFindingNilUUID, route: routeFetch, want: actionRewrite, why: "Stored rows can hold it; R1 applies."},

	// R2: a repeated UUID. Accepted today, and T is a set.
	{code: codersdk.GroupSyncFindingRepeatedUUID, route: routeFullReplace, want: actionRewrite, why: "Accepted today; R2 applies."},
	{code: codersdk.GroupSyncFindingRepeatedUUID, route: routeMerge, want: actionRewrite, why: "Accepted today; R2 applies."},
	{code: codersdk.GroupSyncFindingRepeatedUUID, route: routeFetch, want: actionRewrite, why: "Stored rows can hold it; R2 applies."},

	// R3: a `null` array. Accepted today, and it acts as an empty array.
	{code: codersdk.GroupSyncFindingNullArray, route: routeFullReplace, want: actionRewrite, why: "Accepted today; R3 applies."},
	{code: codersdk.GroupSyncFindingNullArray, route: routeMerge, want: actionRewrite, why: "Accepted today; R3 applies."},
	{code: codersdk.GroupSyncFindingNullArray, route: routeFetch, want: actionRewrite, why: "Stored rows can hold it; R3 applies."},

	// R4: a key the filter excludes. Accepted today, and the member never
	// applies.
	{code: codersdk.GroupSyncFindingFilterExcludedKey, route: routeFullReplace, want: actionRewrite, why: "Accepted today; R4 applies."},
	{code: codersdk.GroupSyncFindingFilterExcludedKey, route: routeMerge, want: actionRewrite, why: "Accepted today, including a filter changed by the config endpoint; R4 applies."},
	{code: codersdk.GroupSyncFindingFilterExcludedKey, route: routeFetch, want: actionRewrite, why: "Stored rows can hold it; R4 applies."},

	// R5: the Everyone group. Accepted today, and T always contains it.
	{code: idpsync.GroupSyncFindingEveryoneGroup, route: routeFullReplace, want: actionRewrite, why: "Accepted today, and harmless; R5 applies rather than a new rejection."},
	{code: idpsync.GroupSyncFindingEveryoneGroup, route: routeMerge, want: actionRewrite, why: "Accepted today, and harmless; R5 applies rather than a new rejection."},
	{code: idpsync.GroupSyncFindingEveryoneGroup, route: routeFetch, want: actionRewrite, why: "Stored rows can hold it; R5 applies."},

	// R6: a UUID that names no group. Rejected on requests as a deliberate
	// change; removed from stored rows.
	{code: idpsync.GroupSyncFindingMissingGroup, route: routeFullReplace, want: actionReject, why: "A deliberate new rejection: V3."},
	{code: idpsync.GroupSyncFindingMissingGroup, route: routeMerge, want: actionReject, why: "A deliberate new rejection: V3. Stale entries in the stored row were already removed on the read before merging."},
	{code: idpsync.GroupSyncFindingMissingGroup, route: routeFetch, want: actionRewrite, why: "A stored row cannot be rejected; R6 applies."},

	// R7: a group in another organization. Rejected on requests as a
	// deliberate change, which is the fix for SEC-183; removed from stored
	// rows.
	{code: idpsync.GroupSyncFindingOutsideGroup, route: routeFullReplace, want: actionReject, why: "A deliberate new rejection: V4."},
	{code: idpsync.GroupSyncFindingOutsideGroup, route: routeMerge, want: actionReject, why: "A deliberate new rejection: V4."},
	{code: idpsync.GroupSyncFindingOutsideGroup, route: routeFetch, want: actionRewrite, why: "A stored row cannot be rejected; R7 applies."},

	// An empty `field` with other nonempty members. Accepted today; kept as
	// received by policy.
	{code: codersdk.GroupSyncFindingFieldEmpty, route: routeFullReplace, want: actionKeep, why: "Accepted today, and kept as received by policy."},
	{code: codersdk.GroupSyncFindingFieldEmpty, route: routeMerge, want: actionKeep, why: "Accepted today, including `field` cleared by the config endpoint; kept as received."},
	{code: codersdk.GroupSyncFindingFieldEmpty, route: routeFetch, want: actionKeep, why: "Stored rows can hold it; kept as stored."},

	// A legacy mapping. Rejected today by the full-replace endpoint, and
	// copied through today by the merging endpoints.
	{code: codersdk.GroupSyncFindingLegacyMapping, route: routeFullReplace, want: actionReject, why: "Rejected today; relaxed mode is never less strict."},
	{code: codersdk.GroupSyncFindingLegacyMapping, route: routeMerge, want: actionKeep, why: "The merging endpoints copy the stored legacy mapping through today; kept as received."},
	{code: codersdk.GroupSyncFindingLegacyMapping, route: routeFetch, want: actionKeep, why: "Stored rows can hold it; kept as stored."},

	// An empty mapping key. Accepted today, and not safely removable.
	{code: codersdk.GroupSyncFindingEmptyMappingKey, route: routeFullReplace, want: actionKeep, why: "Accepted today; kept as received."},
	{code: codersdk.GroupSyncFindingEmptyMappingKey, route: routeMerge, want: actionKeep, why: "Accepted today; kept as received."},
	{code: codersdk.GroupSyncFindingEmptyMappingKey, route: routeFetch, want: actionKeep, why: "Stored rows can hold it; kept as stored."},

	// A deleted organization. Rejected on requests as a deliberate change;
	// a stored row is kept.
	{code: idpsync.GroupSyncFindingOrganizationDeleted, route: routeFullReplace, want: actionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationDeleted, route: routeMerge, want: actionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationDeleted, route: routeFetch, want: actionKeep, why: "A stored row cannot be rejected, and no rule repairs it; kept as stored."},

	// An absent organization. Rejected on requests as a deliberate change;
	// a stored row is kept.
	{code: idpsync.GroupSyncFindingOrganizationAbsent, route: routeFullReplace, want: actionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationAbsent, route: routeMerge, want: actionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationAbsent, route: routeFetch, want: actionKeep, why: "A stored row cannot be rejected, and no rule repairs it; kept as stored."},

	// An unknown code. Rejected on every route, so a code added without a
	// policy entry fails closed.
	{code: codeUnknown, route: routeFullReplace, want: actionReject, why: "Fails closed."},
	{code: codeUnknown, route: routeMerge, want: actionReject, why: "Fails closed."},
	{code: codeUnknown, route: routeFetch, want: actionReject, why: "Fails closed."},
}

// Fixture policies used by decideVectors. A test maps each name to a function
// of the policy type.
const (
	// policyRelaxed is RelaxedPolicy itself.
	policyRelaxed = "relaxed"
	// policyNeverReject rewrites every rewritable code and keeps every other
	// code, on every route. The rewriter's tests use it.
	policyNeverReject = "never_reject"
	// policyStrict rejects every finding, whatever its code or route. It stands
	// for strict mode with every relaxation removed. Settings with no findings
	// give it nothing to reject.
	policyStrict = "strict"
	// policyRewriteAll asks to rewrite every code on every route, including
	// codes that have no rewrite rule. It stands for a defective policy.
	policyRewriteAll = "rewrite_all"
)

// decideVector is one test vector for Decide.
type decideVector struct {
	// name identifies the vector in test output. It is unique within the
	// table.
	name string
	// why says what the vector checks and why its outcome is the right one.
	why string
	// policy names the fixture policy.
	policy string
	// route is the route the findings arrive by.
	route string
	// codes are the codes of the findings, in the order the validator reports
	// them. Decide does not look at paths, so the vectors give codes only.
	codes []codersdk.GroupSyncFindingCode
	// wantOutcome is the outcome Decide must return.
	wantOutcome string
	// wantActions is the action Decide must return for each finding, in the
	// same order as codes.
	wantActions []string
}

// decideVectors is the table of test vectors for Decide.
//
// The vectors are grouped in this order:
//
//  1. No findings.
//  2. One finding of each action.
//  3. Precedence among actions: reject over keep, keep over rewrite.
//  4. The route changes the action for the same code.
//  5. A policy asks to rewrite a code that has no rewrite rule.
//  6. The fixture policies the other tests rely on.
//  7. Order: actions follow the order of the findings.
var decideVectors = []decideVector{
	// Group 1: no findings.
	{
		name:        "none/relaxed",
		why:         "With no findings the settings are strictly valid.",
		policy:      policyRelaxed,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{},
		wantOutcome: outcomeStrictlyValid,
		wantActions: []string{},
	},
	{
		name:        "none/strict",
		why:         "The strict policy rejects every finding, so with no findings it rejects nothing: strict form passes strict mode.",
		policy:      policyStrict,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{},
		wantOutcome: outcomeStrictlyValid,
		wantActions: []string{},
	},

	// Group 2: one finding of each action.
	{
		name:        "single/rewrite",
		why:         "One rewritable finding the policy rewrites gives a rewritten outcome.",
		policy:      policyRelaxed,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNilUUID},
		wantOutcome: outcomeRewritten,
		wantActions: []string{actionRewrite},
	},
	{
		name:        "single/keep",
		why:         "One finding the policy keeps gives a not-rewritten outcome.",
		policy:      policyRelaxed,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantOutcome: outcomeNotRewritten,
		wantActions: []string{actionKeep},
	},
	{
		name:        "single/reject",
		why:         "One finding the policy rejects gives a rejected outcome.",
		policy:      policyRelaxed,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingOutsideGroup},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionReject},
	},

	// Group 3: precedence. Actions are reported per finding; the outcome is
	// decided by the strongest one.
	{
		name:        "precedence/keep_over_rewrite",
		why:         "A rewritable finding and a kept one: there is no partial rewrite, so the outcome is not rewritten. The rewritable finding still reports its action.",
		policy:      policyRelaxed,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNilUUID, codersdk.GroupSyncFindingLegacyMapping},
		wantOutcome: outcomeNotRewritten,
		wantActions: []string{actionRewrite, actionKeep},
	},
	{
		name:        "precedence/reject_over_rewrite",
		why:         "A single rejection rejects the settings, whatever else could be rewritten.",
		policy:      policyRelaxed,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingRepeatedUUID, idpsync.GroupSyncFindingMissingGroup},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionRewrite, actionReject},
	},
	{
		name:        "precedence/reject_over_keep",
		why:         "A rejection outranks a finding that would keep the settings as received.",
		policy:      policyRelaxed,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingFieldEmpty, idpsync.GroupSyncFindingOrganizationDeleted},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionKeep, actionReject},
	},
	{
		name:        "precedence/all_three",
		why:         "Rewrite, keep, and reject together: reject wins.",
		policy:      policyRelaxed,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNullArray, codersdk.GroupSyncFindingEmptyMappingKey, idpsync.GroupSyncFindingOutsideGroup},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionRewrite, actionKeep, actionReject},
	},

	// Group 4: the route changes the action for the same code.
	{
		name:        "route/legacy_on_full_replace",
		why:         "The full-replace endpoint rejects a legacy mapping today.",
		policy:      policyRelaxed,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionReject},
	},
	{
		name:        "route/legacy_on_merge",
		why:         "The merging endpoints copy a stored legacy mapping through today.",
		policy:      policyRelaxed,
		route:       routeMerge,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantOutcome: outcomeNotRewritten,
		wantActions: []string{actionKeep},
	},
	{
		name:        "route/missing_group_on_merge",
		why:         "A missing group in a merged request is rejected.",
		policy:      policyRelaxed,
		route:       routeMerge,
		codes:       []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingMissingGroup},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionReject},
	},
	{
		name:        "route/missing_group_on_fetch",
		why:         "A missing group in a stored row is removed.",
		policy:      policyRelaxed,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingMissingGroup},
		wantOutcome: outcomeRewritten,
		wantActions: []string{actionRewrite},
	},

	// Group 5: a policy asks to rewrite a code that has no rewrite rule.
	// Decide fails closed by rejecting that finding.
	{
		name:        "unrewritable/legacy",
		why:         "No rule rewrites a legacy mapping, so the defective policy's request is turned into a rejection.",
		policy:      policyRewriteAll,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionReject},
	},
	{
		name:        "unrewritable/with_rewritable",
		why:         "Only the finding without a rule is turned into a rejection; the rewritable one keeps its action, and the outcome is rejected.",
		policy:      policyRewriteAll,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNilUUID, codersdk.GroupSyncFindingEmptyMappingKey},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionRewrite, actionReject},
	},
	{
		name:        "unrewritable/unknown_code",
		why:         "An unknown code has no rule, so even a policy that asks to rewrite it gets a rejection.",
		policy:      policyRewriteAll,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{codeUnknown},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionReject},
	},

	// Group 6: the fixture policies the other tests rely on.
	{
		name:        "fixture/never_reject_on_request",
		why:         "A request with a missing group: the never-reject fixture rewrites it, which is what the rewriter's tests need.",
		policy:      policyNeverReject,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingMissingGroup},
		wantOutcome: outcomeRewritten,
		wantActions: []string{actionRewrite},
	},
	{
		name:        "fixture/never_reject_unrewritable",
		why:         "The never-reject fixture keeps a code that has no rule, so it never trips the rejection in group 5.",
		policy:      policyNeverReject,
		route:       routeFullReplace,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantOutcome: outcomeNotRewritten,
		wantActions: []string{actionKeep},
	},
	{
		name:        "fixture/strict",
		why:         "The strict fixture rejects even a finding relaxed mode would rewrite.",
		policy:      policyStrict,
		route:       routeFetch,
		codes:       []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNullArray},
		wantOutcome: outcomeRejected,
		wantActions: []string{actionReject},
	},

	// Group 7: order. Actions follow the order of the findings, not the
	// order of the codes or of the actions.
	{
		name:   "order/preserved",
		why:    "Findings in an order unrelated to their codes or actions; each action stays at its finding's position.",
		policy: policyRelaxed,
		route:  routeMerge,
		codes: []codersdk.GroupSyncFindingCode{
			codersdk.GroupSyncFindingLegacyMapping,
			idpsync.GroupSyncFindingEveryoneGroup,
			codersdk.GroupSyncFindingFieldEmpty,
			codersdk.GroupSyncFindingRepeatedUUID,
		},
		wantOutcome: outcomeNotRewritten,
		wantActions: []string{actionKeep, actionRewrite, actionKeep, actionRewrite},
	},
}
