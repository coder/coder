import type { Entitlements, Organization } from "#/api/typesGenerated";
import type { Permissions } from "#/modules/permissions";

/**
 * Allotments need deployment config access or group update access in at
 * least one organization.
 */
export const canManageAgentHoursAllotments = (
	permissions: Permissions,
	allotmentOrganizations: readonly Organization[] | undefined,
): boolean =>
	permissions.editDeploymentConfig || (allotmentOrganizations?.length ?? 0) > 0;

/** Navigation also needs Agent Hours to be licensed. */
export const canViewAgentHours = (
	entitlements: Entitlements,
	permissions: Permissions,
	allotmentOrganizations: readonly Organization[] | undefined,
): boolean =>
	entitlements.features.agent_runtime_hours.enabled &&
	canManageAgentHoursAllotments(permissions, allotmentOrganizations);
