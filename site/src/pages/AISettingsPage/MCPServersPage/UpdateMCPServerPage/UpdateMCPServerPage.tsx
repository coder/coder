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
	const manageableOrganizations = permissions.editDeploymentConfig
		? organizations
		: organizations.filter((organization) => {
				const organizationPermissions =
					organizationPermissionsQuery.data?.[organization.id];
				return Boolean(
					organizationPermissions?.updateMCPServerConfig ||
						organizationPermissions?.deleteMCPServerConfig ||
						organizationPermissions?.shareMCPServerConfig,
				);
			});
	const requestedOrganizationName = searchParams.get(orgSearchParam);
	const organization =
		requestedOrganizationName === null
			? manageableOrganizations.length > 0
				? selectOrganization(manageableOrganizations, null)
				: undefined
			: manageableOrganizations.find(
					(organization) => organization.name === requestedOrganizationName,
				);
	const organizationPermissions = organization
		? organizationPermissionsQuery.data?.[organization.id]
		: undefined;
	const canUpdate =
		permissions.editDeploymentConfig ||
		Boolean(organizationPermissions?.updateMCPServerConfig);
	const canDelete =
		permissions.editDeploymentConfig ||
		Boolean(organizationPermissions?.deleteMCPServerConfig);
	const canShare =
		permissions.editDeploymentConfig ||
		Boolean(organizationPermissions?.shareMCPServerConfig);
	const canManage = canUpdate || canDelete || canShare;
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const serverQuery = useQuery({
		...mcpServerConfig(organization?.id ?? "", serverId ?? ""),
		enabled: Boolean(serverId) && canManage,
	});
	const server = serverQuery.data;
	// The backend rejects any update touching a user_oidc server without
	// deployment permission, so org-level update grants cannot manage them.
	const canUpdateServer =
		permissions.editDeploymentConfig ||
		(canUpdate && server?.auth_type !== "user_oidc");
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

	return (
		<RequirePermission
			isFeatureVisible={
				permissions.editDeploymentConfig ||
				permissions.updateAnyMCPServerConfig ||
				permissions.deleteAnyMCPServerConfig ||
				organizationPermissionsQuery.data === undefined ||
				manageableOrganizations.length > 0
			}
		>
			{organizationPermissionsQuery.isLoadingError ? (
				<ErrorAlert error={organizationPermissionsQuery.error} />
			) : !permissions.editDeploymentConfig &&
				!organizationPermissionsQuery.data ? (
				<Loader />
			) : (
				<RequirePermission
					isFeatureVisible={canManage && Boolean(organization)}
				>
					{organizationPermissionsQuery.isRefetchError && (
						<div className="mb-4">
							<ErrorAlert error={organizationPermissionsQuery.error} />
						</div>
					)}
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
					) : notFound || !server || !organization ? (
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
								organizations={manageableOrganizations}
								organization={organization}
								listPath={listPath}
								isSaving={updateMutation.isPending}
								isDeleting={deleteMutation.isPending}
								canSelectUserOIDC={permissions.editDeploymentConfig}
								onCancel={() => void navigate(listPath)}
								onUpdateServer={
									canUpdateServer
										? async (id, req) => {
												try {
													const updated = await updateMutation.mutateAsync({
														id,
														req,
													});
													toast.success(
														`MCP server "${updated.display_name}" updated.`,
													);
													await navigate(listPath);
												} catch (error) {
													toast.error(
														getErrorMessage(
															error,
															"Failed to update MCP server.",
														),
													);
												}
											}
										: undefined
								}
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
									canUpdateServer
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
