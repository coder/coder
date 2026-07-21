import type { FC } from "react";
import { useQuery } from "react-query";
import { mcpServerConfigs } from "#/api/queries/chats";
import { AIResourceOrganizationSelector } from "#/components/AIResourceOrganizationSelector/AIResourceOrganizationSelector";
import { useAIResourceOrganization } from "#/contexts/AIResourceOrganizationContext";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import MCPServersPageView from "./MCPServersPageView";

const MCPServersPage: FC = () => {
	const { organization, permissions: organizationPermissions } =
		useAIResourceOrganization();
	const serversQuery = useQuery(mcpServerConfigs(organization.name));
	const servers = (serversQuery.data ?? []).toSorted((a, b) =>
		a.display_name.localeCompare(b.display_name),
	);

	return (
		<RequirePermission
			isFeatureVisible={Boolean(organizationPermissions?.viewMCPServers)}
		>
			<title>{pageTitle("MCP servers", "AI Settings")}</title>
			<AIResourceOrganizationSelector />
			<MCPServersPageView
				isLoading={serversQuery.isLoading}
				error={serversQuery.error}
				servers={servers}
				canCreateMCPServers={organizationPermissions.createMCPServers}
				canEditMCPServers={organizationPermissions.editMCPServers}
			/>
		</RequirePermission>
	);
};

export default MCPServersPage;
