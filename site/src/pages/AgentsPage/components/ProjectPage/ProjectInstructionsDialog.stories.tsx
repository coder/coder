import type { Meta, StoryObj } from "@storybook/react-vite";
import { useSyncExternalStore } from "react";
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

/** An error without an API message is labelled by the failed action. */
export const DeleteErrorWithoutMessage: Story = {
	args: {
		instructions: MockChatProjectInstructions.instructions,
		deleteError: mockApiError({ message: "" }),
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

/**
 * Stands in for the server's copy of the instructions, so the Conflict
 * story's play function can save a change from "another user" while the
 * dialog is open.
 */
const serverInstructions = (() => {
	let value = MockChatProjectInstructions.instructions;
	const listeners = new Set<() => void>();
	return {
		get: () => value,
		set: (next: string) => {
			value = next;
			for (const listener of listeners) {
				listener();
			}
		},
		subscribe: (listener: () => void) => {
			listeners.add(listener);
			return () => listeners.delete(listener);
		},
	};
})();

/** Another user saved new instructions while this editor was open. */
export const Conflict: Story = {
	beforeEach: () => {
		serverInstructions.set(MockChatProjectInstructions.instructions);
	},
	render: function ConflictStory(args) {
		const instructions = useSyncExternalStore(
			serverInstructions.subscribe,
			serverInstructions.get,
		);
		return <ProjectInstructionsDialog {...args} instructions={instructions} />;
	},
	play: () => {
		serverInstructions.set("Edited in another tab.");
	},
};
