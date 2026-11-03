package idpsync_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/codersdk"
)

// This file holds the test vectors for the rewrite policy of group sync
// settings, which sits between the validators and the rewriter; see the
// relaxed-mode section of `SYNC_SETTINGS.md` in this directory. A policy has
// three parts: an action function, which picks an action for each finding;
// the rewritability of each finding code, which the specification fixes; and
// the rule that combines the actions into one instruction for the settings
// as a whole. The tests that use these vectors follow them in this file.
//
// The vectors come in three tables:
//
//  1. rewritableVectors: for each finding code, whether one of the rewrite
//     rules R1 through R7 applies to it.
//  2. relaxedActionVectors: the action RelaxedGroupSyncAction picks for
//     every finding code on every route.
//  3. rewritePlanVectors: lists of findings, with a fixture action function
//     and a route, and the instruction and actions RewritePlan must produce.
//
// The finding codes, routes, actions, and instructions are the exported
// constants of codersdk and this package.

// codeUnknown is a finding code that no validator produces. It stands for a
// code added to a validator without an entry in the action function or in
// the rewritability of codes.
const codeUnknown codersdk.GroupSyncFindingCode = "unknown_code"

// rewritableVector is one test vector for GroupSyncRewritable.
type rewritableVector struct {
	// code is the finding code.
	code codersdk.GroupSyncFindingCode
	// rewritable is true when one of the rewrite rules applies to the code.
	rewritable bool
	// why names the rewrite rule, or says why there is none.
	why string
}

// rewritableVectors lists every finding code and whether it can be
// rewritten. A test also checks that every code the validators export
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

// relaxedActionVector is one test vector for RelaxedGroupSyncAction.
type relaxedActionVector struct {
	// code is the finding code.
	code codersdk.GroupSyncFindingCode
	// route is the route the finding arrives by.
	route idpsync.GroupSyncRoute
	// want is the action RelaxedGroupSyncAction must pick.
	want idpsync.GroupSyncAction
	// why says why the action keeps the current behavior, or why the current
	// behavior is deliberately changed.
	why string
}

