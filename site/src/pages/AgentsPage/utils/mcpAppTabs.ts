import type { ChatMCPApp, ChatMCPAppEntrypoint } from "#/api/typesGenerated";
import type { UserRightPanelTab } from "./rightPanelTabs";

export type McpAppRightPanelTab = Extract<
	UserRightPanelTab,
	{ kind: "mcp_app" }
>;

// Omit each member on its own: Omit on the union would make both sources
// optional.
export type McpAppTabSource =
	| Omit<Extract<McpAppRightPanelTab, { toolCallId: string }>, "id">
	| Omit<Extract<McpAppRightPanelTab, { entrypointToolName: string }>, "id">;

export const mcpAppTabForToolResult = (
	toolCallId: string,
	app: ChatMCPApp,
): McpAppTabSource => ({
	kind: "mcp_app",
	label: app.title,
	mcpServerConfigId: app.mcp_server_config_id,
	toolCallId,
});

export const mcpAppTabForEntrypoint = (
	entrypoint: ChatMCPAppEntrypoint,
): McpAppTabSource => ({
	kind: "mcp_app",
	label: entrypoint.title,
	mcpServerConfigId: entrypoint.mcp_server_config_id,
	entrypointToolName: entrypoint.tool_name,
});

/** The open tab that shows the same view source, so a second open selects it. */
export const findMcpAppTab = (
	tabs: readonly UserRightPanelTab[],
	source: McpAppTabSource,
): McpAppRightPanelTab | undefined =>
	tabs.find(
		(tab): tab is McpAppRightPanelTab =>
			tab.kind === "mcp_app" &&
			tab.mcpServerConfigId === source.mcpServerConfigId &&
			tab.toolCallId === source.toolCallId &&
			tab.entrypointToolName === source.entrypointToolName,
	);
