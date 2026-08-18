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
	const viewableOrganizations = permissions.editDeploymentConfig
		? organizations
		: organizations.filter(
				(organization) =>
					organizationPermissionsQuery.data?.[organization.id]
						.viewMCPServerConfigs,
			);
	const creatableOrganizations = permissions.editDeploymentConfig
		? organizations
		: organizations.filter(
				(organization) =>
					organizationPermissionsQuery.data?.[organization.id]
						.createMCPServerConfig,
			);
	const requestedOrganizationName = searchParams.get(orgSearchParam);
	const requestedOrganization =
		viewableOrganizations.find(
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
	const canView =
		permissions.editDeploymentConfig ||
		Boolean(organizationPermissions?.viewMCPServerConfigs);
	const canCreate =
		permissions.editDeploymentConfig ||
		Boolean(organizationPermissions?.createMCPServerConfig);
	const canOpenServer =
		canView &&
		(permissions.editDeploymentConfig ||
			Boolean(
				organizationPermissions?.updateMCPServerConfig ||
					organizationPermissions?.deleteMCPServerConfig,
			));
	// The list page rejects callers without a site-wide view grant, so exit
	// controls pointing at it are only offered when it is reachable.
	const canViewServerList =
		permissions.editDeploymentConfig || permissions.viewAnyMCPServerConfigs;
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
							canViewServerList={canViewServerList}
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
									} else if (canView) {
										await navigate(mcpServersPath(organization));
									}
								} catch (error) {
									toast.error(
										getErrorMessage(error, "Failed to add MCP server."),
									);
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
