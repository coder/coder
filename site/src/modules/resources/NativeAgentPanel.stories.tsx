import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import {
	MockPermissions,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { NativeAgentPanel } from "./NativeAgentPanel";

const mockOfflineAgent: WorkspaceAgent = {
	...MockWorkspaceAgent,
	status: "disconnected",
};

const meta = {
	title: "modules/resources/NativeAgentPanel",
	component: NativeAgentPanel,
	args: { agent: mockOfflineAgent, workspace: MockWorkspace },
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "padded",
		user: MockUserOwner,
		permissions: MockPermissions,
		experiments: ["native-agents"],
	},
} satisfies Meta<typeof NativeAgentPanel>;
export default meta;
type Story = StoryObj<typeof meta>;

export const WorkspaceOffline: Story = {};