// relaxedActionVectors gives RelaxedGroupSyncAction's action for every
// finding code on every route, in the order of the rewrite rules, then the
// codes that are not rewritten, then the unknown code. A test checks that
// the table covers every pair of code and route exactly once.
//
// Relaxed mode keeps the current behavior: a finding the current code accepts
// is rewritten when a rule exists and kept otherwise, and a finding the
// current code rejects stays rejected. The one deliberate change is that V2,
// V3, and V4 violations are rejected on the request routes.
var relaxedActionVectors = []relaxedActionVector{
	// R1: the nil UUID. Accepted today, and removing it changes nothing.
	{code: codersdk.GroupSyncFindingNilUUID, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionRewrite, why: "Accepted today; R1 applies."},
	{code: codersdk.GroupSyncFindingNilUUID, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionRewrite, why: "Accepted today, including a mapping-diff pair with no `Gets`; R1 applies."},
	{code: codersdk.GroupSyncFindingNilUUID, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionRewrite, why: "Stored rows can hold it; R1 applies."},

	// R2: a repeated UUID. Accepted today, and T is a set.
	{code: codersdk.GroupSyncFindingRepeatedUUID, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionRewrite, why: "Accepted today; R2 applies."},
	{code: codersdk.GroupSyncFindingRepeatedUUID, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionRewrite, why: "Accepted today; R2 applies."},
	{code: codersdk.GroupSyncFindingRepeatedUUID, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionRewrite, why: "Stored rows can hold it; R2 applies."},

	// R3: a `null` array. Accepted today, and it acts as an empty array.
	{code: codersdk.GroupSyncFindingNullArray, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionRewrite, why: "Accepted today; R3 applies."},
	{code: codersdk.GroupSyncFindingNullArray, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionRewrite, why: "Accepted today; R3 applies."},
	{code: codersdk.GroupSyncFindingNullArray, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionRewrite, why: "Stored rows can hold it; R3 applies."},

	// R4: a key the filter excludes. Accepted today, and the member never
	// applies.
	{code: codersdk.GroupSyncFindingFilterExcludedKey, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionRewrite, why: "Accepted today; R4 applies."},
	{code: codersdk.GroupSyncFindingFilterExcludedKey, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionRewrite, why: "Accepted today, including a filter changed by the config endpoint; R4 applies."},
	{code: codersdk.GroupSyncFindingFilterExcludedKey, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionRewrite, why: "Stored rows can hold it; R4 applies."},

	// R5: the Everyone group. Accepted today, and T always contains it.
	{code: idpsync.GroupSyncFindingEveryoneGroup, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionRewrite, why: "Accepted today, and harmless; R5 applies rather than a new rejection."},
	{code: idpsync.GroupSyncFindingEveryoneGroup, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionRewrite, why: "Accepted today, and harmless; R5 applies rather than a new rejection."},
	{code: idpsync.GroupSyncFindingEveryoneGroup, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionRewrite, why: "Stored rows can hold it; R5 applies."},

	// R6: a UUID that names no group. Rejected on requests as a deliberate
	// change; removed from stored rows.
	{code: idpsync.GroupSyncFindingMissingGroup, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V3."},
	{code: idpsync.GroupSyncFindingMissingGroup, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V3. Stale entries in the stored row were already removed on the read before merging."},
	{code: idpsync.GroupSyncFindingMissingGroup, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionRewrite, why: "A stored row cannot be rejected; R6 applies."},

	// R7: a group in another organization. Rejected on requests as a
	// deliberate change, which is the fix for SEC-183; removed from stored
	// rows.
	{code: idpsync.GroupSyncFindingOutsideGroup, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V4."},
	{code: idpsync.GroupSyncFindingOutsideGroup, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V4."},
	{code: idpsync.GroupSyncFindingOutsideGroup, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionRewrite, why: "A stored row cannot be rejected; R7 applies."},

	// An empty `field` with other nonempty members. Accepted today; kept as
	// received by policy.
	{code: codersdk.GroupSyncFindingFieldEmpty, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionKeep, why: "Accepted today, and kept as received by policy."},
	{code: codersdk.GroupSyncFindingFieldEmpty, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionKeep, why: "Accepted today, including `field` cleared by the config endpoint; kept as received."},
	{code: codersdk.GroupSyncFindingFieldEmpty, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionKeep, why: "Stored rows can hold it; kept as stored."},

	// A legacy mapping. Rejected today by the full-replace endpoint, and
	// copied through today by the merging endpoints.
	{code: codersdk.GroupSyncFindingLegacyMapping, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionReject, why: "Rejected today; relaxed mode is never less strict."},
	{code: codersdk.GroupSyncFindingLegacyMapping, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionKeep, why: "The merging endpoints copy the stored legacy mapping through today; kept as received."},
	{code: codersdk.GroupSyncFindingLegacyMapping, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionKeep, why: "Stored rows can hold it; kept as stored."},

	// An empty mapping key. Accepted today, and not safely removable.
	{code: codersdk.GroupSyncFindingEmptyMappingKey, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionKeep, why: "Accepted today; kept as received."},
	{code: codersdk.GroupSyncFindingEmptyMappingKey, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionKeep, why: "Accepted today; kept as received."},
	{code: codersdk.GroupSyncFindingEmptyMappingKey, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionKeep, why: "Stored rows can hold it; kept as stored."},

	// A deleted organization. Rejected on requests as a deliberate change;
	// a stored row is kept.
	{code: idpsync.GroupSyncFindingOrganizationDeleted, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationDeleted, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationDeleted, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionKeep, why: "A stored row cannot be rejected, and no rule repairs it; kept as stored."},

	// An absent organization. Rejected on requests as a deliberate change;
	// a stored row is kept.
	{code: idpsync.GroupSyncFindingOrganizationAbsent, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationAbsent, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionReject, why: "A deliberate new rejection: V2."},
	{code: idpsync.GroupSyncFindingOrganizationAbsent, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionKeep, why: "A stored row cannot be rejected, and no rule repairs it; kept as stored."},

	// An unknown code. Rejected on every route, so a code added without an
	// entry here fails closed.
	{code: codeUnknown, route: idpsync.GroupSyncRouteFullReplace, want: idpsync.GroupSyncActionReject, why: "Fails closed."},
	{code: codeUnknown, route: idpsync.GroupSyncRouteMerge, want: idpsync.GroupSyncActionReject, why: "Fails closed."},
	{code: codeUnknown, route: idpsync.GroupSyncRouteFetch, want: idpsync.GroupSyncActionReject, why: "Fails closed."},
}

// Fixture action functions used by rewritePlanVectors. A test maps each name
// to a function of type idpsync.GroupSyncActionFunc.
const (
	// actionsRelaxed is RelaxedGroupSyncAction itself.
	actionsRelaxed = "relaxed"
	// actionsNeverReject rewrites every rewritable code and keeps every other
	// code, on every route. The rewriter's tests use it.
	actionsNeverReject = "never_reject"
	// actionsStrict rejects every finding, whatever its code or route. It
	// stands for strict mode with every relaxation removed. Settings with no
	// findings give it nothing to reject.
	actionsStrict = "strict"
	// actionsRewriteAll asks to rewrite every code on every route, including
	// codes that are not rewritable. It stands for a defective action
	// function.
	actionsRewriteAll = "rewrite_all"
)

// rewritePlanVector is one test vector for RewritePlan.
type rewritePlanVector struct {
	// name identifies the vector in test output. It is unique within the
	// table.
	name string
	// why says what the vector checks and why its plan is the right one.
	why string
	// actions names the fixture action function.
	actions string
	// route is the route the findings arrive by.
	route idpsync.GroupSyncRoute
	// codes are the codes of the findings, in the order the validator reports
	// them. RewritePlan does not look at paths, so the vectors give codes
	// only.
	codes []codersdk.GroupSyncFindingCode
	// wantInstruction is the instruction RewritePlan must give.
	wantInstruction idpsync.GroupSyncInstruction
	// wantActions is the action RewritePlan must give for each finding, in the
	// same order as codes.
	wantActions []idpsync.GroupSyncAction
}

// rewritePlanVectors is the table of test vectors for RewritePlan.
//
// The vectors are grouped in this order:
//
//  1. No findings.
//  2. One finding of each action.
//  3. Precedence among actions: reject over keep, keep over rewrite.
//  4. The route changes the action for the same code.
//  5. An action function asks to rewrite a code that is not rewritable.
//  6. The fixture action functions the other tests rely on.
//  7. Order: actions follow the order of the findings.
var rewritePlanVectors = []rewritePlanVector{
	// Group 1: no findings.
	{
		name:            "none/relaxed",
		why:             "With no findings the instruction is accept.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{},
		wantInstruction: idpsync.GroupSyncInstructionAccept,
		wantActions:     []idpsync.GroupSyncAction{},
	},
	{
		name:            "none/strict",
		why:             "The strict action function rejects every finding, so with no findings it rejects nothing: strict form passes strict mode.",
		actions:         actionsStrict,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{},
		wantInstruction: idpsync.GroupSyncInstructionAccept,
		wantActions:     []idpsync.GroupSyncAction{},
	},

	// Group 2: one finding of each action.
	{
		name:            "single/rewrite",
		why:             "One finding the action function rewrites gives the instruction rewrite.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNilUUID},
		wantInstruction: idpsync.GroupSyncInstructionRewrite,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionRewrite},
	},
	{
		name:            "single/keep",
		why:             "One finding the action function keeps gives the instruction keep.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantInstruction: idpsync.GroupSyncInstructionKeep,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionKeep},
	},
	{
		name:            "single/reject",
		why:             "One finding the action function rejects gives the instruction reject.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingOutsideGroup},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionReject},
	},

	// Group 3: precedence. Actions are given per finding; the instruction is
	// set by the strongest one.
	{
		name:            "precedence/keep_over_rewrite",
		why:             "A rewritable finding and a kept one: there is no partial rewrite, so the instruction is keep. The rewritable finding still gets its action.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNilUUID, codersdk.GroupSyncFindingLegacyMapping},
		wantInstruction: idpsync.GroupSyncInstructionKeep,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionRewrite, idpsync.GroupSyncActionKeep},
	},
	{
		name:            "precedence/reject_over_rewrite",
		why:             "A single rejection rejects the settings, whatever else could be rewritten.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingRepeatedUUID, idpsync.GroupSyncFindingMissingGroup},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionRewrite, idpsync.GroupSyncActionReject},
	},
	{
		name:            "precedence/reject_over_keep",
		why:             "A rejection outranks a finding that would keep the settings as received.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingFieldEmpty, idpsync.GroupSyncFindingOrganizationDeleted},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionKeep, idpsync.GroupSyncActionReject},
	},
	{
		name:            "precedence/all_three",
		why:             "Rewrite, keep, and reject together: reject wins.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNullArray, codersdk.GroupSyncFindingEmptyMappingKey, idpsync.GroupSyncFindingOutsideGroup},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionRewrite, idpsync.GroupSyncActionKeep, idpsync.GroupSyncActionReject},
	},

	// Group 4: the route changes the action for the same code.
	{
		name:            "route/legacy_on_full_replace",
		why:             "The full-replace endpoint rejects a legacy mapping today.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionReject},
	},
	{
		name:            "route/legacy_on_merge",
		why:             "The merging endpoints copy a stored legacy mapping through today.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteMerge,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantInstruction: idpsync.GroupSyncInstructionKeep,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionKeep},
	},
	{
		name:            "route/missing_group_on_merge",
		why:             "A missing group in a merged request is rejected.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteMerge,
		codes:           []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingMissingGroup},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionReject},
	},
	{
		name:            "route/missing_group_on_fetch",
		why:             "A missing group in a stored row is removed.",
		actions:         actionsRelaxed,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingMissingGroup},
		wantInstruction: idpsync.GroupSyncInstructionRewrite,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionRewrite},
	},

	// Group 5: an action function asks to rewrite a code that is not
	// rewritable. RewritePlan fails closed by rejecting that finding.
	{
		name:            "unrewritable/legacy",
		why:             "No rule rewrites a legacy mapping, so the defective function's request is turned into a rejection.",
		actions:         actionsRewriteAll,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionReject},
	},
	{
		name:            "unrewritable/with_rewritable",
		why:             "Only the finding without a rule is turned into a rejection; the rewritable one keeps its action, and the instruction is reject.",
		actions:         actionsRewriteAll,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNilUUID, codersdk.GroupSyncFindingEmptyMappingKey},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionRewrite, idpsync.GroupSyncActionReject},
	},
	{
		name:            "unrewritable/unknown_code",
		why:             "An unknown code has no rule, so even a function that asks to rewrite it gets a rejection.",
		actions:         actionsRewriteAll,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{codeUnknown},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionReject},
	},

	// Group 6: the fixture action functions the other tests rely on.
	{
		name:            "fixture/never_reject_on_request",
		why:             "A request with a missing group: the never-reject fixture rewrites it, which is what the rewriter's tests need.",
		actions:         actionsNeverReject,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{idpsync.GroupSyncFindingMissingGroup},
		wantInstruction: idpsync.GroupSyncInstructionRewrite,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionRewrite},
	},
	{
		name:            "fixture/never_reject_unrewritable",
		why:             "The never-reject fixture keeps a code that is not rewritable, so it never trips the rejection in group 5.",
		actions:         actionsNeverReject,
		route:           idpsync.GroupSyncRouteFullReplace,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingLegacyMapping},
		wantInstruction: idpsync.GroupSyncInstructionKeep,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionKeep},
	},
	{
		name:            "fixture/strict",
		why:             "The strict fixture rejects even a finding relaxed mode would rewrite.",
		actions:         actionsStrict,
		route:           idpsync.GroupSyncRouteFetch,
		codes:           []codersdk.GroupSyncFindingCode{codersdk.GroupSyncFindingNullArray},
		wantInstruction: idpsync.GroupSyncInstructionReject,
		wantActions:     []idpsync.GroupSyncAction{idpsync.GroupSyncActionReject},
	},

	// Group 7: order. Actions follow the order of the findings, not the
	// order of the codes or of the actions.
	{
		name:    "order/preserved",
		why:     "Findings in an order unrelated to their codes or actions; each action stays at its finding's position.",
		actions: actionsRelaxed,
		route:   idpsync.GroupSyncRouteMerge,
		codes: []codersdk.GroupSyncFindingCode{
			codersdk.GroupSyncFindingLegacyMapping,
			idpsync.GroupSyncFindingEveryoneGroup,
			codersdk.GroupSyncFindingFieldEmpty,
			codersdk.GroupSyncFindingRepeatedUUID,
		},
		wantInstruction: idpsync.GroupSyncInstructionKeep,
		wantActions: []idpsync.GroupSyncAction{
			idpsync.GroupSyncActionKeep,
			idpsync.GroupSyncActionRewrite,
			idpsync.GroupSyncActionKeep,
			idpsync.GroupSyncActionRewrite,
		},
	},
}

