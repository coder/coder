import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { canViewOrganization } from "#/modules/permissions/organizations";
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
		showOrganizations,
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
			showOrganizations={showOrganizations}
			hasPremiumLicense={entitlements.features.multiple_organizations.enabled}
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
