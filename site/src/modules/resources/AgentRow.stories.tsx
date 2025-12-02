import { chromatic } from "testHelpers/chromatic";
import * as M from "testHelpers/entities";
import {
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { getPreferredProxy } from "contexts/ProxyContext";
import { spyOn, userEvent, within } from "storybook/test";
import { AgentRow } from "./AgentRow";

const defaultAgentMetadata = [
	{
		result: {
			collected_at: "2021-05-05T00:00:00Z",
			error: "",
			value: "Master",
			age: 5,
		},
		description: {
			display_name: "Branch",
			key: "branch",
			interval: 10,
			timeout: 10,
			script: "git branch",
		},
	},
	{
		result: {
			collected_at: "2021-05-05T00:00:00Z",
			error: "",
			value: "No changes",
			age: 5,
		},
		description: {
			display_name: "Changes",
			key: "changes",
			interval: 10,
			timeout: 10,
			script: "git diff",
		},
	},
	{
		result: {
			collected_at: "2021-05-05T00:00:00Z",
			error: "",
			value: "2%",
			age: 5,
		},
		description: {
			display_name: "CPU Usage",
			key: "cpuUsage",
			interval: 10,
			timeout: 10,
			script: "cpu.sh",
		},
	},
	{
		result: {
			collected_at: "2021-05-05T00:00:00Z",
			error: "",
			value: "3%",
			age: 5,
		},
		description: {
			display_name: "Disk Usage",
			key: "diskUsage",
			interval: 10,
			timeout: 10,
			script: "disk.sh",
		},
	},
];

const logs = [
	"\x1b[91mCloning Git repository...",
	"\x1b[2;37;41mStarting Docker Daemon...",
	"\x1b[1;95mAdding some 🧙magic🧙...",
	"Starting VS Code...",
	"\r  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0\r100  1475    0  1475    0     0   4231      0 --:--:-- --:--:-- --:--:--  4238",
].map((line, index) => ({
	id: index,
	level: "info",
	output: line,
	source_id: M.MockWorkspaceAgentLogSource.id,
	created_at: new Date().toISOString(),
}));

const meta: Meta<typeof AgentRow> = {
	title: "components/AgentRow",
	component: AgentRow,
	args: {
		agent: {
			...M.MockWorkspaceAgent,
			logs_length: logs.length,
		},
		workspace: M.MockWorkspace,
		initialMetadata: defaultAgentMetadata,
	},
	decorators: [withProxyProvider(), withDashboardProvider, withWebSocket],
	parameters: {
		chromatic,
		queries: [
			{
				key: ["portForward", M.MockWorkspaceAgent.id],
				data: M.MockListeningPortsResponse,
			},
		],
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(logs),
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof AgentRow>;

export const Example: Story = {};

export const BunchOfApps: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgent,
			apps: [
				M.MockWorkspaceApp,
				M.MockWorkspaceApp,
				M.MockWorkspaceApp,
				M.MockWorkspaceApp,
				M.MockWorkspaceApp,
				M.MockWorkspaceApp,
				M.MockWorkspaceApp,
				M.MockWorkspaceApp,
			],
		},
		workspace: M.MockWorkspace,
	},
};

export const Disconnected: Story = {
	args: {
		agent: M.MockWorkspaceAgentDisconnected,
		initialMetadata: [],
	},
};

export const Connecting: Story = {
	args: {
		agent: M.MockWorkspaceAgentConnecting,
		initialMetadata: [],
	},
};

export const Timeout: Story = {
	args: {
		agent: M.MockWorkspaceAgentTimeout,
	},
};

export const Starting: Story = {
	args: {
		agent: M.MockWorkspaceAgentStarting,
	},
};

export const Started: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgentReady,
			logs_length: 1,
		},
	},
};

export const StartedNoMetadata: Story = {
	args: {
		...Started.args,
		initialMetadata: [],
	},
};

export const StartTimeout: Story = {
	args: {
		agent: M.MockWorkspaceAgentStartTimeout,
	},
};

export const StartError: Story = {
	args: {
		agent: M.MockWorkspaceAgentStartError,
	},
};

export const ShuttingDown: Story = {
	args: {
		agent: M.MockWorkspaceAgentShuttingDown,
	},
};

export const ShutdownTimeout: Story = {
	args: {
		agent: M.MockWorkspaceAgentShutdownTimeout,
	},
};

export const ShutdownError: Story = {
	args: {
		agent: M.MockWorkspaceAgentShutdownError,
	},
};

export const Off: Story = {
	args: {
		agent: M.MockWorkspaceAgentOff,
	},
};

export const ShowingPortForward: Story = {
	decorators: [
		withProxyProvider({
			proxy: getPreferredProxy(
				M.MockWorkspaceProxies,
				M.MockPrimaryWorkspaceProxy,
			),
			proxies: M.MockWorkspaceProxies,
		}),
	],
};

export const Outdated: Story = {
	beforeEach: () => {
		spyOn(API, "getBuildInfo").mockResolvedValue({
			...M.MockBuildInfo,
			version: "v99.999.9999+c1cdf14",
			agent_api_version: "1.0",
		});
	},
	args: {
		agent: M.MockWorkspaceAgentOutdated,
		workspace: M.MockWorkspace,
	},
};

export const Deprecated: Story = {
	beforeEach: () => {
		spyOn(API, "getBuildInfo").mockResolvedValue({
			...M.MockBuildInfo,
			version: "v99.999.9999+c1cdf14",
			agent_api_version: "2.0",
		});
	},
	args: {
		agent: M.MockWorkspaceAgentDeprecated,
		workspace: M.MockWorkspace,
	},
};

export const HideApp: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgent,
			apps: [
				{
					...M.MockWorkspaceApp,
					hidden: true,
				},
			],
		},
	},
};

export const GroupApp: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgent,
			apps: [
				{
					...M.MockWorkspaceApp,
					group: "group",
				},
			],
		},
	},

	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByText("group"));
	},
};

export const Devcontainer: Story = {
	parameters: {
		queries: [
			{
				key: ["agents", M.MockWorkspaceAgent.id, "containers"],
				data: {
					devcontainers: [M.MockWorkspaceAgentDevcontainer],
					containers: [M.MockWorkspaceAgentContainer],
				},
			},
		],
		webSocket: [],
	},
};

export const FoundDevcontainer: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgentReady,
		},
	},
	parameters: {
		queries: [
			{
				key: ["agents", M.MockWorkspaceAgentReady.id, "containers"],
				data: {
					devcontainers: [
						{
							...M.MockWorkspaceAgentDevcontainer,
							status: "stopped",
							container: undefined,
							agent: undefined,
						},
					],
					containers: [],
				},
			},
		],
		webSocket: [],
	},
};
