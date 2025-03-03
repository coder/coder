import { buildInfo } from "api/queries/buildInfo";
import { useProxy } from "contexts/ProxyContext";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import { canViewDeploymentSettings } from "modules/permissions";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useFeatureVisibility } from "../useFeatureVisibility";
import { NavbarView } from "./NavbarView";

export const Navbar: FC = () => {
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const { appearance, canViewOrganizationSettings } = useDashboard();
	const { user: me, permissions, signOut } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();
	const proxyContextValue = useProxy();

	const canViewDeployment = canViewDeploymentSettings(permissions);
	const canViewOrganizations = canViewOrganizationSettings;
	const canViewHealth = permissions.viewDebugInfo;
	const canViewAuditLog =
		featureVisibility.audit_log && permissions.viewAnyAuditLog;

	return (
		<NavbarView
			user={me}
			logo_url={appearance.logo_url}
			buildInfo={buildInfoQuery.data}
			supportLinks={appearance.support_links}
			onSignOut={signOut}
			canViewDeployment={canViewDeployment}
			canViewOrganizations={canViewOrganizations}
			canViewHealth={canViewHealth}
			canViewAuditLog={canViewAuditLog}
			proxyContextValue={proxyContextValue}
		/>
	);
};
