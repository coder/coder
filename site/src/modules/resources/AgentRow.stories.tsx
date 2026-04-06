import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { workspaceAgentContainersKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { getPreferredProxy } from "#/contexts/ProxyContext";
import { chromatic } from "#/testHelpers/chromatic";
import * as M from "#/testHelpers/entities";
import {
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
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

const installScriptLogSource: TypesGen.WorkspaceAgentLogSource = {
	...M.MockWorkspaceAgentLogSource,
	id: "f2ee4b8d-b09d-4f4e-a1f1-5e4adf7d53bb",
	display_name: "Install Script",
};

const tabbedLogs = [
	{
		id: 100,
		level: "info",
		output: "startup: preparing workspace",
		source_id: M.MockWorkspaceAgentLogSource.id,
		created_at: new Date().toISOString(),
	},
	{
		id: 101,
		level: "info",
		output: "install: pnpm install",
		source_id: installScriptLogSource.id,
		created_at: new Date().toISOString(),
	},
	{
		id: 102,
		level: "info",
		output: "install: setup complete",
		source_id: installScriptLogSource.id,
		created_at: new Date().toISOString(),
	},
];

const overflowLogSources: TypesGen.WorkspaceAgentLogSource[] = [
	M.MockWorkspaceAgentLogSource,
	{
		...M.MockWorkspaceAgentLogSource,
		id: "58f5db69-5f78-496f-bce1-0686f5525aa1",
		display_name: "code-server",
		icon: "/icon/code.svg",
	},
	{
		...M.MockWorkspaceAgentLogSource,
		id: "f39d758c-bce2-4f41-8d70-58fdb1f0f729",
		display_name: "Install and start AgentAPI",
		icon: "/icon/claude.svg",
	},
	{
		...M.MockWorkspaceAgentLogSource,
		id: "bf7529b8-1787-4a20-b54f-eb894680e48f",
		display_name: "Mux",
		icon: "/icon/mux.svg",
	},
	{
		...M.MockWorkspaceAgentLogSource,
		id: "0d6ebde6-c534-4551-9f91-bfd98bfb04f4",
		display_name: "Portable Desktop",
		icon: "/icon/portable-desktop.svg",
	},
];

const overflowLogs = overflowLogSources.map((source, index) => ({
	id: 200 + index,
	level: "info",
	output: `${source.display_name}: line`,
	source_id: source.id,
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
				key: workspaceAgentContainersKey(M.MockWorkspaceAgent.id),
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
				key: workspaceAgentContainersKey(M.MockWorkspaceAgentReady.id),
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

export const LogsTabs: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgentReady,
			logs_length: tabbedLogs.length,
			log_sources: [M.MockWorkspaceAgentLogSource, installScriptLogSource],
		},
	},
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(tabbedLogs),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Logs" }));

		const installTab = await canvas.findByRole("tab", {
			name: "Install Script",
		});
		await userEvent.click(installTab);

		await waitFor(() =>
			expect(installTab).toHaveAttribute("data-state", "active"),
		);
		await waitFor(() =>
			expect(
				canvas.queryByText("startup: preparing workspace"),
			).not.toBeInTheDocument(),
		);
		await expect(canvas.getByText("install: pnpm install")).toBeVisible();
	},
};

export const LogsTabsOverflow: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgentReady,
			logs_length: overflowLogs.length,
			log_sources: overflowLogSources,
		},
	},
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(overflowLogs),
			},
		],
	},
	render: (args) => (
		<div className="max-w-[320px]">
			<AgentRow {...args} />
		</div>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const page = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "Logs" }));
		await userEvent.click(
			canvas.getByRole("button", { name: "More log tabs" }),
		);
		const overflowItems = await page.findAllByRole("menuitemradio");
		const selectedItem = overflowItems[0];
		const selectedSource = selectedItem.textContent;
		if (!selectedSource) {
			throw new Error("Overflow menu item must have text content.");
		}
		await userEvent.click(selectedItem);
		await waitFor(() =>
			expect(canvas.getByText(`${selectedSource}: line`)).toBeVisible(),
		);
	},
};
