import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MockChatProject } from "#/testHelpers/entities";
import { MoveToUnpinnedProjectDialog } from "./MoveToUnpinnedProjectDialog";

const meta: Meta<typeof MoveToUnpinnedProjectDialog> = {
	title: "pages/AgentsPage/MoveToUnpinnedProjectDialog",
	component: MoveToUnpinnedProjectDialog,
	args: { project: MockChatProject, open: true, onClose: fn() },
};

export default meta;
type Story = StoryObj<typeof MoveToUnpinnedProjectDialog>;

export const Default: Story = {};
