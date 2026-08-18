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
		: organizations.filter(
				(organization) =>
					organizationPermissionsQuery.data?.[organization.id]
						.viewMCPServerConfigs,
			);
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
		Boolean(organizationPermissions?.viewMCPServerConfigs);
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
				permissions.editDeploymentConfig || permissions.viewAnyMCPServerConfigs
			}
		>
			<title>{pageTitle("MCP servers", "AI Settings")}</title>
			{organizationPermissionsQuery.isError ? (
				<ErrorAlert error={organizationPermissionsQuery.error} />
			) : !permissions.editDeploymentConfig &&
				!organizationPermissionsQuery.data ? (
				<Loader />
			) : (
				<RequirePermission isFeatureVisible={canView && Boolean(organization)}>
					{organization && (
						<MCPServersPageView
							isLoading={serversQuery.isLoading}
							error={serversQuery.error}
							servers={servers}
							organizations={authorizedOrganizations}
							organization={organization}
							canCreate={
								permissions.editDeploymentConfig ||
								Boolean(organizationPermissions?.createMCPServerConfig)
							}
							canUpdate={
								permissions.editDeploymentConfig ||
								Boolean(organizationPermissions?.updateMCPServerConfig)
							}
							canDelete={
								permissions.editDeploymentConfig ||
								Boolean(organizationPermissions?.deleteMCPServerConfig)
							}
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
