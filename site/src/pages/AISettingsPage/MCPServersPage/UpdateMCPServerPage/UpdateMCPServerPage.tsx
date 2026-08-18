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
import { organizationsPermissions } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
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
	const organizationPermissionsQuery = useQuery({
		...organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
		enabled: !permissions.editDeploymentConfig,
	});
	const viewableOrganizations = permissions.editDeploymentConfig
		? organizations
		: organizations.filter(
				(organization) =>
					organizationPermissionsQuery.data?.[organization.id]
						.viewMCPServerConfigs,
			);
	const organization =
		viewableOrganizations.length > 0
			? selectOrganization(
					viewableOrganizations,
					searchParams.get(orgSearchParam),
				)
			: undefined;
	const organizationPermissions = organization
		? organizationPermissionsQuery.data?.[organization.id]
		: undefined;
	const canUpdate =
		permissions.editDeploymentConfig ||
		Boolean(
			organizationPermissions?.viewMCPServerConfigs &&
				organizationPermissions.updateMCPServerConfig,
		);
	const canDelete =
		permissions.editDeploymentConfig ||
		Boolean(organizationPermissions?.deleteMCPServerConfig);
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const serverQuery = useQuery({
		...mcpServerConfig(organization?.id ?? "", serverId ?? ""),
		enabled: Boolean(serverId) && (canUpdate || canDelete),
	});
	const server = serverQuery.data;
	const updateMutation = useMutation(
		updateMCPServerConfig(queryClient, organization?.id ?? ""),
	);
	const deleteMutation = useMutation(
		deleteMCPServerConfig(queryClient, organization?.id ?? ""),
	);
	// A 404 must win over cached data: a refetch failure keeps stale data,
	// which would otherwise render a form for a deleted or concealed server.
	const notFound =
		serverQuery.isError &&
		isApiError(serverQuery.error) &&
		serverQuery.error.response.status === 404;
	const listPath = organization
		? mcpServersPath(organization)
		: "/ai/settings/mcp-servers";
	const onUpdateServer = canUpdate
		? async (id: string, req: TypesGen.UpdateMCPServerConfigRequest) => {
				try {
					const updated = await updateMutation.mutateAsync({ id, req });
					toast.success(`MCP server "${updated.display_name}" updated.`);
					await navigate(listPath);
				} catch (error) {
					toast.error(getErrorMessage(error, "Failed to update MCP server."));
				}
			}
		: undefined;

	return (
		<RequirePermission
			isFeatureVisible={
				permissions.editDeploymentConfig ||
				permissions.updateAnyMCPServerConfig ||
				permissions.viewAnyMCPServerConfigs
			}
		>
			{organizationPermissionsQuery.isError ? (
				<ErrorAlert error={organizationPermissionsQuery.error} />
			) : !permissions.editDeploymentConfig &&
				!organizationPermissionsQuery.data ? (
				<Loader />
			) : (
				<RequirePermission
					isFeatureVisible={(canUpdate || canDelete) && Boolean(organization)}
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
								onUpdateServer={onUpdateServer}
								onDeleteServer={
									canDelete
										? async (id) => {
												try {
													await deleteMutation.mutateAsync(id);
													toast.success(
														`MCP server "${server.display_name}" deleted.`,
													);
													await navigate(listPath, { replace: true });
												} catch (error) {
													toast.error(
														getErrorMessage(
															error,
															"Failed to delete MCP server.",
														),
													);
												}
											}
										: undefined
								}
								onToggleEnabled={
									canUpdate
										? (enabled) => {
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
											}
										: undefined
								}
							/>
						</>
					)}
				</RequirePermission>
			)}
		</RequirePermission>
	);
};

export default UpdateMCPServerPage;
