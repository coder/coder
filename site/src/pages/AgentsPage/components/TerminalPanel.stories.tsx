import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { WorkspaceAgentLifecycle } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	MockDeploymentConfig,
	MockUserAppearanceSettings,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { withProxyProvider, withWebSocket } from "#/testHelpers/storybook";
import { TerminalClientSessionContext } from "../context/TerminalClientSessionContext";
import { TerminalPanel } from "./TerminalPanel";

const terminalQueries = [
	{
		key: ["deployment", "config"],
		data: {
			...MockDeploymentConfig,
			config: {
				...MockDeploymentConfig.config,
				web_terminal_renderer: "canvas",
			},
		},
	},
	{ key: ["me", "appearance"], data: MockUserAppearanceSettings },
];

const createAgent = (lifecycleState: WorkspaceAgentLifecycle) => ({
	...MockWorkspaceAgent,
	lifecycle_state: lifecycleState,
});

const meta = {
	title: "pages/AgentsPage/TerminalPanel",
	component: TerminalPanel,
	args: {
		chatId: "b5a8832c-72db-4679-8393-9a48dff20a20",
		isHot: true,
		workspaceAgent: createAgent("ready"),
	},
	parameters: {
		layout: "centered",
		pixel: { exclude: true },
		queries: terminalQueries,
	},
	decorators: [
		withProxyProvider(),
		(Story) => (
			<TerminalClientSessionContext value="0123456789abcdef0123456789abcdef">
				<div style={{ width: 480, height: 600 }}>
					<Story />
				</div>
			</TerminalClientSessionContext>
		),
	],
} satisfies Meta<typeof TerminalPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

const promptMessage =
	"\u001b[H\u001b[2J\u001b[1m\u001b[32m➜  \u001b[36mcoder\u001b[C\u001b[34mgit:(\u001b[31mmain\u001b[34m) \u001b[33m✗";
const commandConnections = fn();
const commandErrorConnections = fn();
const commandRerenderConnections = fn();
const shellConnections = fn();
// ExponentialBackoff(1000, 6) schedules its first retry after 2 seconds.
const firstReconnectDelay = 2_100;

const CommandRerenderStory = () => {
	const [agentUpdated, setAgentUpdated] = useState(false);
	return (
		<>
			<Button onClick={() => setAgentUpdated(true)}>
				{agentUpdated ? "Agent updated" : "Update agent"}
			</Button>
			<TerminalPanel
				chatId="b5a8832c-72db-4679-8393-9a48dff20a20"
				initialCommand="echo done"
				isHot
				workspaceAgent={{
					...createAgent("ready"),
					operating_system: agentUpdated ? "windows" : "linux",
				}}
			/>
		</>
	);
};

export const Connected: Story = {
	decorators: [withWebSocket],
	parameters: {
		webSocket: [{ event: "message", data: promptMessage }],
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

export const CommandEnded: Story = {
	args: {
		initialCommand: "echo done",
	},
	beforeEach: () => {
		commandConnections.mockClear();
	},
	decorators: [withWebSocket],
	parameters: {
		webSocketOnConnection: commandConnections,
		webSocket: [
			{ event: "open" },
			{ event: "message", data: "done" },
			{ event: "close" },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByRole("alert")).toHaveTextContent(
			"Terminal session ended. Refresh the session to connect again.",
		);
		await new Promise((resolve) => setTimeout(resolve, firstReconnectDelay));
		await expect(commandConnections).toHaveBeenCalledTimes(1);
	},
};

export const CommandRemainsEndedAfterAgentUpdate: Story = {
	beforeEach: () => {
		commandRerenderConnections.mockClear();
	},
	decorators: [withWebSocket],
	parameters: {
		webSocketOnConnection: commandRerenderConnections,
		webSocket: [
			{ event: "open" },
			{ event: "message", data: "done" },
			{ event: "close" },
		],
	},
	render: () => <CommandRerenderStory />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByRole("alert")).toHaveTextContent(
			"Terminal session ended. Refresh the session to connect again.",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Update agent" }));
		await expect(
			canvas.getByRole("button", { name: "Agent updated" }),
		).toBeVisible();
		await new Promise((resolve) => setTimeout(resolve, 0));
		await expect(commandRerenderConnections).toHaveBeenCalledTimes(1);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"Terminal session ended. Refresh the session to connect again.",
		);
	},
};

export const CommandConnectionError: Story = {
	args: {
		initialCommand: "echo done",
	},
	beforeEach: () => {
		commandErrorConnections.mockClear();
	},
	decorators: [withWebSocket],
	parameters: {
		webSocketOnConnection: commandErrorConnections,
		webSocket: [{ event: "error" }, { event: "close" }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByRole("alert")).toHaveTextContent(
			"Terminal connection failed. Refresh the session to try again.",
		);
		await new Promise((resolve) => setTimeout(resolve, firstReconnectDelay));
		await expect(commandErrorConnections).toHaveBeenCalledTimes(1);
	},
};

export const ShellReconnectsAfterClose: Story = {
	beforeEach: () => {
		shellConnections.mockClear();
	},
	decorators: [withWebSocket],
	parameters: {
		webSocketOnConnection: shellConnections,
		webSocket: [{ event: "open" }, { event: "close" }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByRole("alert")).toHaveTextContent(
			"Trying to connect...",
		);
		await waitFor(
			() => {
				expect(shellConnections.mock.calls.length).toBeGreaterThanOrEqual(2);
			},
			{ timeout: 5_000 },
		);
	},
};
