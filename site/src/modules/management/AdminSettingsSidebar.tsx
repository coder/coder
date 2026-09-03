import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { canViewOrganization } from "#/modules/permissions/organizations";
import { useCanShareOrganizationMCPServers } from "#/pages/AISettingsPage/MCPServersPage/organizationSharing";
import { useAccessibleModelOrganizations } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { AdminSettingsSidebarView } from "./AdminSettingsSidebarView";

/**
 * Wires the unified admin sidebar to authentication, dashboard, and
 * organization data. The organization shown is taken from the
 * `organization` route param when on an organization page, falling
 * back to the default organization, then the first viewable one.
 */
export const AdminSettingsSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const {
		entitlements,
		experiments,
		buildInfo,
		organizations,
		canViewOrganizationSettings,
	} = useDashboard();
	const featureVisibility = useFeatureVisibility();
	const { organization: orgName } = useParams() as { organization?: string };

	const orgPermissionsQuery = useQuery(
		organizationsPermissions(organizations.map((org) => org.id)),
	);
	// Organization-scoped AI access lets non-admins reach the Models and
	// MCP servers pages; these queries dedupe with the AI pages themselves.
	const accessibleModelOrgs = useAccessibleModelOrganizations(organizations);
	const organizationMCPSharing = useCanShareOrganizationMCPServers(
		organizations,
		{ enabled: !permissions.editDeploymentConfig },
	);
	const permissionsByOrgId = orgPermissionsQuery.data;
	const viewableOrganizations = permissionsByOrgId
		? organizations.filter((org) =>
				canViewOrganization(permissionsByOrgId[org.id]),
			)
		: [];
	const activeOrganization =
		viewableOrganizations.find((org) => org.name === orgName) ??
		viewableOrganizations.find((org) => org.is_default) ??
		viewableOrganizations[0];

	return (
		<AdminSettingsSidebarView
			permissions={permissions}
			hidePremiumTab={entitlements.has_license && !entitlements.trial}
			experiments={experiments}
			buildInfo={buildInfo}
			canViewOrganizations={canViewOrganizationSettings}
			organizations={viewableOrganizations}
			activeOrganization={activeOrganization}
			orgPermissions={
				activeOrganization
					? permissionsByOrgId?.[activeOrganization.id]
					: undefined
			}
			canAccessOrganizationModels={accessibleModelOrgs.organizations.length > 0}
			canShareOrganizationMCPServers={organizationMCPSharing.canShare}
			canViewAuditLog={
				featureVisibility.audit_log && permissions.viewAnyAuditLog
			}
			canViewConnectionLog={
				featureVisibility.connection_log && permissions.viewAnyConnectionLog
			}
			canViewAIBridge={
				featureVisibility.aibridge && permissions.viewAnyAIBridgeInterception
			}
		/>
	);
};
