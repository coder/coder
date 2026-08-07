import type { FC } from "react";
import { useQuery } from "react-query";
import { mcpServerConfigs } from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import MCPServersPageView from "./MCPServersPageView";

const MCPServersPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const organization = getDefaultOrganizationId(organizations);
	const serversQuery = useQuery({
		...mcpServerConfigs(organization),
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
			/>
		</RequirePermission>
	);
};

export default MCPServersPage;
