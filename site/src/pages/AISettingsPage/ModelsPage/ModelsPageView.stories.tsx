import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type { ChatModelConfig } from "#/api/typesGenerated";
import ModelsPageView from "./ModelsPageView";
import {
	MockAnthropicProviderState,
	MockBedrockProviderState,
	MockDisabledProviderState,
	MockOpenAIProviderState,
	mockBedrockClaude,
	mockClaude,
	mockDisabledModel,
	mockGPT5,
	mockProviderDisabledModel,
} from "./testFixtures";

const meta: Meta<typeof ModelsPageView> = {
	title: "pages/AISettingsPage/ModelsPage/ModelsPageView",
	component: ModelsPageView,
	args: {
		isLoading: false,
		error: null,
		models: [mockGPT5, mockClaude, mockDisabledModel, mockBedrockClaude],
		providerStates: [
			MockOpenAIProviderState,
			MockAnthropicProviderState,
			MockBedrockProviderState,
		],
		providerTypeByID: new Map<string, string>([
			["prov-openai", "openai"],
			["prov-anthropic", "anthropic"],
			["prov-bedrock", "bedrock"],
		]),
	},
	parameters: {
		// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
		pixel: { exclude: true },
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
		await expect(
			canvas.getByRole("button", { name: /add model/i }),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("searchbox", { name: /search models/i }),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("combobox", { name: /filter by provider/i }),
		).toBeInTheDocument();
		await expect(canvas.getByText("GPT-5")).toBeInTheDocument();
		await expect(canvas.getByText("Claude Sonnet 4.5")).toBeInTheDocument();
		await expect(canvas.getAllByText("OpenAI").length).toBeGreaterThan(0);
		await expect(canvas.getByText("Anthropic")).toBeInTheDocument();
		await expect(
			canvas.getByText("Claude Sonnet 4.5 (Bedrock)"),
		).toBeInTheDocument();
		await expect(canvas.getByText("AWS Bedrock")).toBeInTheDocument();
		// The provider icon is decorative (alt=""), so its name comes from the
		// visible label asserted above rather than the image alt text.
		await expect(canvas.getAllByText("Enabled").length).toBeGreaterThan(0);
		await expect(canvas.getByText("Default")).toBeInTheDocument();
		await expect(canvas.getByText("Disabled")).toBeInTheDocument();

		// The Add model menu lists each provider by exact accessible name; a
		// regressed icon would turn a name into "Anthropic Anthropic".
		await userEvent.click(canvas.getByRole("button", { name: /add model/i }));
		const menu = await within(document.body).findByRole("menu");
		await within(menu).findByRole("menuitem", { name: "Anthropic" });
		await userEvent.keyboard("{Escape}");
	},
};

export const SearchByName: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const search = canvas.getByRole("searchbox", { name: /search models/i });
		await userEvent.type(search, "claude");
		await expect(canvas.getByText("Claude Sonnet 4.5")).toBeInTheDocument();
		await expect(canvas.queryByText("GPT-5")).not.toBeInTheDocument();
		await expect(canvas.queryByText("GPT-4o mini")).not.toBeInTheDocument();
	},
};

export const FilterByProvider: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const providerFilter = canvas.getByRole("combobox", {
			name: /filter by provider/i,
		});
		await userEvent.click(providerFilter);
		const listbox = await within(document.body).findByRole("listbox");
		const anthropicOption = await within(listbox).findByRole("option", {
			name: "Anthropic",
		});
		await userEvent.click(anthropicOption);
		await expect(canvas.getByText("Claude Sonnet 4.5")).toBeInTheDocument();
		await expect(canvas.queryByText("GPT-5")).not.toBeInTheDocument();
		await expect(canvas.queryByText("GPT-4o mini")).not.toBeInTheDocument();
	},
};

export const NoMatchingModels: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const search = canvas.getByRole("searchbox", { name: /search models/i });
		await userEvent.type(search, "no-such-model");
		await expect(
			canvas.getByText("No models match your filters"),
		).toBeInTheDocument();
	},
};

export const DisabledProviderModelsStillListed: Story = {
	args: {
		models: [mockGPT5, mockProviderDisabledModel],
		providerStates: [MockOpenAIProviderState, MockDisabledProviderState],
		providerTypeByID: new Map<string, string>([
			["prov-openai", "openai"],
			["prov-openai-disabled", "openai"],
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("GPT-4o Secondary")).toBeInTheDocument();
		await expect(canvas.getByText("OpenAI Secondary")).toBeInTheDocument();
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
		await expect(
			canvas.getAllByRole("button", { name: /add model/i }).length,
		).toBe(2);
	},
};

export const LoadError: Story = {
	args: {
		error: new Error("Failed to load models"),
		models: [],
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
		await expect(canvas.getByText("Model 0")).toBeInTheDocument();
		await expect(canvas.queryByText("Model 10")).not.toBeInTheDocument();
		await expect(canvas.getByText(/Showing/)).toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: /next page/i }));
		await expect(canvas.getByText("Model 10")).toBeInTheDocument();
		await expect(canvas.queryByText("Model 0")).not.toBeInTheDocument();
	},
};
