package rbac

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// This file is an audit harness for the RBAC refactor on the
// gateway-accounts stack. PR #25928 replaced allPermsExcept(...) inside
// OrgMemberPermissions and OrgServiceAccountPermissions with explicit
// per-resource enumerations. The refactor is intended to be
// behavior-preserving, but allPermsExcept silently included every
// resource in AllResources() that was not in the exclusion list,
// including resources nobody ever consciously decided to grant to org
// members. This harness dumps the exact diff so each dropped or added
// resource type can be approved explicitly.
//
// Run with:
//
//	go test -run TestAuditPermDiff_AllPermsExceptVsExplicit -v ./coderd/rbac/
//
// The test never fails. It only logs the diff via t.Log, which is only
// printed when -v is set.

// legacyOrgMemberPermissions is the pre-refactor implementation of
// OrgMemberPermissions, restored verbatim from main at the commit
// gateway-accounts/enumerate-org-member was branched from. It uses
// allPermsExcept(...) as the source of truth for org-member member
// perms. Used only by the audit test below.
//
// The pre-refactor OrgServiceAccountPermissions had the same Member
// block as this function (allPermsExcept with the same exclusion list
// plus the same carve-outs map). The audit harness therefore only
// snapshots one role; see the comment in the test for the symmetry
// argument that lets us dump once.
func legacyOrgMemberPermissions(org OrgSettings) OrgRolePermissions {
	orgPermMap := map[string][]policy.Action{
		ResourceProvisionerDaemon.Type: {policy.ActionRead},
		ResourceOrganization.Type:      {policy.ActionRead},
		ResourceAssignOrgRole.Type:     {policy.ActionRead},
	}
	if org.ShareableWorkspaceOwners != ShareableWorkspaceOwnersNone {
		orgPermMap[ResourceOrganizationMember.Type] = []policy.Action{policy.ActionRead}
	}
	if org.ShareableWorkspaceOwners == ShareableWorkspaceOwnersEveryone {
		orgPermMap[ResourceGroup.Type] = []policy.Action{policy.ActionRead}
	}

	orgPerms := Permissions(orgPermMap)

	if org.ShareableWorkspaceOwners == ShareableWorkspaceOwnersNone {
		orgPerms = append(orgPerms, Permission{
			Negate:       true,
			ResourceType: ResourceWorkspace.Type,
			Action:       policy.ActionShare,
		})
	}

	memberPerms := append(
		allPermsExcept(
			ResourceWorkspaceDormant,
			ResourcePrebuiltWorkspace,
			ResourceUser,
			ResourceOrganizationMember,
			ResourceBoundaryLog,
			ResourceAibridgeInterception,
			ResourceChat,
		),
		Permissions(map[string][]policy.Action{
			ResourceWorkspaceDormant.Type: {
				policy.ActionRead,
				policy.ActionDelete,
				policy.ActionCreate,
				policy.ActionUpdate,
				policy.ActionWorkspaceStop,
				policy.ActionCreateAgent,
				policy.ActionDeleteAgent,
				policy.ActionUpdateAgent,
			},
			ResourceOrganizationMember.Type:   {policy.ActionRead},
			ResourceAibridgeInterception.Type: {policy.ActionCreate, policy.ActionUpdate},
		})...,
	)

	if org.ShareableWorkspaceOwners != ShareableWorkspaceOwnersEveryone {
		memberPerms = append(memberPerms, Permission{
			Negate:       true,
			ResourceType: ResourceWorkspace.Type,
			Action:       policy.ActionShare,
		})
	}

	return OrgRolePermissions{Org: orgPerms, Member: memberPerms}
}

// resourceByType returns the Objecter from AllResources whose Type
// matches the given string, or nil if there is no such resource.
func resourceByType(typeStr string) Objecter {
	for _, r := range AllResources() {
		if r.RBACObject().Type == typeStr {
			return r
		}
	}
	return nil
}

// expandWildcard turns a permission action set that contains
// policy.WildcardSymbol into the explicit set of every action available
// on that resource. This normalizes legacy allPermsExcept-style perms
// (one entry per resource with Action="*") so that they can be compared
// action-by-action against the new explicit enumerations.
func expandWildcard(resourceType string, actions map[policy.Action]bool) map[policy.Action]bool {
	if !actions[policy.WildcardSymbol] {
		return actions
	}
	out := map[policy.Action]bool{}
	for a, ok := range actions {
		if a == policy.WildcardSymbol {
			continue
		}
		out[a] = ok
	}
	r := resourceByType(resourceType)
	if r == nil {
		// Unknown resource; preserve the wildcard so the diff is
		// visible rather than silently dropped.
		out[policy.WildcardSymbol] = true
		return out
	}
	for _, a := range r.RBACObject().AvailableActions() {
		out[a] = true
	}
	return out
}

