package idpsync

import (
	"github.com/coder/coder/v2/codersdk"
)

// A rewrite policy for group sync settings has three parts: an action
// function, which picks an action for each finding; the rewritability of each
// finding code, which the specification fixes; and the rule that combines
// the actions for all the findings into one instruction. RewritePlan applies
// the whole policy. The relaxed policy in effect at present is
// RelaxedGroupSyncAction with that rule.

// GroupSyncRoute is the way group sync settings reach the rewriter. The
// names are stable, so that a policy can later be stored, sent over the API,
// and chosen per deployment.
type GroupSyncRoute string

const (
	// GroupSyncRouteFullReplace is the full-replace endpoint, which receives
	// the whole settings text.
	GroupSyncRouteFullReplace GroupSyncRoute = "full_replace"
	// GroupSyncRouteMerge is the config and mapping-diff endpoints, which
	// merge a request into the stored settings.
	GroupSyncRouteMerge GroupSyncRoute = "merge"
	// GroupSyncRouteFetch is a stored row, read by GET or by a PATCH endpoint
	// before merging.
	GroupSyncRouteFetch GroupSyncRoute = "fetch"
)

// GroupSyncAction is an instruction to the rewriter about one finding.
type GroupSyncAction string

const (
	// GroupSyncActionRewrite: apply the finding's rewrite rule.
	GroupSyncActionRewrite GroupSyncAction = "rewrite"
	// GroupSyncActionKeep: keep the settings as received.
	GroupSyncActionKeep GroupSyncAction = "keep"
	// GroupSyncActionReject: reject the settings.
	GroupSyncActionReject GroupSyncAction = "reject"
)

// GroupSyncInstruction is an instruction to the rewriter about the settings
// as a whole.
type GroupSyncInstruction string

const (
	// GroupSyncInstructionAccept: accept the settings, in canonical form.
	// Given when there are no findings.
	GroupSyncInstructionAccept GroupSyncInstruction = "accept"
	// GroupSyncInstructionRewrite: rewrite the settings into valid canonical
	// form. Given when every action is rewrite.
	GroupSyncInstructionRewrite GroupSyncInstruction = "rewrite"
	// GroupSyncInstructionKeep: keep the settings as received. Given when
	// some action is keep and none is reject.
	GroupSyncInstructionKeep GroupSyncInstruction = "keep"
	// GroupSyncInstructionReject: reject the settings. Given when some action
	// is reject.
	GroupSyncInstructionReject GroupSyncInstruction = "reject"
)

// GroupSyncActionFunc picks the action for a finding of the given code that
// arrives by the given route. It is consulted once per finding, so it can
// say nothing about settings that have no findings.
type GroupSyncActionFunc func(route GroupSyncRoute, code codersdk.GroupSyncFindingCode) GroupSyncAction

// RelaxedGroupSyncAction is the action function of the relaxed policy, which
// is in effect at present. It keeps the current behavior of the group sync
// endpoints: a finding the current code accepts is rewritten when a rewrite
// rule exists and kept otherwise, and a finding the current code rejects
// stays rejected. The one deliberate change is that V2, V3, and V4
// violations are rejected on the request routes. An unknown code is rejected
// on every route.
func RelaxedGroupSyncAction(route GroupSyncRoute, code codersdk.GroupSyncFindingCode) GroupSyncAction {
	if action, ok := relaxedGroupSyncActions[code][route]; ok {
		return action
	}
	return GroupSyncActionReject
}

// relaxedGroupSyncActions is the relaxed policy's choice of action for each
// finding code on each route. A code or route missing from it is rejected.
var relaxedGroupSyncActions = map[codersdk.GroupSyncFindingCode]map[GroupSyncRoute]GroupSyncAction{
	// R1 through R5: accepted today, and rewriting changes nothing.
	codersdk.GroupSyncFindingNilUUID:           everyRoute(GroupSyncActionRewrite),
	codersdk.GroupSyncFindingRepeatedUUID:      everyRoute(GroupSyncActionRewrite),
	codersdk.GroupSyncFindingNullArray:         everyRoute(GroupSyncActionRewrite),
	codersdk.GroupSyncFindingFilterExcludedKey: everyRoute(GroupSyncActionRewrite),
	GroupSyncFindingEveryoneGroup:              everyRoute(GroupSyncActionRewrite),

	// R6 and R7: rejected on the request routes as a deliberate change; a
	// stored row cannot be rejected, so it is rewritten.
	GroupSyncFindingMissingGroup: requestOrFetch(GroupSyncActionReject, GroupSyncActionRewrite),
	GroupSyncFindingOutsideGroup: requestOrFetch(GroupSyncActionReject, GroupSyncActionRewrite),

	// Accepted today and not rewritable, so kept as received.
	codersdk.GroupSyncFindingFieldEmpty:      everyRoute(GroupSyncActionKeep),
	codersdk.GroupSyncFindingEmptyMappingKey: everyRoute(GroupSyncActionKeep),

	// Rejected today by the full-replace endpoint and copied through by the
	// merging endpoints.
	codersdk.GroupSyncFindingLegacyMapping: {
		GroupSyncRouteFullReplace: GroupSyncActionReject,
		GroupSyncRouteMerge:       GroupSyncActionKeep,
		GroupSyncRouteFetch:       GroupSyncActionKeep,
	},

	// V2: rejected on the request routes as a deliberate change; a stored row
	// cannot be rejected or repaired, so it is kept.
	GroupSyncFindingOrganizationDeleted: requestOrFetch(GroupSyncActionReject, GroupSyncActionKeep),
	GroupSyncFindingOrganizationAbsent:  requestOrFetch(GroupSyncActionReject, GroupSyncActionKeep),
}