// allGroupSyncRoutes lists every route. Go cannot enumerate a type's
// constants, so this list is kept by hand.
var allGroupSyncRoutes = []idpsync.GroupSyncRoute{
	idpsync.GroupSyncRouteFullReplace,
	idpsync.GroupSyncRouteMerge,
	idpsync.GroupSyncRouteFetch,
}

// rewritableFromTable reports the rewritability that rewritableVectors gives
// a code. The fixture action functions use it rather than the code under
// test, so a defect in GroupSyncRewritable cannot hide in a fixture.
func rewritableFromTable(code codersdk.GroupSyncFindingCode) bool {
	for _, v := range rewritableVectors {
		if v.code == code {
			return v.rewritable
		}
	}
	return false
}

// fixtureActions returns the fixture action function a rewrite plan vector
// names.
func fixtureActions(t *testing.T, name string) idpsync.GroupSyncActionFunc {
	t.Helper()
	switch name {
	case actionsRelaxed:
		return idpsync.RelaxedGroupSyncAction
	case actionsNeverReject:
		return func(_ idpsync.GroupSyncRoute, code codersdk.GroupSyncFindingCode) idpsync.GroupSyncAction {
			if rewritableFromTable(code) {
				return idpsync.GroupSyncActionRewrite
			}
			return idpsync.GroupSyncActionKeep
		}
	case actionsStrict:
		return func(idpsync.GroupSyncRoute, codersdk.GroupSyncFindingCode) idpsync.GroupSyncAction {
			return idpsync.GroupSyncActionReject
		}
	case actionsRewriteAll:
		return func(idpsync.GroupSyncRoute, codersdk.GroupSyncFindingCode) idpsync.GroupSyncAction {
			return idpsync.GroupSyncActionRewrite
		}
	default:
		require.Failf(t, "malformed vector", "unknown fixture action function %q", name)
		return nil
	}
}

