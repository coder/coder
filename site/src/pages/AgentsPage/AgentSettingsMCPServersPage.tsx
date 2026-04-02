import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	createMCPServerConfig,
	deleteMCPServerConfig,
	mcpServerConfigs,
	updateMCPServerConfig,
} from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AdminBadge } from "./components/AdminBadge";
import { MCPServerAdminPanel } from "./components/MCPServerAdminPanel";

const AgentSettingsMCPServersPage: FC = () => {
	const { permissions } = useAuthenticated();

	const queryClient = useQueryClient();

	const serversQuery = useQuery(mcpServerConfigs());
	const createServerMutation = useMutation(createMCPServerConfig(queryClient));
	const updateServerMutation = useMutation(updateMCPServerConfig(queryClient));
	const deleteServerMutation = useMutation(deleteMCPServerConfig(queryClient));

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<MCPServerAdminPanel
				sectionLabel="MCP Servers"
				sectionDescription="Configure external MCP servers that provide additional tools for Coder Agents."
				sectionBadge={<AdminBadge />}
				serversData={serversQuery.data}
				isLoadingServers={serversQuery.isLoading}
				serversError={serversQuery.isError ? serversQuery.error : null}
				onCreateServer={(req) => createServerMutation.mutateAsync(req)}
				onUpdateServer={(args) => updateServerMutation.mutateAsync(args)}
				onDeleteServer={(id) => deleteServerMutation.mutateAsync(id)}
				isCreatingServer={createServerMutation.isPending}
				isUpdatingServer={updateServerMutation.isPending}
				isDeletingServer={deleteServerMutation.isPending}
				createError={createServerMutation.error}
				updateError={updateServerMutation.error}
				deleteError={deleteServerMutation.error}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsMCPServersPage;