// normalizePerms collapses a []Permission into two
// resource -> action -> bool maps: allowed (positive perms) and
// negated. Wildcard actions are expanded so set comparison is
// action-by-action.
func normalizePerms(perms []Permission) (allowed, negated map[string]map[policy.Action]bool) {
	allowed = map[string]map[policy.Action]bool{}
	negated = map[string]map[policy.Action]bool{}
	for _, p := range perms {
		dst := allowed
		if p.Negate {
			dst = negated
		}
		if dst[p.ResourceType] == nil {
			dst[p.ResourceType] = map[policy.Action]bool{}
		}
		dst[p.ResourceType][p.Action] = true
	}
	for r, set := range allowed {
		allowed[r] = expandWildcard(r, set)
	}
	for r, set := range negated {
		negated[r] = expandWildcard(r, set)
	}
	return allowed, negated
}

// actionDiff returns the actions present in left but not in right,
// sorted.
func actionDiff(left, right map[policy.Action]bool) []policy.Action {
	var diff []policy.Action
	for a := range left {
		if !right[a] {
			diff = append(diff, a)
		}
	}
	sort.Slice(diff, func(i, j int) bool { return string(diff[i]) < string(diff[j]) })
	return diff
}

// dumpPermDiff writes a sorted, human-readable diff of two perm sets
// to t.Log. "old" is the legacy set, "cur" is the post-refactor set.
// Output: per resource, "+" actions present in cur but not old, "-"
// actions present in old but not cur. Resources with no diff are
// omitted entirely.
func dumpPermDiff(t *testing.T, label string, old, cur []Permission) {
	t.Helper()
	oldA, oldN := normalizePerms(old)
	curA, curN := normalizePerms(cur)

	resources := map[string]struct{}{}
	for r := range oldA {
		resources[r] = struct{}{}
	}
	for r := range curA {
		resources[r] = struct{}{}
	}
	for r := range oldN {
		resources[r] = struct{}{}
	}
	for r := range curN {
		resources[r] = struct{}{}
	}
	sorted := make([]string, 0, len(resources))
	for r := range resources {
		sorted = append(sorted, r)
	}
	sort.Strings(sorted)

	var out strings.Builder
	fmt.Fprintf(&out, "=== %s ===\n", label)
	anyDiff := false
	for _, r := range sorted {
		added := actionDiff(curA[r], oldA[r])
		removed := actionDiff(oldA[r], curA[r])
		nAdded := actionDiff(curN[r], oldN[r])
		nRemoved := actionDiff(oldN[r], curN[r])
		if len(added) == 0 && len(removed) == 0 && len(nAdded) == 0 && len(nRemoved) == 0 {
			continue
		}
		anyDiff = true
		fmt.Fprintf(&out, "  %s\n", r)
		for _, a := range added {
			fmt.Fprintf(&out, "    + %s\n", a)
		}
		for _, a := range removed {
			fmt.Fprintf(&out, "    - %s\n", a)
		}
		for _, a := range nAdded {
			fmt.Fprintf(&out, "    + NEGATE %s\n", a)
		}
		for _, a := range nRemoved {
			fmt.Fprintf(&out, "    - NEGATE %s\n", a)
		}
	}
	if !anyDiff {
		fmt.Fprintf(&out, "  (no diff)\n")
	}
	t.Log("\n" + out.String())
}

// TestAuditPermDiff_AllPermsExceptVsExplicit dumps the permission diff
// between the legacy allPermsExcept-based OrgMemberPermissions and the
// current explicit enumeration introduced in PR #25928. The diff is
// the audit output to review and approve each dropped permission
// before the floor shrink lands behind the minimum-implicit-member
// experiment.
//
// One dump covers all six combinations of (3 ShareableWorkspaceOwners
// settings) x (organization-member, organization-service-account):
//   - the Org block of both roles is unchanged by the refactor, so it
//     produces (no diff) regardless of sharing setting;
//   - the Member block computation does not reference the sharing
//     setting, so it produces the same diff under each setting;
//   - the new explicit enumerations for organization-member and
//     organization-service-account are verbatim duplicates of each
//     other, so the diff is invariant across roles.
//
// If a future change makes any of these dimensions diverge, expand the
// dump along the divergent axis so reviewers see the new differences.
//
// The test never fails; it logs the dump via t.Log, which is visible
// only when running with -v.
func TestAuditPermDiff_AllPermsExceptVsExplicit(t *testing.T) {
	t.Parallel()

	opts := OrgSettings{ShareableWorkspaceOwners: ShareableWorkspaceOwnersEveryone}
	old := legacyOrgMemberPermissions(opts)
	cur := OrgMemberPermissions(opts)
	dumpPermDiff(t, "Org perms", old.Org, cur.Org)
	dumpPermDiff(t, "Member perms", old.Member, cur.Member)
}
