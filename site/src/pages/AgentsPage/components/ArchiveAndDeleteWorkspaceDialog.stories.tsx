import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import { MockWorkspace } from "#/testHelpers/entities";
import { ArchiveAndDeleteWorkspaceDialog } from "./ArchiveAndDeleteWorkspaceDialog";

const meta: Meta<typeof ArchiveAndDeleteWorkspaceDialog> = {
	title: "pages/AgentsPage/ArchiveAndDeleteWorkspaceDialog",
	component: ArchiveAndDeleteWorkspaceDialog,
	args: {
		workspace: MockWorkspace,
		onConfirm: fn(),
		onCancel: fn(),
	},
};
export default meta;
type Story = StoryObj<typeof ArchiveAndDeleteWorkspaceDialog>;

export const Default: Story = {};

export const NameConfirmed: Story = {
	play: async () => {
		const dialog = await screen.findByRole("dialog");
		await userEvent.type(
			within(dialog).getByLabelText(/name of the workspace/i),
			MockWorkspace.name,
		);
	},
};
