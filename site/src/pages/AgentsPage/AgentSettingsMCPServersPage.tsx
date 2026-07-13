import type { FC } from "react";
import { useQuery } from "react-query";
import { mcpServerConfigs } from "#/api/queries/chats";
import MCPServersPageView from "#/pages/AISettingsPage/MCPServersPage/MCPServersPageView";
import { pageTitle } from "#/utils/page";

const AgentSettingsMCPServersPage: FC = () => {
	const serversQuery = useQuery(mcpServerConfigs("personal"));
	const servers = (serversQuery.data ?? []).toSorted((a, b) =>
		a.display_name.localeCompare(b.display_name),
	);

	return (
		<>
			<title>{pageTitle("Personal MCP servers")}</title>
			<MCPServersPageView
				isLoading={serversQuery.isLoading}
				error={serversQuery.error}
				servers={servers}
				basePath="/agents/settings/mcp-servers"
				description="Configure your own MCP servers that provide additional tools for Coder Agents. Only you can use them."
				variant="personal"
			/>
		</>
	);
};

export default AgentSettingsMCPServersPage;
