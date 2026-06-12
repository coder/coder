import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ModelSelector, type ModelSelectorOption } from "./ModelSelector";

const anthropicModels: ModelSelectorOption[] = [
	{
		id: "anthropic/haiku-4.6",
		provider: "anthropic",
		model: "claude-haiku-4-6",
		displayName: "Haiku 4.6",
		contextLimit: 200_000,
	},
	{
		id: "anthropic/sonnet-4.6",
		provider: "anthropic",
		model: "claude-sonnet-4-6",
		displayName: "Sonnet 4.6",
		contextLimit: 1_000_000,
	},
	{
		id: "anthropic/opus-4.6",
		provider: "anthropic",
		model: "claude-opus-4-6",
		displayName: "Opus 4.6",
		contextLimit: 1_000_000,
	},
	{
		id: "anthropic/opus-4.7",
		provider: "anthropic",
		model: "claude-opus-4-7",
		displayName: "Opus 4.7",
		contextLimit: 1_000_000,
		effort: "xhigh",
	},
	{
		id: "anthropic/opus-4.8",
		provider: "anthropic",
		model: "claude-opus-4-8",
		displayName: "Opus 4.8",
		contextLimit: 1_000_000,
	},
];

const openAIModels: ModelSelectorOption[] = [
	{
		id: "openai/gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
		contextLimit: 128_000,
	},
	{
		id: "openai/gpt-4o-mini",
		provider: "openai",
		model: "gpt-4o-mini",
		displayName: "GPT-4o Mini",
		contextLimit: 128_000,
	},
	{
		id: "openai/o3",
		provider: "openai",
		model: "o3",
		displayName: "o3",
		contextLimit: 200_000,
		effort: "high",
	},
];

const allModels: ModelSelectorOption[] = [...anthropicModels, ...openAIModels];

const formatProviderLabel = (provider: string): string => {
	const labels: Record<string, string> = {
		openai: "OpenAI",
		anthropic: "Anthropic",
	};
	return labels[provider] ?? provider;
};

const meta: Meta<typeof ModelSelector> = {
	title: "pages/AgentsPage/ChatElements/ModelSelector",
	component: ModelSelector,
	decorators: [
		(Story) => (
			<div className="flex w-72 justify-start rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		options: anthropicModels,
		value: "",
		onValueChange: fn(),
		formatProviderLabel,
	},
};

export default meta;
type Story = StoryObj<typeof ModelSelector>;

// ---------------------------------------------------------------------------
// Trigger (closed) stories
// ---------------------------------------------------------------------------

export const Default: Story = {};

export const WithSelectedValue: Story = {
	args: {
		value: "anthropic/opus-4.7",
	},
};

export const CustomPlaceholder: Story = {
	args: {
		placeholder: "Choose a model…",
	},
};

export const Disabled: Story = {
	args: {
		disabled: true,
		value: "anthropic/opus-4.7",
	},
};

// ---------------------------------------------------------------------------
// Open-by-default stories (visualize the dropdown without an interaction).
// ---------------------------------------------------------------------------

export const OpenSingleProvider: Story = {
	args: {
		options: anthropicModels,
		value: "anthropic/opus-4.7",
		open: true,
	},
};

export const OpenMultipleProviders: Story = {
	args: {
		options: allModels,
		value: "openai/o3",
		open: true,
	},
};

export const OpenWithoutEffort: Story = {
	args: {
		// None of the selected model's siblings have an effort field
		// set, so the Effort row should not render.
		options: anthropicModels.map(({ effort: _effort, ...rest }) => rest),
		value: "anthropic/sonnet-4.6",
		open: true,
	},
};

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

export const NoOptions: Story = {
	args: {
		options: [],
		value: "",
	},
};

// ---------------------------------------------------------------------------
// Play function: interaction smoke test for click-to-select.
// ---------------------------------------------------------------------------

export const SelectsModel: Story = {
	args: {
		options: openAIModels,
		value: "",
		onValueChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const trigger = canvas.getByRole("combobox");
		await userEvent.click(trigger);

		// The popover is portaled, so the listbox renders in document.body.
		const option = await within(document.body).findByText("GPT-4o Mini");
		await userEvent.click(option);

		expect(args.onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	},
};

// Search filters the visible options by display name.
export const SearchFiltersOptions: Story = {
	args: {
		options: anthropicModels,
		value: "",
		open: true,
	},
	play: async () => {
		const body = within(document.body);
		const search = await body.findByPlaceholderText("Search...");
		await userEvent.type(search, "opus");

		expect(await body.findByText("Opus 4.6")).toBeInTheDocument();
		expect(await body.findByText("Opus 4.7")).toBeInTheDocument();
		expect(await body.findByText("Opus 4.8")).toBeInTheDocument();
		expect(body.queryByText("Haiku 4.6")).not.toBeInTheDocument();
		expect(body.queryByText("Sonnet 4.6")).not.toBeInTheDocument();
	},
};
