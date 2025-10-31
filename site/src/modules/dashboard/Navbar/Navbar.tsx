import { buildInfo } from "api/queries/buildInfo";
import type { LinkConfig } from "api/typesGenerated";
import { useProxy } from "contexts/ProxyContext";
import { useAuthenticated } from "hooks";
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
	const canViewConnectionLog =
		featureVisibility.connection_log && permissions.viewAnyConnectionLog;
	const canViewAIGovernance =
		featureVisibility.aibridge && permissions.viewAnyAIBridgeInterception;

	const uniqueLinks = new Map<string, LinkConfig>();
	for (const link of appearance.support_links ?? []) {
		if (!uniqueLinks.has(link.name)) {
			uniqueLinks.set(link.name, link);
		}
	}
	return (
		<NavbarView
			user={me}
			logo_url={appearance.logo_url}
			buildInfo={buildInfoQuery.data}
			supportLinks={Array.from(uniqueLinks.values())}
			onSignOut={signOut}
			canViewDeployment={canViewDeployment}
			canViewOrganizations={canViewOrganizations}
			canViewHealth={canViewHealth}
			canViewAuditLog={canViewAuditLog}
			canViewConnectionLog={canViewConnectionLog}
			canViewAIGovernance={canViewAIGovernance}
			proxyContextValue={proxyContextValue}
		/>
	);
};
