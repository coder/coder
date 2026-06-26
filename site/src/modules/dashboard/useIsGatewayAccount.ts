import type { Organization, User } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "./useDashboard";

/**
 * Built-in role name that grants member workspace operations
 * (create/start/stop/delete/SSH). When this role is absent from every org
 * the user belongs to, and the user has no site roles, the user is a
 * Gateway Account: API-only with no workspace UI.
 */
const WORKSPACE_OPS_ROLE = "organization-workspace-access";

/**
 * MINIMUM_IMPLICIT_MEMBER is the experiment that gates Gateway Accounts.
 */
const MINIMUM_IMPLICIT_MEMBER = "minimum-implicit-member";

/**
 * Returns true when the signed-in user is a Gateway Account: the
 * Gateway Accounts experiment is on, the user has no site roles, and
 * none of their orgs default new members into the workspace-access
 * role.
 *
 * NOTE: This is a frontend heuristic. It does not see explicit per-org
 * role assignments on the OrganizationMember record, so a user who was
 * granted org-workspace-access explicitly will still be treated as a
 * Gateway Account by the UI. The cleanest fix is a backend-provided
 * signal on `/api/v2/users/me` (for example, `is_gateway_account`) that
 * the dashboard layer can read. Until then this heuristic matches the
 * default `default_org_member_roles: []` flow.
 */
export const useIsGatewayAccount = (): boolean => {
	const { user } = useAuthenticated();
	const { experiments, organizations } = useDashboard();
	return isGatewayAccount(user, experiments, organizations);
};

const isGatewayAccount = (
	user: User,
	experiments: readonly string[],
	organizations: readonly Organization[],
): boolean => {
	if (!experiments.includes(MINIMUM_IMPLICIT_MEMBER)) {
		return false;
	}
	if (user.roles.length > 0) {
		return false;
	}
	if (organizations.length === 0) {
		return false;
	}
	return organizations.every(
		(org) => !org.default_org_member_roles.includes(WORKSPACE_OPS_ROLE),
	);
};
