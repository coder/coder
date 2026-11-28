package idpsync

import (
	"github.com/coder/coder/v2/codersdk"
)

// A rewrite policy for group sync settings has three parts: an action
// function, which picks an action for each finding; the rewritability of each
// finding code, which the specification fixes; and the rule that combines
// the actions for all the findings into one instruction. RewritePlan applies
// the whole policy. The relaxed policy in effect at present is
// RelaxedGroupSyncFindingAction with that rule.

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

// GroupSyncFindingAction is an instruction to the rewriter about one finding.
type GroupSyncFindingAction string

const (
	// GroupSyncFindingActionRewrite: apply the finding's rewrite rule.
	GroupSyncFindingActionRewrite GroupSyncFindingAction = "rewrite"
	// GroupSyncFindingActionKeep: keep the settings as received.
	GroupSyncFindingActionKeep GroupSyncFindingAction = "keep"
	// GroupSyncFindingActionReject: reject the settings.
	GroupSyncFindingActionReject GroupSyncFindingAction = "reject"
)

// GroupSyncSettingsInstruction is an instruction to the rewriter about the
// settings as a whole.
type GroupSyncSettingsInstruction string

const (
	// GroupSyncSettingsInstructionAccept: accept the settings as received.
	// Given when there are no findings.
	GroupSyncSettingsInstructionAccept GroupSyncSettingsInstruction = "accept"
	// GroupSyncSettingsInstructionRewrite: rewrite the settings into valid
	// canonical form. Given when every action is rewrite.
	GroupSyncSettingsInstructionRewrite GroupSyncSettingsInstruction = "rewrite"
	// GroupSyncSettingsInstructionKeep: keep the settings as received. Given
	// when some action is keep and none is reject.
	GroupSyncSettingsInstructionKeep GroupSyncSettingsInstruction = "keep"
	// GroupSyncSettingsInstructionReject: reject the settings. Given when some
	// action is reject.
	GroupSyncSettingsInstructionReject GroupSyncSettingsInstruction = "reject"
)

// GroupSyncFindingActionFunc picks the action for a finding of the given code
// that arrives by the given route. It is consulted once per finding, so it can
// say nothing about settings that have no findings.
type GroupSyncFindingActionFunc func(route GroupSyncRoute, code codersdk.GroupSyncFindingCode) GroupSyncFindingAction

// RelaxedGroupSyncFindingAction is the action function of the relaxed policy,
// which is in effect at present. It keeps the current behavior of the group
// sync endpoints: a finding the current code accepts is rewritten when a
// rewrite rule exists and kept otherwise, and a finding the current code
// rejects stays rejected. The one deliberate change is that V2, V3, and V4
// violations are rejected on the request routes. An unknown code is rejected
// on every route.
func RelaxedGroupSyncFindingAction(route GroupSyncRoute, code codersdk.GroupSyncFindingCode) GroupSyncFindingAction {
	if action, ok := relaxedGroupSyncActions[code][route]; ok {
		return action
	}
	return GroupSyncFindingActionReject
}

