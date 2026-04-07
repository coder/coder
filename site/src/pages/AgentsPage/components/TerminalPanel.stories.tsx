import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceAgentLifecycle } from "#/api/typesGenerated";
import {
	MockDeploymentConfig,
	MockUserAppearanceSettings,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { withProxyProvider, withWebSocket } from "#/testHelpers/storybook";
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
