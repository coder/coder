import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	Navigate,
	useNavigate,
	useParams,
	useSearchParams,
} from "react-router";
import { toast } from "sonner";
import { getErrorMessage, isApiError } from "#/api/errors";
import {
	deleteMCPServerConfig,
	mcpServerConfig,
	updateMCPServerConfig,
} from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import {
	mcpServersPath,
	orgSearchParam,
	selectOrganization,
} from "../organizationParam";
import UpdateMCPServerPageView from "./UpdateMCPServerPageView";

const UpdateMCPServerPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const { serverId } = useParams<{ serverId: string }>();
	const [searchParams] = useSearchParams();
	const organization = selectOrganization(
		organizations,
		searchParams.get(orgSearchParam),
	);
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const serverQuery = useQuery({
		...mcpServerConfig(organization.id, serverId ?? ""),
		enabled: Boolean(serverId),
	});
	const server = serverQuery.data;
	const updateMutation = useMutation(
		updateMCPServerConfig(queryClient, organization.id),
	);
	const deleteMutation = useMutation(
		deleteMCPServerConfig(queryClient, organization.id),
	);
	// A 404 must win over cached data: a refetch failure keeps stale data,
	// which would otherwise render a form for a deleted or concealed server.
	const notFound =
		serverQuery.isError &&
		isApiError(serverQuery.error) &&
		serverQuery.error.response.status === 404;
	const listPath = mcpServersPath(organization);

	return (
		<RequirePermission
			isFeatureVisible={
				permissions.editDeploymentConfig || permissions.updateAnyMCPServerConfig
			}
		>
			{!serverId ? (
				<Navigate to={listPath} replace />
			) : serverQuery.isLoading ? (
				<>
					<title>{pageTitle("Loading...", "AI Settings")}</title>
					<Loader fullscreen />
				</>
			) : serverQuery.isLoadingError && !notFound ? (
				<>
					<title>{pageTitle("MCP servers", "AI Settings")}</title>
					<div className="mb-4">
						<ErrorAlert error={serverQuery.error} />
					</div>
				</>
			) : notFound || !server ? (
				<Navigate to={listPath} replace />
			) : (
				<>
					{serverQuery.isRefetchError && (
						<div className="mb-4">
							<ErrorAlert error={serverQuery.error} />
						</div>
					)}
					<UpdateMCPServerPageView
						server={server}
						listPath={listPath}
						isSaving={updateMutation.isPending}
						isDeleting={deleteMutation.isPending}
						onCancel={() => void navigate(listPath)}
						onUpdateServer={async (id, req) => {
							try {
								const updated = await updateMutation.mutateAsync({ id, req });
								toast.success(`MCP server "${updated.display_name}" updated.`);
								await navigate(listPath);
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
								await navigate(listPath, { replace: true });
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
				</>
			)}
		</RequirePermission>
	);
};

export default UpdateMCPServerPage;
