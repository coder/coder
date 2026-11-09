import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { OrganizationInstructionsSettings } from "./OrganizationInstructionsSettings";

const meta: Meta<typeof OrganizationInstructionsSettings> = {
	title:
		"pages/AISettingsPage/InstructionsPage/OrganizationInstructionsSettings",
	component: OrganizationInstructionsSettings,
	args: {
		systemPrompt:
			"Use the platform team's templates and run make lint before opening a pull request.",
		isLoading: false,
		loadError: null,
		refetchError: null,
		canEdit: true,
		onSave: fn(),
		isSaving: false,
		saveError: null,
		onResetSave: fn(),
	},
};
export default meta;
type Story = StoryObj<typeof OrganizationInstructionsSettings>;

export const Default: Story = {};

export const Loading: Story = {
	args: { systemPrompt: undefined, isLoading: true },
};

export const LoadError: Story = {
	args: {
		systemPrompt: undefined,
		loadError: mockApiError({
			message: "Failed to load organization instructions.",
		}),
	},
};

export const RefetchError: Story = {
	args: {
		refetchError: mockApiError({
			message: "Internal error fetching organization chat system prompt.",
		}),
	},
};

export const SaveError: Story = {
	args: {
		saveError: mockApiError({
			message: "System prompt exceeds the maximum length.",
		}),
	},
	// The error follows a failed save of an edit, so the field is dirty.
	play: async ({ canvasElement }) => {
		await userEvent.type(
			within(canvasElement).getByRole("textbox", {
				name: "Organization instructions",
			}),
			" Keep pull requests small.",
		);
	},
};

export const ReadOnly: Story = {
	args: { canEdit: false },
};
