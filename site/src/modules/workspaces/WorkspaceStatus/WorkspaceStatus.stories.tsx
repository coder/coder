import { MockDormantWorkspace, MockWorkspace } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkspaceStatus } from "./WorkspaceStatus";

const meta: Meta<typeof WorkspaceStatus> = {
	title: "modules/workspaces/WorkspaceStatus",
	component: WorkspaceStatus,
	args: {
		workspace: MockWorkspace,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceStatus>;

export const Running: Story = {};

export const Dormant: Story = {
	args: {
		workspace: MockDormantWorkspace,
	},
};
