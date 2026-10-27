import { useState } from "react";
import type { Chat, ChatMCPApp, ChatMessage } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { generateUUID } from "#/utils/random";
import {
	findMcpAppTab,
	type McpAppTabSource,
	mcpAppTabForEntrypoint,
	mcpAppTabForToolResult,
} from "../../../utils/mcpAppTabs";
import type { UserRightPanelTab } from "../../../utils/rightPanelTabs";
import type { ChatStore } from "../../ChatConversation/chatStore";
import type { McpAppsMenuSource } from "../RightPanelAddTabControl";
import { useMcpAppAutoOpen } from "./useMcpAppAutoOpen";

type UseMcpAppTabsOptions = {
	chat: Chat;
	store: ChatStore;
	initialMessages: readonly ChatMessage[];
	selectedMCPServerIds: readonly string[];
	tabs: readonly UserRightPanelTab[];
	addTab: (tab: UserRightPanelTab) => void;
	activateTab: (tabId: string) => void;
};

type McpAppTabs = {
	/** Opens the view of a tool result. Unset when the user cannot open views. */
	onOpenMcpApp: ((toolCallId: string, app: ChatMCPApp) => void) | undefined;
	/** The MCP apps part of the Add panel menu. Unset like onOpenMcpApp. */
	menu: McpAppsMenuSource | undefined;
};

/**
 * Opens MCP Apps views in right panel tabs from the tool card, from the Add
 * panel menu, and at once for each new tool result.
 */
export function useMcpAppTabs({
	chat,
	store,
	initialMessages,
	selectedMCPServerIds,
	tabs,
	addTab,
	activateTab,
}: UseMcpAppTabsOptions): McpAppTabs {
	const { experiments } = useDashboard();
	const { user } = useAuthenticated();
	// Tool results in messages above this ID arrived after mount, so their
	// views open at once.
	const [newestMessageIdAtMount] = useState(
		() => initialMessages.at(-1)?.id ?? 0,
	);
	// Views use the chat owner's MCP credentials, so only the owner can open
	// them.
	const canOpen =
		experiments.includes("mcp-apps") &&
		!chat.archived &&
		user.id === chat.owner_id;

	const openTab = (source: McpAppTabSource) => {
		const existingTab = findMcpAppTab(tabs, source);
		if (existingTab) {
			activateTab(existingTab.id);
			return;
		}
		const tab: UserRightPanelTab = {
			id: `mcp_app-${generateUUID()}`,
			...source,
		};
		addTab(tab);
		activateTab(tab.id);
	};
	const openToolResult = (toolCallId: string, app: ChatMCPApp) =>
		openTab(mcpAppTabForToolResult(toolCallId, app));

	useMcpAppAutoOpen(store, canOpen, newestMessageIdAtMount, openToolResult);

	if (!canOpen) {
		return { onOpenMcpApp: undefined, menu: undefined };
	}
	return {
		onOpenMcpApp: openToolResult,
		menu: {
			chatId: chat.id,
			mcpServerIds: selectedMCPServerIds,
			onOpen: (entrypoint) => openTab(mcpAppTabForEntrypoint(entrypoint)),
		},
	};
}
