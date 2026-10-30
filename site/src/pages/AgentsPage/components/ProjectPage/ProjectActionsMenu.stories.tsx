import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { MockChatProject } from "#/testHelpers/entities";
import { ProjectActionsMenu } from "./ProjectActionsMenu";

const meta: Meta<typeof ProjectActionsMenu> = {
	title: "pages/AgentsPage/ProjectPage/ProjectActionsMenu",
	component: ProjectActionsMenu,
	args: { project: MockChatProject },
};

export default meta;
type Story = StoryObj<typeof ProjectActionsMenu>;

export const Closed: Story = {};

export const MenuOpen: Story = {
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Agent actions" }),
		);
	},
};

export const DeleteConfirmation: Story = {
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Agent actions" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Delete agent" }),
		);
	},
};

/** Viewers without delete permission see no menu. */
export const CannotDelete: Story = {
	args: {
		project: {
			...MockChatProject,
			permissions: { ...MockChatProject.permissions, delete: false },
		},
	},
};
