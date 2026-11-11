import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
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

export const PinPending: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "pinChatProject").mockReturnValue(
			new Promise(() => {}),
		);
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Pin project" }),
		);
	},
};
