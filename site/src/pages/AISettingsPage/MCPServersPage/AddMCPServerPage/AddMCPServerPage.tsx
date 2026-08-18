import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createMCPServerConfig } from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import {
	mcpServersPath,
	orgSearchParam,
	selectOrganization,
	updateMCPServerPath,
} from "../organizationParam";
import AddMCPServerPageView from "./AddMCPServerPageView";

const AddMCPServerPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const [searchParams, setSearchParams] = useSearchParams();
	const organizationPermissionsQuery = useQuery({
		...organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
		enabled: !permissions.editDeploymentConfig,
	});
	const listableOrganizations = permissions.editDeploymentConfig
		? organizations
		: organizations.filter((organization) => {
				const organizationPermissions =
					organizationPermissionsQuery.data?.[organization.id];
				return Boolean(
					organizationPermissions?.viewMCPServerConfigs ||
						organizationPermissions?.updateMCPServerConfig ||
						organizationPermissions?.deleteMCPServerConfig,
				);
			});
	const creatableOrganizations = permissions.editDeploymentConfig
		? organizations
		: organizations.filter(
				(organization) =>
					organizationPermissionsQuery.data?.[organization.id]
						.createMCPServerConfig,
			);
	const requestedOrganizationName = searchParams.get(orgSearchParam);
	const requestedOrganization =
		listableOrganizations.find(
			(organization) => organization.name === requestedOrganizationName,
		) ??
		creatableOrganizations.find(
			(organization) => organization.name === requestedOrganizationName,
		);
	const organization =
		requestedOrganization ??
		(creatableOrganizations.length > 0
			? selectOrganization(creatableOrganizations, null)
			: undefined);
	const organizationPermissions = organization
		? organizationPermissionsQuery.data?.[organization.id]
		: undefined;
	const canViewServerList =
		permissions.editDeploymentConfig ||
		Boolean(
			organizationPermissions?.viewMCPServerConfigs ||
				organizationPermissions?.updateMCPServerConfig ||
				organizationPermissions?.deleteMCPServerConfig,
		);
	const canCreate =
		permissions.editDeploymentConfig ||
		Boolean(organizationPermissions?.createMCPServerConfig);
	const canOpenServer =
		permissions.editDeploymentConfig ||
		Boolean(
			organizationPermissions?.updateMCPServerConfig ||
				organizationPermissions?.deleteMCPServerConfig,
		);
	const canOpenAnyServerList =
		permissions.editDeploymentConfig ||
		permissions.viewAnyMCPServerConfigs ||
		permissions.updateAnyMCPServerConfig ||
		permissions.deleteAnyMCPServerConfig;
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const createMutation = useMutation(
		createMCPServerConfig(queryClient, organization?.id ?? ""),
	);

	return (
		<RequirePermission
			isFeatureVisible={
				permissions.editDeploymentConfig || permissions.createAnyMCPServerConfig
			}
		>
			{organizationPermissionsQuery.isLoadingError ? (
				<ErrorAlert error={organizationPermissionsQuery.error} />
			) : !permissions.editDeploymentConfig &&
				!organizationPermissionsQuery.data ? (
				<Loader />
			) : creatableOrganizations.length === 0 ? (
				<RequirePermission isFeatureVisible={false} />
			) : (
				organization && (
					<>
						{organizationPermissionsQuery.isRefetchError && (
							<div className="mb-4">
								<ErrorAlert error={organizationPermissionsQuery.error} />
							</div>
						)}
						<AddMCPServerPageView
							isSaving={createMutation.isPending}
							canCreate={canCreate}
							canSelectUserOIDC={permissions.editDeploymentConfig}
							canViewServerList={canOpenAnyServerList}
							organizations={creatableOrganizations}
							organization={organization}
							onSelectOrganization={(org) => {
								setSearchParams((params) => {
									const next = new URLSearchParams(params);
									next.set(orgSearchParam, org.name);
									return next;
								});
							}}
							onCancel={() => void navigate(mcpServersPath(organization))}
							onCreateServer={async (req) => {
								try {
									const server = await createMutation.mutateAsync(req);
									toast.success(`MCP server "${server.display_name}" added.`);
									if (canOpenServer) {
										await navigate(
											updateMCPServerPath(server.id, organization),
										);
									} else if (canViewServerList) {
										await navigate(mcpServersPath(organization));
									}
									return true;
								} catch (error) {
									toast.error(
										getErrorMessage(error, "Failed to add MCP server."),
									);
									return false;
								}
							}}
						/>
					</>
				)
			)}
		</RequirePermission>
	);
};

export default AddMCPServerPage;
