import { useQuery } from "react-query";
import { Navigate } from "react-router";
import { aiSpendOrganizations } from "#/api/queries/aiBridge";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { canAccessAnyChatModelConfig } from "#/modules/permissions";
import { useCanShareOrganizationMCPServers } from "./MCPServersPage/organizationSharing";
import { useAccessibleModelOrganizations } from "./ModelsPage/organizationModels";
import { canViewAISpend } from "./SpendPage/spendAccess";

export const AISettingsIndexRedirect = () => {
	const { permissions } = useAuthenticated();
	const { entitlements, organizations } = useDashboard();
	const accessibleOrgsQuery = useAccessibleModelOrganizations(organizations);
	const organizationMCPSharing = useCanShareOrganizationMCPServers(
		organizations,
		{ enabled: !permissions.editDeploymentConfig },
	);
	const spendOrganizationsQuery = useQuery({
		...aiSpendOrganizations(),
		enabled:
			entitlements.features.aibridge.enabled &&
			!permissions.editDeploymentConfig,
	});

	if (permissions.viewAnyAIProvider) {
		return <Navigate to="/ai/settings/providers" replace />;
	}

	if (permissions.viewAIGatewayKeys) {
		return <Navigate to="/ai/settings/gateway-keys" replace />;
	}

	if (canAccessAnyChatModelConfig({ ...permissions })) {
		return <Navigate to="/ai/settings/models" replace />;
	}

	if (
		permissions.viewAnyMCPServerConfigs ||
		permissions.updateAnyMCPServerConfig ||
		permissions.deleteAnyMCPServerConfig
	) {
		return <Navigate to="/ai/settings/mcp-servers" replace />;
	}

	if (permissions.createAnyMCPServerConfig) {
		return <Navigate to="/ai/settings/mcp-servers/add" replace />;
	}

	if (permissions.updateAnyTemplate) {
		return <Navigate to="/ai/settings/templates" replace />;
	}

	if (accessibleOrgsQuery.isLoading) {
		return <Loader fullscreen />;
	}

	if (accessibleOrgsQuery.error !== null) {
		return <ErrorAlert error={accessibleOrgsQuery.error} />;
	}

	if (accessibleOrgsQuery.organizations.length > 0) {
		return <Navigate to="/ai/settings/models" replace />;
	}

	if (organizationMCPSharing.isLoading) {
		return <Loader fullscreen />;
	}

	if (organizationMCPSharing.error !== null) {
		return <ErrorAlert error={organizationMCPSharing.error} />;
	}

	if (organizationMCPSharing.canShare) {
		return <Navigate to="/ai/settings/mcp-servers" replace />;
	}

	if (permissions.editDeploymentConfig) {
		return <Navigate to="/ai/settings/coder-agents" replace />;
	}

	if (spendOrganizationsQuery.isLoading) {
		return <Loader fullscreen />;
	}

	if (spendOrganizationsQuery.error !== null) {
		return <ErrorAlert error={spendOrganizationsQuery.error} />;
	}

	if (canViewAISpend(entitlements, spendOrganizationsQuery.data)) {
		return <Navigate to="/ai/settings/spend" replace />;
	}

	return <Navigate to="/ai/settings/providers" replace />;
};
