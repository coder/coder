import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { mcpServerConfigs } from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import MCPServersPageView from "./MCPServersPageView";
import { orgSearchParam, selectOrganization } from "./organizationParam";

const MCPServersPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const [searchParams, setSearchParams] = useSearchParams();
	const organizationPermissionsQuery = useQuery({
		...organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
		enabled: !permissions.editDeploymentConfig,
	});
	const authorizedOrganizations = permissions.editDeploymentConfig
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
	const organization =
		authorizedOrganizations.length > 0
			? selectOrganization(
					authorizedOrganizations,
					searchParams.get(orgSearchParam),
				)
			: undefined;
	const organizationPermissions = organization
		? organizationPermissionsQuery.data?.[organization.id]
		: undefined;
	const canView =
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
	const serversQuery = useQuery({
		...mcpServerConfigs(organization?.id ?? ""),
		enabled: canView,
	});
	const servers = (serversQuery.data ?? []).toSorted((a, b) =>
		a.display_name.localeCompare(b.display_name),
	);

	return (
		<RequirePermission
			isFeatureVisible={
				permissions.editDeploymentConfig ||
				permissions.viewAnyMCPServerConfigs ||
				permissions.updateAnyMCPServerConfig ||
				permissions.deleteAnyMCPServerConfig
			}
		>
			<title>{pageTitle("MCP servers", "AI Settings")}</title>
			{organizationPermissionsQuery.isLoadingError ? (
				<ErrorAlert error={organizationPermissionsQuery.error} />
			) : !permissions.editDeploymentConfig &&
				!organizationPermissionsQuery.data ? (
				<Loader />
			) : (
				<RequirePermission isFeatureVisible={canView && Boolean(organization)}>
					{organizationPermissionsQuery.isRefetchError && (
						<div className="mb-4">
							<ErrorAlert error={organizationPermissionsQuery.error} />
						</div>
					)}
					{organization && (
						<MCPServersPageView
							isLoading={serversQuery.isLoading}
							error={serversQuery.error}
							servers={servers}
							organizations={authorizedOrganizations}
							organization={organization}
							canCreate={canCreate}
							canOpenServer={canOpenServer}
							onSelectOrganization={(org) => {
								setSearchParams((params) => {
									const next = new URLSearchParams(params);
									next.set(orgSearchParam, org.name);
									return next;
								});
							}}
						/>
					)}
				</RequirePermission>
			)}
		</RequirePermission>
	);
};

export default MCPServersPage;
