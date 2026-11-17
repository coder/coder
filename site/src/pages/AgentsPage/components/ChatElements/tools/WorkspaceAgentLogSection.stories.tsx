import type { Meta, StoryObj } from "@storybook/react-vite";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentLogs,
} from "#/testHelpers/entities";
import { withWebSocket } from "#/testHelpers/storybook";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceAgentLogSection } from "./WorkspaceAgentLogSection";

const mockOverflowedWorkspace = {
	...MockWorkspace,
	latest_build: {
		...MockWorkspace.latest_build,
		resources: MockWorkspace.latest_build.resources.map((resource) => ({
			...resource,
			agents: resource.agents?.map((agent) => ({
				...agent,
				logs_overflowed: true,
			})),
		})),
	},
};

const meta: Meta<typeof WorkspaceAgentLogSection> = {
	title: "pages/AgentsPage/ChatElements/tools/WorkspaceAgentLogSection",
	component: WorkspaceAgentLogSection,
	args: { status: "completed", buildId: MockWorkspace.latest_build.id },
	decorators: [
		withWebSocket,
		(Story) => (
			<ChatWorkspaceContext
				value={{
					workspaceId: MockWorkspace.id,
					agentId: MockWorkspaceAgent.id,
				}}
			>
				<Story />
			</ChatWorkspaceContext>
		),
	],
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify(MockWorkspaceAgentLogs.slice(0, 4)),
			},
		],
	},
};
export default meta;
type Story = StoryObj<typeof WorkspaceAgentLogSection>;

export const WithLogs: Story = {
	parameters: {
		queries: [{ key: workspaceByIdKey(MockWorkspace.id), data: MockWorkspace }],
	},
};

export const LogsOverflowed: Story = {
	parameters: {
		queries: [
			{
				key: workspaceByIdKey(MockWorkspace.id),
				data: mockOverflowedWorkspace,
			},
		],
	},
};
