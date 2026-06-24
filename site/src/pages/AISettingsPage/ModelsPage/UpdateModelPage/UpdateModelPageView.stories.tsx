import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { withToaster } from "#/testHelpers/storybook";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
	mockGPT5,
} from "../testFixtures";
import UpdateModelPageView from "./UpdateModelPageView";

const meta: Meta<typeof UpdateModelPageView> = {
	title: "pages/AISettingsPage/ModelsPage/UpdateModelPageView",
	component: UpdateModelPageView,
	decorators: [withToaster],
	args: {
		model: mockGPT5,
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		onProviderChange: fn(),
		isSaving: false,
		isDeleting: false,
		onUpdateModel: fn(async () => undefined),
		onDeleteModel: fn(async () => undefined),
		onDuplicate: fn(),
		onToggleEnabled: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UpdateModelPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: /^update model$/i }),
		).toBeVisible();
		await expect(canvas.getByLabelText(/model identifier/i)).toBeEnabled();
	},
};
