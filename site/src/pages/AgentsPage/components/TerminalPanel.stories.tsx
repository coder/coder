import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import type { WorkspaceAgentLifecycle } from "#/api/typesGenerated";
import {
	MockDeploymentConfig,
	MockUserAppearanceSettings,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { withProxyProvider, withWebSocket } from "#/testHelpers/storybook";
import { TerminalPanel } from "./TerminalPanel";

const createDeploymentConfigQuery = (renderer: string) => ({
	key: ["deployment", "config"],
	data: {
		...MockDeploymentConfig,
		config: {
			...MockDeploymentConfig.config,
			web_terminal_renderer: renderer,
		},
	},
});

const createTerminalQueries = (renderer: string) => [
	createDeploymentConfigQuery(renderer),
	{ key: ["me", "appearance"], data: MockUserAppearanceSettings },
];

const terminalQueries = createTerminalQueries("dom");

const expectTerminalOutput = async (
	canvasElement: HTMLElement,
	text: string,
) => {
	await waitFor(
		() => {
			const rows = canvasElement.getElementsByClassName("xterm-rows");
			expect(rows.length).toBeGreaterThan(0);
			expect(rows[0]).toHaveTextContent(text);
		},
		{ timeout: 5_000 },
	);
};

const expectTerminalCanvas = async (canvasElement: HTMLElement) => {
	await waitFor(
		() => {
			const terminal = canvasElement.getElementsByClassName("xterm");
			const canvases = canvasElement.getElementsByTagName("canvas");
			expect(terminal.length).toBeGreaterThan(0);
			expect(canvases.length).toBeGreaterThan(0);
		},
		{ timeout: 5_000 },
	);
};

const createAgent = (lifecycleState: WorkspaceAgentLifecycle) => ({
	...MockWorkspaceAgent,
	lifecycle_state: lifecycleState,
});

const meta = {
	title: "pages/AgentsPage/TerminalPanel",
	component: TerminalPanel,
	args: {
		chatId: "b5a8832c-72db-4679-8393-9a48dff20a20",
		workspaceAgent: createAgent("ready"),
	},
	parameters: {
		layout: "centered",
		chromatic: { disableSnapshot: true },
		queries: terminalQueries,
	},
	decorators: [
		withProxyProvider(),
		(Story) => (
			<div style={{ width: 480, height: 600 }}>
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof TerminalPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

const promptMessage =
	"\u001b[H\u001b[2J\u001b[1m\u001b[32m➜  \u001b[36mcoder\u001b[C\u001b[34mgit:(\u001b[31mmain\u001b[34m) \u001b[33m✗";

export const Connected: Story = {
	decorators: [withWebSocket],
	parameters: {
		webSocket: [{ event: "message", data: promptMessage }],
	},
	play: async ({ canvasElement }) => {
		await expectTerminalOutput(canvasElement, "coder");
	},
};

export const CanvasRendererFallback: Story = {
	decorators: [withWebSocket],
	parameters: {
		queries: createTerminalQueries("canvas"),
		webSocket: [
			{ event: "message", data: "canvas fallback renderer smoke test" },
		],
	},
	play: async ({ canvasElement }) => {
		await expectTerminalOutput(
			canvasElement,
			"canvas fallback renderer smoke test",
		);
	},
};

export const WebGLRenderer: Story = {
	decorators: [withWebSocket],
	parameters: {
		queries: createTerminalQueries("webgl"),
		webSocket: [{ event: "message", data: "webgl renderer smoke test" }],
	},
	play: async ({ canvasElement }) => {
		await expectTerminalCanvas(canvasElement);
	},
};

export const AgentUnavailable: Story = {
	args: {
		workspaceAgent: undefined,
	},
};

export const StartingAgent: Story = {
	args: {
		workspaceAgent: createAgent("starting"),
	},
	decorators: [withWebSocket],
	parameters: {
		webSocket: [{ event: "message", data: promptMessage }],
	},
};

export const StartError: Story = {
	args: {
		workspaceAgent: createAgent("start_error"),
	},
	decorators: [withWebSocket],
	parameters: {
		webSocket: [],
	},
};

export const Disconnected: Story = {
	decorators: [withWebSocket],
	parameters: {
		webSocket: [{ event: "error" }],
	},
};
