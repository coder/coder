import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	MockChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectInstructionsDialog } from "./ProjectInstructionsDialog";

const meta: Meta<typeof ProjectInstructionsDialog> = {
	title: "pages/AgentsPage/ProjectPage/ProjectInstructionsDialog",
	component: ProjectInstructionsDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		instructions: "",
		isSaving: false,
		isDeleting: false,
		error: undefined,
		onSave: fn(),
		onDelete: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectInstructionsDialog>;

export const Create: Story = {};

export const Edit: Story = {
	args: { instructions: MockChatProjectInstructions.instructions },
};

export const SaveError: Story = {
	args: {
		instructions: MockChatProjectInstructions.instructions,
		error: mockApiError({ message: "Instructions exceed maximum length." }),
	},
};
