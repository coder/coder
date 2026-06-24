import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { withToaster } from "#/testHelpers/storybook";
import {
	MockAnthropicProviderState,
	MockOpenAIProviderState,
} from "../testFixtures";
import AddModelPageView from "./AddModelPageView";

const meta: Meta<typeof AddModelPageView> = {
	title: "pages/AISettingsPage/ModelsPage/AddModelPageView",
	component: AddModelPageView,
	decorators: [withToaster],
	args: {
		isLoading: false,
		providerStates: [MockOpenAIProviderState, MockAnthropicProviderState],
		selectedProviderState: MockOpenAIProviderState,
		isSaving: false,
		onProviderChange: fn(),
		onCreateModel: fn(async () => undefined),
	},
};

export default meta;
type Story = StoryObj<typeof AddModelPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: /add an? OpenAI model/i }),
		).toBeInTheDocument();
	},
};

export const WebSearchDependentFields: Story = {
	args: { selectedProviderState: MockAnthropicProviderState },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /provider configuration/i }),
		);
		expect(canvas.queryByLabelText(/allowed domains/i)).not.toBeInTheDocument();
		const webSearchField = canvas.getByRole("radiogroup", {
			name: /web search enabled/i,
		});
		await userEvent.click(
			within(webSearchField).getByRole("radio", { name: /on/i }),
		);
		const allowed = await canvas.findByLabelText(/allowed domains/i);
		const blocked = await canvas.findByLabelText(/blocked domains/i);
		await userEvent.type(allowed, "example.com");
		expect(blocked).toBeDisabled();
	},
};

export const ProviderNotFound: Story = {
	args: { selectedProviderState: null },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Provider not found")).toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: { isLoading: true },
};
