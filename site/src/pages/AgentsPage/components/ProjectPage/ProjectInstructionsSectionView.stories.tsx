import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	MockChatProjectInstructions,
	MockUnsetChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectInstructionsSectionView } from "./ProjectInstructionsSection";

const saved = MockChatProjectInstructions.instructions;

const meta: Meta<typeof ProjectInstructionsSectionView> = {
	title: "pages/AgentsPage/ProjectPage/ProjectInstructionsSectionView",
	component: ProjectInstructionsSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		instructions: MockChatProjectInstructions,
		loadError: undefined,
		onRetry: fn(),
		canEdit: true,
		draft: undefined,
		onDraftChange: fn(),
		isSaving: false,
		isDeleting: false,
		saveError: undefined,
		deleteError: undefined,
		onSave: fn(),
		onDelete: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectInstructionsSectionView>;

export const Saved: Story = {};

export const Empty: Story = {
	args: { instructions: MockUnsetChatProjectInstructions },
};

export const ReadOnly: Story = { args: { canEdit: false } };

export const ReadOnlyEmpty: Story = {
	args: { canEdit: false, instructions: MockUnsetChatProjectInstructions },
};

export const UnsavedChanges: Story = {
	args: {
		draft: { text: `${saved}\nPrefer small pull requests.`, base: saved },
	},
};

export const Saving: Story = {
	args: {
		draft: { text: `${saved}\nPrefer small pull requests.`, base: saved },
		isSaving: true,
	},
};

export const Clearing: Story = { args: { draft: { text: "", base: saved } } };

export const Deleting: Story = {
	args: { draft: { text: "", base: saved }, isDeleting: true },
};

/** Someone else saved while the draft was open. */
export const Conflict: Story = {
	args: {
		instructions: {
			...MockChatProjectInstructions,
			instructions: "Edited in another tab.",
		},
		draft: { text: `${saved}\nMy edit.`, base: saved },
	},
};

export const SaveError: Story = {
	args: {
		draft: { text: "Too long", base: saved },
		saveError: mockApiError({
			message: "Instructions exceed maximum length.",
			detail: "Maximum length is 131072 bytes, got 140000.",
		}),
	},
};

export const Loading: Story = { args: { instructions: undefined } };

export const LoadError: Story = {
	args: {
		instructions: undefined,
		loadError: mockApiError({ message: "Failed to load instructions." }),
	},
};

/** A failed refetch keeps the cached instructions and shows the error. */
export const RefetchError: Story = {
	args: {
		loadError: mockApiError({ message: "Failed to refresh instructions." }),
	},
};