// TestGroupSyncPolicyTables checks the three tables themselves and their
// agreement with each other: every exported code appears in the
// rewritability table exactly once, the relaxed-action table covers every
// pair of code and route exactly once and never asks to rewrite a code that
// is not rewritable, and every rewrite plan vector is well formed and agrees
// with the relaxed-action table where it uses RelaxedGroupSyncAction. A
// defect found here is in the tables, not in the code under test.
func TestGroupSyncPolicyTables(t *testing.T) {
	t.Parallel()

	rewritable := make(map[codersdk.GroupSyncFindingCode]bool, len(rewritableVectors))
	for _, v := range rewritableVectors {
		_, dup := rewritable[v.code]
		require.False(t, dup, "code %q appears twice in rewritableVectors", v.code)
		rewritable[v.code] = v.rewritable
		require.NotEmpty(t, v.why, "rewritable vector %q has no why", v.code)
	}
	for _, code := range slices.Concat(syntaxCodesPassedThrough, allValidityCodes) {
		_, ok := rewritable[code]
		require.True(t, ok, "exported code %q is missing from rewritableVectors", code)
	}

	relaxed := make(map[[2]string]idpsync.GroupSyncAction, len(relaxedActionVectors))
	for _, v := range relaxedActionVectors {
		key := [2]string{string(v.code), string(v.route)}
		_, dup := relaxed[key]
		require.False(t, dup, "pair %v appears twice in relaxedActionVectors", key)
		relaxed[key] = v.want
		_, known := rewritable[v.code]
		require.True(t, known, "relaxed vector uses code %q, which rewritableVectors lacks", v.code)
		require.False(t, v.want == idpsync.GroupSyncActionRewrite && !rewritable[v.code],
			"relaxed vector rewrites %q, which is not rewritable", v.code)
		require.NotEmpty(t, v.why, "relaxed vector %v has no why", key)
	}
	for code := range rewritable {
		for _, route := range allGroupSyncRoutes {
			_, ok := relaxed[[2]string{string(code), string(route)}]
			require.True(t, ok, "relaxedActionVectors lacks code %q on route %q", code, route)
		}
	}

	names := make(map[string]bool, len(rewritePlanVectors))
	for _, v := range rewritePlanVectors {
		require.False(t, names[v.name], "vector name %q is not unique", v.name)
		names[v.name] = true
		require.NotEmpty(t, v.why, "vector %q has no why", v.name)
		require.Len(t, v.wantActions, len(v.codes), "vector %q: one action per finding", v.name)
		if v.actions == actionsRelaxed {
			for i, code := range v.codes {
				require.Equal(t, relaxed[[2]string{string(code), string(v.route)}], v.wantActions[i],
					"vector %q: action %d disagrees with relaxedActionVectors", v.name, i)
			}
		}
	}
}