// relaxedGroupSyncActions is the relaxed policy's choice of action for each
// finding code on each route. A code or route missing from it is rejected.
var relaxedGroupSyncActions = map[codersdk.GroupSyncFindingCode]map[GroupSyncRoute]GroupSyncFindingAction{
	// R1 through R5: accepted today, and rewriting changes nothing.
	codersdk.GroupSyncFindingNilUUID:           everyRoute(GroupSyncFindingActionRewrite),
	codersdk.GroupSyncFindingRepeatedUUID:      everyRoute(GroupSyncFindingActionRewrite),
	codersdk.GroupSyncFindingNullArray:         everyRoute(GroupSyncFindingActionRewrite),
	codersdk.GroupSyncFindingFilterExcludedKey: everyRoute(GroupSyncFindingActionRewrite),
	codersdk.GroupSyncFindingEveryoneGroup:     everyRoute(GroupSyncFindingActionRewrite),

	// R6 and R7: rejected on the request routes as a deliberate change; a
	// stored row cannot be rejected, so it is rewritten.
	codersdk.GroupSyncFindingMissingGroup: requestOrFetch(GroupSyncFindingActionReject, GroupSyncFindingActionRewrite),
	codersdk.GroupSyncFindingOutsideGroup: requestOrFetch(GroupSyncFindingActionReject, GroupSyncFindingActionRewrite),

	// Accepted today and not rewritable, so kept as received.
	codersdk.GroupSyncFindingFieldEmpty:      everyRoute(GroupSyncFindingActionKeep),
	codersdk.GroupSyncFindingEmptyMappingKey: everyRoute(GroupSyncFindingActionKeep),

	// Rejected today by the full-replace endpoint and copied through by the
	// merging endpoints.
	codersdk.GroupSyncFindingLegacyMapping: {
		GroupSyncRouteFullReplace: GroupSyncFindingActionReject,
		GroupSyncRouteMerge:       GroupSyncFindingActionKeep,
		GroupSyncRouteFetch:       GroupSyncFindingActionKeep,
	},

	// V2: rejected on the request routes as a deliberate change; a stored row
	// cannot be rejected or repaired, so it is kept.
	codersdk.GroupSyncFindingOrganizationDeleted: requestOrFetch(GroupSyncFindingActionReject, GroupSyncFindingActionKeep),
	codersdk.GroupSyncFindingOrganizationAbsent:  requestOrFetch(GroupSyncFindingActionReject, GroupSyncFindingActionKeep),
}

// everyRoute gives the same action on every route.
func everyRoute(action GroupSyncFindingAction) map[GroupSyncRoute]GroupSyncFindingAction {
	return requestOrFetch(action, action)
}

// requestOrFetch gives request on both request routes and fetch on the fetch
// route.
func requestOrFetch(request, fetch GroupSyncFindingAction) map[GroupSyncRoute]GroupSyncFindingAction {
	return map[GroupSyncRoute]GroupSyncFindingAction{
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
		codersdk.GroupSyncFindingEveryoneGroup,     // R5
		codersdk.GroupSyncFindingMissingGroup,      // R6
		codersdk.GroupSyncFindingOutsideGroup:      // R7
		return true
	default:
		return false
	}
}

// GroupSyncPlannedAction is the action a rewrite policy picked for one
// finding. Pairing them means a plan cannot hold an action without its
// finding, or a finding without its action.
type GroupSyncPlannedAction struct {
	// Finding is the finding the action is for.
	Finding codersdk.GroupSyncFinding
	// Action is the action picked for it.
	Action GroupSyncFindingAction
}

// GroupSyncRewritePlan is the result of applying a rewrite policy to a list
// of findings: what the rewriter is to do.
type GroupSyncRewritePlan struct {
	// Instruction is the instruction for the settings as a whole.
	Instruction GroupSyncSettingsInstruction
	// Actions holds each finding with the action picked for it, in the same
	// order as the findings given to RewritePlan.
	Actions []GroupSyncPlannedAction
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
func RewritePlan(actions GroupSyncFindingActionFunc, route GroupSyncRoute, findings []codersdk.GroupSyncFinding) GroupSyncRewritePlan {
	plan := GroupSyncRewritePlan{Actions: make([]GroupSyncPlannedAction, 0, len(findings))}

	anyKeep, anyReject := false, false
	for _, f := range findings {
		action := actions(route, f.Code)
		switch action {
		case GroupSyncFindingActionRewrite:
			// A request to rewrite what no rewrite rule covers is defective.
			if !GroupSyncRewritable(f.Code) {
				action = GroupSyncFindingActionReject
			}
		case GroupSyncFindingActionKeep:
		default:
			// Reject, and any value that is not an action at all, which a
			// defective action function could return.
			action = GroupSyncFindingActionReject
		}
		plan.Actions = append(plan.Actions, GroupSyncPlannedAction{Finding: f, Action: action})
		anyKeep = anyKeep || action == GroupSyncFindingActionKeep
		anyReject = anyReject || action == GroupSyncFindingActionReject
	}

	switch {
	case anyReject:
		plan.Instruction = GroupSyncSettingsInstructionReject
	case anyKeep:
		plan.Instruction = GroupSyncSettingsInstructionKeep
	case len(findings) > 0:
		plan.Instruction = GroupSyncSettingsInstructionRewrite
	default:
		plan.Instruction = GroupSyncSettingsInstructionAccept
	}
	return plan
}