// everyRoute gives the same action on every route.
func everyRoute(action GroupSyncAction) map[GroupSyncRoute]GroupSyncAction {
	return requestOrFetch(action, action)
}

// requestOrFetch gives request on both request routes and fetch on the fetch
// route.
func requestOrFetch(request, fetch GroupSyncAction) map[GroupSyncRoute]GroupSyncAction {
	return map[GroupSyncRoute]GroupSyncAction{
		GroupSyncRouteFullReplace: request,
		GroupSyncRouteMerge:       request,
		GroupSyncRouteFetch:       fetch,
	}
}

// GroupSyncRewritable reports whether findings of the given code can be
// rewritten. The specification fixes this for each code: a code is
// rewritable exactly when one of the rewrite rules R1 through R7 in
// `SYNC_SETTINGS.md` applies to it. No action function can make a code
// rewritable.
func GroupSyncRewritable(code codersdk.GroupSyncFindingCode) bool {
	switch code {
	case codersdk.GroupSyncFindingNilUUID, // R1
		codersdk.GroupSyncFindingRepeatedUUID,      // R2
		codersdk.GroupSyncFindingNullArray,         // R3
		codersdk.GroupSyncFindingFilterExcludedKey, // R4
		GroupSyncFindingEveryoneGroup,              // R5
		GroupSyncFindingMissingGroup,               // R6
		GroupSyncFindingOutsideGroup:               // R7
		return true
	default:
		return false
	}
}

// GroupSyncRewritePlan is the result of applying a rewrite policy to a list
// of findings: what the rewriter is to do.
type GroupSyncRewritePlan struct {
	// Instruction is the instruction for the settings as a whole.
	Instruction GroupSyncInstruction
	// Actions holds the action for each finding, in the same order as the
	// findings given to RewritePlan.
	Actions []GroupSyncAction
}

// RewritePlan applies a rewrite policy, whose action function is actions, to
// findings that arrived by route. It picks an action for each finding and
// combines them into one instruction: reject if any action is reject,
// otherwise keep if any is keep, otherwise rewrite if there are findings,
// otherwise accept.
//
// An action function that asks to rewrite a finding whose code is not
// rewritable, or that returns a value that is not an action, is defective.
// RewritePlan turns that action into reject, so a defective action function
// fails closed.
func RewritePlan(actions GroupSyncActionFunc, route GroupSyncRoute, findings []codersdk.GroupSyncFinding) GroupSyncRewritePlan {
	plan := GroupSyncRewritePlan{Actions: make([]GroupSyncAction, 0, len(findings))}

	anyKeep, anyReject := false, false
	for _, f := range findings {
		action := actions(route, f.Code)
		switch action {
		case GroupSyncActionRewrite:
			// A request to rewrite what no rewrite rule covers is defective.
			if !GroupSyncRewritable(f.Code) {
				action = GroupSyncActionReject
			}
		case GroupSyncActionKeep:
		default:
			// Reject, and any value that is not an action at all, which a
			// defective action function could return.
			action = GroupSyncActionReject
		}
		plan.Actions = append(plan.Actions, action)
		anyKeep = anyKeep || action == GroupSyncActionKeep
		anyReject = anyReject || action == GroupSyncActionReject
	}

	switch {
	case anyReject:
		plan.Instruction = GroupSyncInstructionReject
	case anyKeep:
		plan.Instruction = GroupSyncInstructionKeep
	case len(findings) > 0:
		plan.Instruction = GroupSyncInstructionRewrite
	default:
		plan.Instruction = GroupSyncInstructionAccept
	}
	return plan
}
