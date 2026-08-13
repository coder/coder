import type { FC } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { mcpServerConfigs } from "#/api/queries/chats";
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
	const organization = selectOrganization(
		organizations,
		searchParams.get(orgSearchParam),
	);
	const serversQuery = useQuery({
		...mcpServerConfigs(organization?.id ?? ""),
		enabled: Boolean(organization),
	});
	const servers = (serversQuery.data ?? []).toSorted((a, b) =>
		a.display_name.localeCompare(b.display_name),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("MCP servers", "AI Settings")}</title>
			<MCPServersPageView
				isLoading={serversQuery.isLoading}
				error={serversQuery.error}
				servers={servers}
				organizations={organizations}
				organization={organization}
				onSelectOrganization={(org) => {
					setSearchParams((params) => {
						params.set(orgSearchParam, org.name);
						return params;
					});
				}}
			/>
		</RequirePermission>
	);
};

export default MCPServersPage;
