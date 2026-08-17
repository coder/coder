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
	const organizationPermissionsQuery = useQuery(
		organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
	);
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
	const canCreate =
		permissions.editDeploymentConfig ||
		Boolean(
			organizationPermissions?.viewMCPServerConfigs &&
				organizationPermissions.createMCPServerConfig,
		);
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
			{organizationPermissionsQuery.isError ? (
				<ErrorAlert error={organizationPermissionsQuery.error} />
			) : !permissions.editDeploymentConfig &&
				!organizationPermissionsQuery.data ? (
				<Loader />
			) : (
				<RequirePermission
					isFeatureVisible={canCreate && Boolean(organization)}
				>
					{organization && (
						<AddMCPServerPageView
							isSaving={createMutation.isPending}
							organizations={viewableOrganizations}
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
									await navigate(updateMCPServerPath(server.id, organization));
								} catch (error) {
									toast.error(
										getErrorMessage(error, "Failed to add MCP server."),
									);
								}
							}}
						/>
					)}
				</RequirePermission>
			)}
		</RequirePermission>
	);
};

export default AddMCPServerPage;
