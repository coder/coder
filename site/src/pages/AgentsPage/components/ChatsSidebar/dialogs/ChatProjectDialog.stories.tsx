import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

const meta: Meta<typeof ChatProjectDialog> = {
	title: "pages/AgentsPage/ChatProjectDialog",
	component: ChatProjectDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		isSubmitting: false,
		error: undefined,
		onSubmit: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatProjectDialog>;

export const NewProject: Story = {};

export const EditProject: Story = {
	args: {
		project: { ...MockChatProject, icon: "/emojis/1f680.png" },
	},
};

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(body.getByLabelText(/Name/), "x".repeat(65));
	},
};

export const Submitting: Story = {
	args: {
		project: MockChatProject,
		isSubmitting: true,
	},
};

export const SaveError: Story = {
	args: {
		error: mockApiError({
			message:
				"You can have at most 100 chat projects. Delete a project to create another.",
		}),
	},
};
