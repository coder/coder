import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import { MoveToUnpinnedProjectDialog } from "./MoveToUnpinnedProjectDialog";

const meta: Meta<typeof MoveToUnpinnedProjectDialog> = {
	title: "pages/AgentsPage/MoveToUnpinnedProjectDialog",
	component: MoveToUnpinnedProjectDialog,
	args: { project: MockChatProject, open: true, onClose: fn() },
};

export default meta;
type Story = StoryObj<typeof MoveToUnpinnedProjectDialog>;

export const Default: Story = {};

export const PinFailed: Story = {
	decorators: [withToaster],
	beforeEach: () => {
		spyOn(API.experimental, "pinChatProject").mockRejectedValue(
			mockApiError({ message: "Project pins are full." }),
		);
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Pin project" }),
		);
	},
};

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
