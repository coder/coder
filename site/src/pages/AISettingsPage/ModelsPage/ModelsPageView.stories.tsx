import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type { ChatModelConfig } from "#/api/typesGenerated";
import ModelsPageView from "./ModelsPageView";
import {
	mockAnthropicProviderState,
	mockClaude,
	mockDisabledModel,
	mockGPT5,
	mockOpenAIProviderState,
	mockProviderStateWithoutKey,
} from "./testFixtures";

const meta: Meta<typeof ModelsPageView> = {
	title: "pages/AISettingsPage/ModelsPage/ModelsPageView",
	component: ModelsPageView,
	args: {
		isLoading: false,
		error: null,
		models: [mockGPT5, mockClaude, mockDisabledModel],
		providerStates: [mockOpenAIProviderState, mockAnthropicProviderState],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/models" },
			routing: [
				{ path: "/ai/settings/models", useStoryElement: true },
				{ path: "/ai/settings/models/add", useStoryElement: true },
				{ path: "/ai/settings/models/:modelId", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ModelsPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("GPT-5")).toBeInTheDocument();
		await expect(canvas.getByText("Claude Sonnet 4.5")).toBeInTheDocument();
		// Provider column shows each model's provider label.
		await expect(canvas.getAllByText("OpenAI").length).toBeGreaterThan(0);
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		// Status shows Enabled/Disabled badges with a Default badge on the default.
		await expect(canvas.getAllByText("Enabled").length).toBeGreaterThan(0);
		await expect(canvas.getByText("Default")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		models: [],
	},
};

export const Empty: Story = {
	args: {
		models: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("No models configured")).toBeInTheDocument();
	},
};

export const EmptyWithoutManageableProvider: Story = {
	args: {
		models: [],
		providerStates: [mockProviderStateWithoutKey],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/connect a/i)).toBeInTheDocument();
	},
};

export const LoadError: Story = {
	args: {
		error: new Error("Failed to load models"),
		models: [],
	},
};

export const AddModelDropdownListsProviders: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /add model/i }));
		// The dropdown lets the admin pick which provider to add a model for.
		await expect(screen.getByText(/select a provider/i)).toBeInTheDocument();
		await expect(
			screen.getByRole("menuitem", { name: /anthropic/i }),
		).toBeInTheDocument();
	},
};

const manyModels: ChatModelConfig[] = Array.from({ length: 23 }, (_, i) => ({
	...mockClaude,
	id: `model-${i}`,
	model: `model-${i}`,
	display_name: `Model ${i}`,
	is_default: false,
}));

export const Paginated: Story = {
	args: {
		models: manyModels,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// First page shows the first 10 models and the total count.
		await expect(canvas.getByText("Model 0")).toBeInTheDocument();
		await expect(canvas.queryByText("Model 10")).not.toBeInTheDocument();
		await expect(canvas.getByText(/Showing/)).toBeInTheDocument();
		// Advancing a page reveals later models.
		await userEvent.click(canvas.getByRole("button", { name: /next page/i }));
		await expect(canvas.getByText("Model 10")).toBeInTheDocument();
		await expect(canvas.queryByText("Model 0")).not.toBeInTheDocument();
	},
};
