import type { Organization, User } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "./useDashboard";

/**
 * Built-in role name that grants member workspace operations
 * (create/start/stop/delete/SSH).
 */
const WORKSPACE_OPS_ROLE = "organization-workspace-access";

/**
 * useCanCreateWorkspace reports whether the signed-in user has the
 * ability to create a workspace anywhere they are a member. The dashboard
 * uses this single signal to decide between the standard experience and
 * the Gateway Account experience (members who can only use AI Gateway
 * endpoints and not workspace operations).
 *
 * IMPORTANT: This is a frontend approximation. It infers ability from
 * `user.roles` and each org's `default_org_member_roles`, which misses
 * explicit per-org role grants on `OrganizationMember`. A user granted
 * `organization-workspace-access` explicitly per-org (rather than via the
 * org default) will be treated here as unable to create a workspace.
 *
 * The accurate fix is a backend-provided boolean on `/api/v2/users/me`
 * (for example, `permissions.createWorkspace` exposed through
 * `site/permissions.json`) computed from the same rbac decision the
 * workspace-create endpoint enforces. When that lands, this hook should
 * collapse to `!permissions.createWorkspace`.
 */
export const useCanCreateWorkspace = (): boolean => {
	const { user } = useAuthenticated();
	const { organizations } = useDashboard();
	return canCreateWorkspace(user, organizations);
};

const canCreateWorkspace = (
	user: User,
	organizations: readonly Organization[],
): boolean => {
	// A site role (owner, user-admin, auditor, etc.) currently implies the
	// ability to reach workspace-create paths in the UI.
	if (user.roles.length > 0) {
		return true;
	}
	// If the user is not a member of any org, default to permitting the
	// standard UX rather than the Gateway Account flow. The org list shown
	// by the dashboard is the orgs the user belongs to.
	if (organizations.length === 0) {
		return true;
	}
	// Workspace-ops elevation comes from at least one org defaulting new
	// members into the workspace-access role.
	return organizations.some((org) =>
		org.default_org_member_roles.includes(WORKSPACE_OPS_ROLE),
	);
};
