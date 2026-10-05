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
		saveError: undefined,
		deleteError: undefined,
		onDraftChange: fn(),
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
		saveError: mockApiError({
			message: "Instructions exceed maximum length.",
			detail: "Maximum length is 131072 bytes, got 140000.",
		}),
	},
};

export const InvisibleCharacterWarning: Story = {
	args: { instructions: "Use TypeScript\u200B for new code.\u2060" },
};

export const Saving: Story = {
	args: {
		instructions: MockChatProjectInstructions.instructions,
		isSaving: true,
	},
};

export const Deleting: Story = {
	args: {
		instructions: MockChatProjectInstructions.instructions,
		isDeleting: true,
	},
};
