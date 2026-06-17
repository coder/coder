import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { workspaceAgentContainersKey } from "#/api/queries/workspaces";
import type { WorkspaceAgentLogSource } from "#/api/typesGenerated";
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

const fixedLogTimestamp = "2021-05-05T00:00:00.000Z";

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
	created_at: fixedLogTimestamp,
}));

const installScriptLogSource: WorkspaceAgentLogSource = {
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
		created_at: fixedLogTimestamp,
	},
	{
		id: 101,
		level: "info",
		output: "install: pnpm install",
		source_id: installScriptLogSource.id,
		created_at: fixedLogTimestamp,
	},
	{
		id: 102,
		level: "info",
		output: "install: setup complete",
		source_id: installScriptLogSource.id,
		created_at: fixedLogTimestamp,
	},
];

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
			...M.MockWorkspaceAgentReady,
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

export const ConnectingWithStartupLogs: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgentConnecting,
			logs_length: 1,
		},
		initialMetadata: [],
	},
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify([
					{
						id: 1,
						level: "info",
						output: "starting up",
						source_id: M.MockWorkspaceAgentLogSource.id,
						created_at: fixedLogTimestamp,
					},
				]),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Agent is connecting (hasConnectivityIssues=true) but no script has failed.
		// Old code snapped to the Startup Script tab; the fix keeps us on All Logs.
		const allLogsTab = await canvas.findByRole("tab", { name: "All Logs" });
		await waitFor(() =>
			expect(allLogsTab).toHaveAttribute("data-state", "active"),
		);
	},
};

export const Timeout: Story = {
	args: {
		agent: M.MockWorkspaceAgentTimeout,
	},
};

export const Starting: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgentStarting,
			logs_length: logs.length,
		},
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
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(
					M.MockWorkspaceAgentStartError.log_sources.flatMap((l, i) => {
						return [
							{
								id: i,
								level: "info",
								output: `running '${l.display_name}' script`,
								source_id: l.id,
								created_at: fixedLogTimestamp,
							},
							{
								id: i + 100,
								level: "error",
								output: `stderr from '${l.display_name}' script`,
								source_id: l.id,
								created_at: fixedLogTimestamp,
							},
						];
					}),
				),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// MockWorkspaceAgentStartError ships with a Startup Script whose script
		// has exit_code: 1, so the auto-select should land us there.
		const startupScriptTab = await canvas.findByRole("tab", {
			name: "Startup Script",
		});
		await waitFor(() =>
			expect(startupScriptTab).toHaveAttribute("data-state", "active"),
		);
	},
};

export const StartErrorWithoutFailedSourceLogs: Story = {
	args: {
		agent: M.MockWorkspaceAgentStartError,
	},
	parameters: {
		// Send log entries only for the OK script, mirroring the case where a
		// failed script never emitted any output. The selected tab must not be
		// initialized to a source that has no rendered tab.
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(
					M.MockWorkspaceAgentStartError.log_sources
						.filter((source) => {
							const script = M.MockWorkspaceAgentStartError.scripts.find(
								(s) => s.log_source_id === source.id,
							);
							return !script?.exit_code && script?.status === "ok";
						})
						.flatMap((source, i) => [
							{
								id: i,
								level: "info",
								output: `output from '${source.display_name}'`,
								source_id: source.id,
								created_at: fixedLogTimestamp,
							},
						]),
				),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for a non-failed source tab to render, confirming logs streamed in.
		await canvas.findByRole("tab", { name: "coder" });

		// All Logs must stay active because no failed source has rendered logs.
		const allLogsTab = canvas.getByRole("tab", { name: "All Logs" });
		await waitFor(() =>
			expect(allLogsTab).toHaveAttribute("data-state", "active"),
		);
	},
};

const NON_STARTUP_SCRIPT_SOURCE_ID = "install-script-source-id";

export const NonStartupScriptError: Story = {
	args: {
		agent: {
			...M.MockWorkspaceAgent,
			logs_length: 2,
			scripts: [
				// Startup Script succeeded.
				{
					...M.MockWorkspaceAgent.scripts[0],
					exit_code: 0,
					status: "ok",
				},
				// A non-startup script failed; that's the tab we should auto-select.
				{
					...M.MockWorkspaceAgent.scripts[0],
					id: "install-script-id",
					log_source_id: NON_STARTUP_SCRIPT_SOURCE_ID,
					exit_code: 1,
					status: "exit_failure",
					display_name: "Install Script",
				},
			],
			log_sources: [
				...M.MockWorkspaceAgent.log_sources,
				{
					...M.MockWorkspaceAgent.log_sources[0],
					id: NON_STARTUP_SCRIPT_SOURCE_ID,
					display_name: "Install Script",
				},
			],
		},
	},
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify([
					{
						id: 1,
						level: "info",
						output: "startup ok",
						source_id: M.MockWorkspaceAgentLogSource.id,
						created_at: fixedLogTimestamp,
					},
					{
						id: 2,
						level: "error",
						output: "install failed",
						source_id: NON_STARTUP_SCRIPT_SOURCE_ID,
						created_at: fixedLogTimestamp,
					},
				]),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Startup Script is OK; only Install Script failed. The auto-select must
		// follow the failure, not the position or display name.
		const installScriptTab = await canvas.findByRole("tab", {
			name: "Install Script",
		});
		await waitFor(() =>
			expect(installScriptTab).toHaveAttribute("data-state", "active"),
		);
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
			...M.MockWorkspaceAgentReady,
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
			...M.MockWorkspaceAgentReady,
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
