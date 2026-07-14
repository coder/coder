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
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import UpdateMCPServerPageView from "./UpdateMCPServerPageView";

const UpdateMCPServerPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { serverId } = useParams<{ serverId: string }>();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const serversQuery = useQuery(mcpServerConfigs());
	const updateMutation = useMutation(updateMCPServerConfig(queryClient));
	const deleteMutation = useMutation(deleteMCPServerConfig(queryClient));
	const server = serversQuery.data?.find((item) => item.id === serverId);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			{!serverId ? (
				<Navigate to="/ai/settings/mcp-servers" replace />
			) : serversQuery.isLoading ? (
				<>
					<title>{pageTitle("Loading...", "AI Settings")}</title>
					<Loader fullscreen />
				</>
			) : !server ? (
				<Navigate to="/ai/settings/mcp-servers" replace />
			) : (
				<UpdateMCPServerPageView
					server={server}
					isSaving={updateMutation.isPending}
					isDeleting={deleteMutation.isPending}
					onCancel={() => void navigate("/ai/settings/mcp-servers")}
					onUpdateServer={async (id, req) => {
						try {
							const updated = await updateMutation.mutateAsync({ id, req });
							toast.success(`MCP server "${updated.display_name}" updated.`);
							await navigate("/ai/settings/mcp-servers");
						} catch (error) {
							toast.error(
								getErrorMessage(error, "Failed to update MCP server."),
							);
						}
					}}
					onDeleteServer={async (id) => {
						try {
							await deleteMutation.mutateAsync(id);
							toast.success(`MCP server "${server.display_name}" deleted.`);
							await navigate("/ai/settings/mcp-servers", { replace: true });
						} catch (error) {
							toast.error(
								getErrorMessage(error, "Failed to delete MCP server."),
							);
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
			)}
		</RequirePermission>
	);
};

export default UpdateMCPServerPage;
