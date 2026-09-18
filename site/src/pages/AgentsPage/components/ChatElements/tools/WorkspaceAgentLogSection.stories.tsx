import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { WorkspaceAgentLogSection } from "./WorkspaceAgentLogSection";

const meta: Meta<typeof WorkspaceAgentLogSection> = {
	title: "pages/AgentsPage/ChatElements/tools/WorkspaceAgentLogSection",
	component: WorkspaceAgentLogSection,
	args: {
		status: "completed",
		buildId: MockWorkspace.latest_build.id,
	},
	decorators: [
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
		queries: [{ key: workspaceByIdKey(MockWorkspace.id), data: MockWorkspace }],
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceAgentLogSection>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaceAgentLogs").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};

export const FetchError: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaceAgentLogs").mockRejectedValue(
			new Error("Internal Server Error"),
		);
	},
};

export const CompletedEmptyLogs: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaceAgentLogs").mockResolvedValue([]);
	},
};
