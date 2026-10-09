import type { Entitlements, Organization } from "#/api/typesGenerated";
import type { Permissions } from "#/modules/permissions";

/**
 * The page needs Agent Hours to be licensed, plus either deployment config
 * access or group update access in at least one organization.
 */
export const canViewAgentHours = (
	entitlements: Entitlements,
	permissions: Permissions,
	allotmentOrganizations: readonly Organization[] | undefined,
): boolean =>
	entitlements.features.agent_runtime_hours.enabled &&
	(permissions.editDeploymentConfig ||
		(allotmentOrganizations?.length ?? 0) > 0);