// TestGroupSyncRewritable checks GroupSyncRewritable against every
// rewritability vector.
func TestGroupSyncRewritable(t *testing.T) {
	t.Parallel()

	for _, v := range rewritableVectors {
		t.Run(string(v.code), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, v.rewritable, idpsync.GroupSyncRewritable(v.code), v.why)
		})
	}
}

// TestRelaxedGroupSyncAction checks RelaxedGroupSyncAction against every
// relaxed-action vector.
func TestRelaxedGroupSyncAction(t *testing.T) {
	t.Parallel()

	for _, v := range relaxedActionVectors {
		t.Run(string(v.code)+"/"+string(v.route), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, v.want, idpsync.RelaxedGroupSyncAction(v.route, v.code), v.why)
		})
	}
}

// TestRewritePlan runs every rewrite plan vector through RewritePlan with
// the fixture action function it names. The findings are built from the
// vector's codes, with paths RewritePlan does not read.
func TestRewritePlan(t *testing.T) {
	t.Parallel()

	for _, v := range rewritePlanVectors {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()

			findings := make([]codersdk.GroupSyncFinding, 0, len(v.codes))
			for i, code := range v.codes {
				findings = append(findings, codersdk.GroupSyncFinding{Code: code, Path: "/finding/" + string(rune('a'+i))})
			}

			plan := idpsync.RewritePlan(fixtureActions(t, v.actions), v.route, findings)
			require.Equal(t, v.wantInstruction, plan.Instruction, v.why)
			got := plan.Actions
			if got == nil {
				got = []idpsync.GroupSyncAction{}
			}
			require.Equal(t, v.wantActions, got, v.why)
		})
	}
}
