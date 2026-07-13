import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	deleteMCPServerConfig,
	mcpServerConfigs,
	updateMCPServerConfig,
} from "#/api/queries/chats";
import { Loader } from "#/components/Loader/Loader";
import { MCPServerForm } from "#/pages/AISettingsPage/MCPServersPage/components/MCPServerForm";
import { pageTitle } from "#/utils/page";

const AgentSettingsUpdateMCPServerPage: FC = () => {
	const { serverId } = useParams<{ serverId: string }>();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const serversQuery = useQuery(mcpServerConfigs("personal"));
	const updateMutation = useMutation(updateMCPServerConfig(queryClient));
	const deleteMutation = useMutation(deleteMCPServerConfig(queryClient));
	const server = serversQuery.data?.find((item) => item.id === serverId);

	if (!serverId) {
		return <Navigate to="/agents/settings/mcp-servers" replace />;
	}
	if (serversQuery.isLoading) {
		return (
			<>
				<title>{pageTitle("Loading...")}</title>
				<Loader fullscreen />
			</>
		);
	}
	if (!server) {
		return <Navigate to="/agents/settings/mcp-servers" replace />;
	}

	return (
		<>
			<title>{pageTitle(server.display_name)}</title>
			<MCPServerForm
				key={server.id}
				server={server}
				variant="personal"
				isSaving={updateMutation.isPending}
				isDeleting={deleteMutation.isPending}
				onCancel={() => void navigate("/agents/settings/mcp-servers")}
				onUpdateServer={async (id, req) => {
					try {
						const updated = await updateMutation.mutateAsync({ id, req });
						toast.success(`MCP server "${updated.display_name}" updated.`);
						await navigate("/agents/settings/mcp-servers");
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to update MCP server."));
					}
				}}
				onDeleteServer={async (id) => {
					try {
						await deleteMutation.mutateAsync(id);
						toast.success(`MCP server "${server.display_name}" deleted.`);
						await navigate("/agents/settings/mcp-servers", { replace: true });
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to delete MCP server."));
					}
				}}
				onToggleEnabled={(enabled) => {
					updateMutation.mutate(
						{ id: server.id, req: { enabled } },
						{
							onSuccess: () => {
								toast.success(
									`MCP server "${server.display_name}" ${enabled ? "enabled" : "disabled"}.`,
								);
							},
							onError: (error) => {
								toast.error(
									getErrorMessage(
										error,
										`Failed to ${enabled ? "enable" : "disable"} MCP server.`,
									),
								);
							},
						},
					);
				}}
			/>
		</>
	);
};

export default AgentSettingsUpdateMCPServerPage;
