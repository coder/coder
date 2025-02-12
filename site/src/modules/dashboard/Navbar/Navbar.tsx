import { buildInfo } from "api/queries/buildInfo";
import { useProxy } from "contexts/ProxyContext";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useFeatureVisibility } from "../useFeatureVisibility";
import { NavbarView } from "./NavbarView";

export const Navbar: FC = () => {
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));

	const { appearance, showOrganizations } = useDashboard();
	const { user: me, permissions, signOut } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();
	const canViewAuditLog =
		featureVisibility.audit_log && Boolean(permissions.viewAnyAuditLog);
	const canViewDeployment = Boolean(permissions.viewDeploymentValues);
	const canViewOrganizations =
		Boolean(permissions.editAnyOrganization) && showOrganizations;
	const proxyContextValue = useProxy();
	const canViewHealth = canViewDeployment;

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
