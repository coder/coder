import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useLocation, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	deleteMCPServerConfig,
	mcpServerConfig,
	updateMCPServerConfig,
} from "#/api/queries/chats";
import { Loader } from "#/components/Loader/Loader";
import { useAIResourceOrganization } from "#/contexts/AIResourceOrganizationContext";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import UpdateMCPServerPageView from "./UpdateMCPServerPageView";

const UpdateMCPServerPage: FC = () => {
	const { organization, permissions: organizationPermissions } =
		useAIResourceOrganization();
	const { serverId } = useParams<{ serverId: string }>();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const serverQuery = useQuery({
		...mcpServerConfig(serverId ?? ""),
		enabled: Boolean(serverId),
	});
	const updateMutation = useMutation(updateMCPServerConfig(queryClient));
	const deleteMutation = useMutation(deleteMCPServerConfig(queryClient));
	const server =
		serverQuery.data?.organization_id === organization.id
			? serverQuery.data
			: undefined;

	return (
		<RequirePermission
			isFeatureVisible={Boolean(organizationPermissions?.editMCPServers)}
		>
			{!serverId ? (
				<Navigate
					to={{ pathname: "/ai/settings/mcp-servers", search: location.search }}
					replace
				/>
			) : serverQuery.isLoading ? (
				<>
					<title>{pageTitle("Loading...", "AI Settings")}</title>
					<Loader fullscreen />
				</>
			) : !server ? (
				<Navigate
					to={{ pathname: "/ai/settings/mcp-servers", search: location.search }}
					replace
				/>
			) : (
				<UpdateMCPServerPageView
					server={server}
					isSaving={updateMutation.isPending}
					isDeleting={deleteMutation.isPending}
					onCancel={() =>
						void navigate({
							pathname: "/ai/settings/mcp-servers",
							search: location.search,
						})
					}
					onUpdateServer={async (id, req) => {
						try {
							const updated = await updateMutation.mutateAsync({
								organization: organization.name,
								serverId: id,
								req,
							});
							toast.success(`MCP server "${updated.display_name}" updated.`);
							await navigate({
								pathname: "/ai/settings/mcp-servers",
								search: location.search,
							});
						} catch (error) {
							toast.error(
								getErrorMessage(error, "Failed to update MCP server."),
							);
						}
					}}
					onDeleteServer={
						organizationPermissions.deleteMCPServers
							? async (id) => {
									try {
										await deleteMutation.mutateAsync({
											organization: organization.name,
											serverId: id,
										});
										toast.success(
											`MCP server "${server.display_name}" deleted.`,
										);
										await navigate(
											{
												pathname: "/ai/settings/mcp-servers",
												search: location.search,
											},
											{ replace: true },
										);
									} catch (error) {
										toast.error(
											getErrorMessage(error, "Failed to delete MCP server."),
										);
									}
								}
							: undefined
					}
					onToggleEnabled={(enabled) => {
						updateMutation.mutate(
							{
								organization: organization.name,
								serverId: server.id,
								req: { enabled },
							},
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
