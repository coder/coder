import { useQuery } from "react-query";
import { agentHoursAllotmentOrganizations } from "#/api/queries/agentHours";
import { aiSpendOrganizations } from "#/api/queries/aiBridge";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";
import { canViewAgentHours } from "#/pages/AISettingsPage/AgentHoursPage/agentHoursAccess";
import { readableInstructionsOrganizations } from "#/pages/AISettingsPage/InstructionsPage/readableInstructionsOrganizations";
import { useCanShareOrganizationMCPServers } from "#/pages/AISettingsPage/MCPServersPage/organizationSharing";
import { useAccessibleModelOrganizations } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { canViewAISpend } from "#/pages/AISettingsPage/SpendPage/spendAccess";

/**
 * A sidebar for AI settings.
 */
export const AISettingsSidebar: React.FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements, organizations } = useDashboard();
	const accessibleOrgsQuery = useAccessibleModelOrganizations(organizations);
	const organizationMCPSharing = useCanShareOrganizationMCPServers(
		organizations,
		{ enabled: !permissions.editDeploymentConfig },
	);
	const spendOrganizationsQuery = useQuery({
		...aiSpendOrganizations(),
		enabled: entitlements.features.aibridge.enabled,
	});
	const agentHoursOrganizationsQuery = useQuery({
		...agentHoursAllotmentOrganizations(),
		enabled:
			entitlements.features.agent_runtime_hours.enabled &&
			!permissions.editDeploymentConfig,
	});

	return (
		<AISettingsSidebarView
			permissions={permissions}
			canViewAISpend={canViewAISpend(
				entitlements,
				spendOrganizationsQuery.data,
			)}
			canAccessOrganizationModels={
				(accessibleOrgsQuery.organizations.length ?? 0) > 0
			}
			canShareOrganizationMCPServers={organizationMCPSharing.canShare}
			canViewAgentHours={canViewAgentHours(
				entitlements,
				permissions,
				agentHoursOrganizationsQuery.data,
			)}
			canViewOrganizationInstructions={
				readableInstructionsOrganizations(
					organizations,
					accessibleOrgsQuery.permissionsByOrganization,
				).length > 0
			}
		/>
	);
};
