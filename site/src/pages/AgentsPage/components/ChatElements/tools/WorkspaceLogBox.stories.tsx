import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type { WorkspaceAgentLog } from "#/api/typesGenerated";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentLogs,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { withWebSocket } from "#/testHelpers/storybook";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceLogBox } from "./WorkspaceLogBox";

type SocketMessage = { event: "message"; data: string };

const buildLogEvents = MockWorkspaceBuildLogs.map<SocketMessage>((log) => ({
	event: "message",
	data: JSON.stringify(log),
}));

const agentLogEvents = (
	logs: readonly WorkspaceAgentLog[],
): SocketMessage[] => [{ event: "message", data: JSON.stringify(logs) }];

const agentLogLines: WorkspaceAgentLog[] = Array.from(
	{ length: 30 },
	(_, i) => ({
		...MockWorkspaceAgentLogs[0],
		id: 500000 + i,
		created_at: new Date(Date.UTC(2024, 0, 1, 0, 0, i)).toISOString(),
		output: `agent line ${i}`,
	}),
);

const meta: Meta<typeof WorkspaceLogBox> = {
	title: "pages/AgentsPage/ChatElements/tools/WorkspaceLogBox",
	component: WorkspaceLogBox,
	args: {
		status: "completed",
		buildId: MockWorkspace.latest_build.id,
		action: "start",
	},
	decorators: [
		withWebSocket,
		(Story) => (
			<ChatWorkspaceContext
				value={{
					workspaceId: MockWorkspace.id,
					buildId: MockWorkspace.latest_build.id,
					agentId: MockWorkspaceAgent.id,
				}}
			>
				<div className="max-w-2xl">
					<Story />
				</div>
			</ChatWorkspaceContext>
		),
	],
	parameters: {
		queries: [{ key: workspaceByIdKey(MockWorkspace.id), data: MockWorkspace }],
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceLogBox>;

export const BuildAndAgentLogs: Story = {
	parameters: {
		webSocket: {
			"/workspacebuilds/": buildLogEvents,
			"/workspaceagents/": agentLogEvents(MockWorkspaceAgentLogs),
		},
	},
};

export const WithNotice: Story = {
	args: {
		notice:
			"A startup script or dev container failed. Skills, instructions, or tools they set up may be missing.",
	},
	parameters: {
		webSocket: {
			"/workspacebuilds/": buildLogEvents,
			"/workspaceagents/": agentLogEvents(MockWorkspaceAgentLogs),
		},
	},
};

export const WaitingForAgentStartup: Story = {
	args: { status: "running", buildId: undefined },
	parameters: {
		webSocket: { "/workspacebuilds/": buildLogEvents },
	},
};

export const LoadingBuildLogs: Story = {
	args: { status: "running", buildId: undefined },
	// Sockets open but receive no lines.
	parameters: { webSocket: {} },
};

export const StopBuildLogs: Story = {
	args: { action: "stop" },
	parameters: {
		webSocket: { "/workspacebuilds/": buildLogEvents },
	},
};

/** The box scrolled to its top by the user. */
export const ScrolledUp: Story = {
	parameters: {
		webSocket: {
			"/workspacebuilds/": buildLogEvents,
			"/workspaceagents/": agentLogEvents(agentLogLines),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("agent line 29");
		canvas.getByRole("region", { name: "Workspace log" }).scrollTop = 0;
	},
};

/**
 * The box below the fold of a scrolling transcript. Streamed lines must
 * not scroll the transcript (CODAGT-1184), so the capture shows the
 * transcript's top.
 */
export const BelowTranscriptFold: Story = {
	decorators: [
		(Story) => (
			<div style={{ height: 400, overflowY: "auto" }}>
				<div style={{ height: 2000 }}>
					<p>Earlier messages</p>
				</div>
				<Story />
			</div>
		),
	],
	parameters: {
		webSocket: {
			"/workspacebuilds/": buildLogEvents,
			"/workspaceagents/": agentLogEvents(agentLogLines),
		},
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByText("agent line 29");
	},
};
