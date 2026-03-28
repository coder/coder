import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AdminBadge } from "../components/AdminBadge";
import { MCPServerAdminPanel } from "../components/MCPServerAdminPanel";

const AgentSettingsMCPServersPage: FC = () => {
	const { permissions } = useAuthenticated();
	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<MCPServerAdminPanel
				sectionLabel="MCP Servers"
				sectionDescription="Configure external MCP servers that provide additional tools for AI chat sessions."
				sectionBadge={<AdminBadge />}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsMCPServersPage;
