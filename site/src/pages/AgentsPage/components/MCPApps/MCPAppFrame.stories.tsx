import type { Meta, StoryObj } from "@storybook/react-vite";
import { fireEvent, within } from "storybook/test";
import { MockChatMCPApp } from "#/testHelpers/chatEntities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { createChatStore } from "../ChatConversation/chatStore";
import { GenericToolRenderer, Tool } from "../ChatElements/tools/Tool";
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
		src:
			"data:text/html," +
			encodeURIComponent(
				'<!doctype html><html lang="en"><meta charset="utf-8"><title>Sales chart</title><style>html,body{margin:0;height:100%;box-sizing:border-box}body{padding:24px;background:#10243b;color:#fff;font:16px system-ui;border:4px solid #63b3ff}h1{margin:0 0 24px;font-size:20px}.bar{height:36px;margin:12px 0;padding:8px;box-sizing:border-box;background:#63b3ff;color:#10243b}</style><h1>Sales chart</h1><div class="bar" style="width:40%">10</div><div class="bar" style="width:80%">20</div></html>',
			),
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
export const Resized: Story = {
	play: async (context) => {
		await initialize(context);
		const source = within(context.canvasElement).getByTitle<HTMLIFrameElement>(
			"Sales chart",
		).contentWindow;
		window.dispatchEvent(
			new MessageEvent("message", {
				source,
				data: {
					jsonrpc: "2.0",
					method: "ui/notifications/size-change",
					params: { width: 400, height: 200 },
				},
			}),
		);
	},
};
export const Panel: Story = {
	args: { displayMode: "fullscreen" },
	play: initialize,
};
export const InactivePanel: Story = {
	args: { displayMode: "fullscreen" },
	render: (args) => (
		<div style={{ height: "100%" }}>
			<p>Another tab is active</p>
			<div style={{ visibility: "hidden", height: "100%" }}>
				<MCPAppFrame {...args} />
			</div>
		</div>
	),
	play: initialize,
};
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

export const InlineError: Story = {
	render: () => (
		<MCPAppContext value={{ chatId: "chat", onOpenApp: () => {} }}>
			<Tool
				name="charts__sales"
				toolCallId="tool"
				mcpApp={MockChatMCPApp}
				status="error"
				isError
				modelIntent="Loading sales chart"
				args={{}}
				result="Sales data unavailable"
			/>
		</MCPAppContext>
	),
	play: async ({ canvasElement }) => {
		fireEvent.error(await within(canvasElement).findByTitle("charts__sales"));
	},
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
