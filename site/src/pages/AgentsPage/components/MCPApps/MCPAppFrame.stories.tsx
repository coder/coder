import type { Meta, StoryObj } from "@storybook/react-vite";
import { fireEvent, within } from "storybook/test";
import { MockChatMCPApp } from "#/testHelpers/chatEntities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { createChatStore } from "../ChatConversation/chatStore";
import { GenericToolRenderer } from "../ChatElements/tools/Tool";
import { MCPAppContext } from "./MCPAppContext";
import { MCPAppFrame } from "./MCPAppFrame";
import { MCPAppPanel } from "./MCPAppPanel";
import { MCPAppTool } from "./MCPAppTool";

const meta = {
	title: "pages/AgentsPage/MCPApps/MCPAppFrame",
	component: MCPAppFrame,
	decorators: [
		withDashboardProvider,
		(Story) => (
			<div style={{ width: 560, height: 480 }}>
				<Story />
			</div>
		),
	],
	parameters: { experiments: ["chat-mcp-apps"] },
	args: {
		src: "about:blank",
		title: "Sales chart",
		args: { metric: "sales" },
		result: MockChatMCPApp.result,
		displayMode: "inline",
		fallback: (
			<GenericToolRenderer
				name="Sales chart"
				status="completed"
				isError={false}
				args={{ metric: "sales" }}
				result="Sales: 10, 20"
			/>
		),
	},
} satisfies Meta<typeof MCPAppFrame>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {};

const initialize: NonNullable<Story["play"]> = async ({ canvasElement }) => {
	const source =
		within(canvasElement).getByTitle<HTMLIFrameElement>(
			"Sales chart",
		).contentWindow;
	window.dispatchEvent(
		new MessageEvent("message", {
			source,
			data: { jsonrpc: "2.0", id: 1, method: "ui/initialize" },
		}),
	);
	window.dispatchEvent(
		new MessageEvent("message", {
			source,
			data: { jsonrpc: "2.0", method: "ui/notifications/initialized" },
		}),
	);
};

export const Ready: Story = { play: initialize };
export const Panel: Story = { args: { displayMode: "pip" }, play: initialize };
export const InitializationError: Story = {
	play: async ({ canvasElement }) => {
		fireEvent.error(within(canvasElement).getByTitle("Sales chart"));
	},
};
export const UnavailablePanel: Story = {
	render: () => (
		<MCPAppPanel
			chatId="chat"
			tab={{
				kind: "mcp_app",
				id: "app",
				label: "Sales chart",
				toolCallId: "tool",
				serverName: MockChatMCPApp.server_name,
				resourceUri: MockChatMCPApp.resource_uri,
			}}
			store={createChatStore()}
		/>
	),
};

export const InlineCard: Story = {
	render: (args) => (
		<MCPAppContext value={{ chatId: "chat", onOpenApp: () => {} }}>
			<MCPAppTool
				app={MockChatMCPApp}
				toolCallId="tool"
				name="Sales chart"
				args={args.args}
				fallback={args.fallback}
			/>
		</MCPAppContext>
	),
};

export const NarrowInlineCard: Story = {
	render: (args) => (
		<div style={{ width: 320 }}>
			<MCPAppContext value={{ chatId: "chat", onOpenApp: () => {} }}>
				<MCPAppTool
					app={MockChatMCPApp}
					toolCallId="tool"
					name="Sales chart with a very long descriptive tool name"
					args={args.args}
					fallback={args.fallback}
				/>
			</MCPAppContext>
		</div>
	),
};
