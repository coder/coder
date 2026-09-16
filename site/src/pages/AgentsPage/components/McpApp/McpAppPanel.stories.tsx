import { AppBridge } from "@modelcontextprotocol/ext-apps/app-bridge";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import { chatMCPAppResourceKey } from "#/api/queries/chats";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChat, MockMCPServerConfig } from "#/testHelpers/chatEntities";
import boardHtml from "#/testHelpers/mcpAppBoard.html?raw";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { createChatStore } from "../ChatConversation/chatStore";
import McpAppPanel from "./McpAppPanel";
import { type McpAppTab, mcpAppTabId } from "./mcpAppTab";
import { MCP_APP_MIME_TYPE } from "./sandboxUrl";

const RESOURCE_URI = "ui://taskboard/board";
const TOOL_CALL_ID = "call-1";

const tab: McpAppTab = {
	id: mcpAppTabId(MockMCPServerConfig.id, RESOURCE_URI),
	kind: "mcp_app",
	mcpServerConfigId: MockMCPServerConfig.id,
	resourceUri: RESOURCE_URI,
	toolCallId: TOOL_CALL_ID,
	label: MockMCPServerConfig.display_name,
};

const taskboardServer = {
	...MockMCPServerConfig,
	display_name: "Task board",
	slug: "taskboard",
};

const toolCallMessages: ChatMessage[] = [
	{
		id: 1,
		chat_id: MockChat.id,
		created_at: MockChat.created_at,
		role: "assistant",
		content: [
			{
				type: "tool-call",
				tool_call_id: TOOL_CALL_ID,
				tool_name: "add_task",
				args: { title: "Write stories" },
				mcp_server_config_id: MockMCPServerConfig.id,
				mcp_app_resource_uri: RESOURCE_URI,
			},
		],
	},
	{
		id: 2,
		chat_id: MockChat.id,
		created_at: MockChat.created_at,
		role: "tool",
		content: [
			{
				type: "tool-result",
				tool_call_id: TOOL_CALL_ID,
				tool_name: "add_task",
				result: "Added task #1",
				mcp_result: { content: [{ type: "text", text: "Added task #1" }] },
			},
		],
	},
];

const storeWith = (messages: readonly ChatMessage[]) => {
	const store = createChatStore();
	store.replaceMessages(messages);
	store.setChatStatus("waiting");
	return store;
};

const resourceKey = chatMCPAppResourceKey(
	MockChat.id,
	MockMCPServerConfig.id,
	RESOURCE_URI,
);

const resource = (mimeType: string) => ({
	result: {
		contents: [
			{
				uri: RESOURCE_URI,
				mimeType,
				text: boardHtml,
				_meta: { ui: { prefersBorder: true } },
			},
		],
	},
});

// The panel fills the right-panel tab, so give stories a bounded height.
const PanelFrame = (Story: FC) => (
	<div className="h-[480px] w-[420px] max-w-full border border-solid border-border-default">
		<Story />
	</div>
);

const meta: Meta<typeof McpAppPanel> = {
	title: "pages/AgentsPage/McpAppPanel",
	component: McpAppPanel,
	decorators: [PanelFrame, withDashboardProvider],
	args: {
		chatId: MockChat.id,
		tab,
		store: storeWith(toolCallMessages),
		chatStatus: "waiting",
		mcpServer: taskboardServer,
		wildcardHostname: "*.apps.example.com",
		submitAppMessage: () => Promise.resolve(),
		isClosing: false,
		onClosed: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof McpAppPanel>;

export const MissingWildcardHost: Story = {
	args: {
		wildcardHostname: undefined,
	},
};

export const MissingServer: Story = {
	args: {
		mcpServer: undefined,
	},
};

export const ToolCallNotLoaded: Story = {
	args: {
		store: storeWith([]),
	},
	parameters: {
		queries: [{ key: resourceKey, data: resource(MCP_APP_MIME_TYPE) }],
	},
};

export const LoadingResource: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "readChatMCPAppResource").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const WrongMimeType: Story = {
	parameters: {
		queries: [{ key: resourceKey, data: resource("text/html") }],
	},
};

export const ResourceReadFailed: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "readChatMCPAppResource").mockRejectedValue(
			new Error("MCP server is unreachable"),
		);
	},
};

export const BootFailed: Story = {
	parameters: {
		queries: [{ key: resourceKey, data: resource(MCP_APP_MIME_TYPE) }],
	},
	beforeEach: () => {
		spyOn(AppBridge.prototype, "connect").mockRejectedValue(
			new Error("The sandbox refused the connection"),
		);
	},
};

export const Ready: Story = {
	parameters: {
		queries: [{ key: resourceKey, data: resource(MCP_APP_MIME_TYPE) }],
		// The sandbox iframe points at a wildcard host that does not resolve in
		// Storybook; the frame and bridge behavior are covered by Vitest.
		pixel: { exclude: true },
	},
};
