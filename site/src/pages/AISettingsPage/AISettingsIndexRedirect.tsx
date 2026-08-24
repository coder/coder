import { Navigate } from "react-router";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { canAccessAnyChatModelConfig } from "#/modules/permissions";
import { useAccessibleModelOrganizations } from "./ModelsPage/organizationModels";

export const AISettingsIndexRedirect = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const accessibleOrgsQuery = useAccessibleModelOrganizations(organizations);

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

	if (accessibleOrgsQuery.isLoading) {
		return <Loader fullscreen />;
	}

	if (accessibleOrgsQuery.error !== null) {
		return <ErrorAlert error={accessibleOrgsQuery.error} />;
	}

	if (accessibleOrgsQuery.organizations.length > 0) {
		return <Navigate to="/ai/settings/models" replace />;
	}

	if (permissions.editDeploymentConfig) {
		return <Navigate to="/ai/settings/coder-agents" replace />;
	}

	return <Navigate to="/ai/settings/providers" replace />;
};
