import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { MockChatProject } from "#/testHelpers/entities";
import { ProjectGeneralSectionView } from "./ProjectGeneralSection";

const meta: Meta<typeof ProjectGeneralSectionView> = {
	title: "pages/AgentsPage/ProjectPage/ProjectGeneralSectionView",
	component: ProjectGeneralSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		description: MockChatProject.description,
		icon: MockChatProject.icon,
		agentUsername: "agent-launch",
		onSaveDescription: async () => undefined,
		onSaveIcon: async () => undefined,
	},
};

export default meta;
type Story = StoryObj<typeof ProjectGeneralSectionView>;

export const Default: Story = {};

export const ReadOnly: Story = {
	args: { onSaveDescription: undefined, onSaveIcon: undefined },
};

export const NoDescription: Story = { args: { description: "" } };

export const EditingDescription: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Edit Description" }),
		);
	},
};

export const EditingIcon: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Edit Icon" }),
		);
	},
};
