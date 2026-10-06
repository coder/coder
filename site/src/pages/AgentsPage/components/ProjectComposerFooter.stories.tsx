import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { MockChatProject } from "#/testHelpers/entities";
import { ProjectComposerFooter } from "./ProjectComposerFooter";

const meta: Meta<typeof ProjectComposerFooter> = {
	title: "pages/AgentsPage/ProjectComposerFooter",
	component: ProjectComposerFooter,
	args: { project: MockChatProject },
};

export default meta;
type Story = StoryObj<typeof ProjectComposerFooter>;

export const Default: Story = {};

export const EditDialogOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Edit project" }),
		);
	},
};
