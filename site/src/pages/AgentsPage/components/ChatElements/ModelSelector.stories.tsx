import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ModelSelector, type ModelSelectorOption } from "./ModelSelector";

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
		id: "openai/o3-mini",
		provider: "openai",
		model: "o3-mini",
		displayName: "o3-mini",
		contextLimit: 200_000,
	},
];

const anthropicModels: ModelSelectorOption[] = [
	{
		id: "anthropic/claude-sonnet-4",
		provider: "anthropic",
		model: "claude-sonnet-4-20250514",
		displayName: "Claude Sonnet 4",
		contextLimit: 200_000,
	},
	{
		id: "anthropic/claude-haiku-3.5",
		provider: "anthropic",
		model: "claude-3-5-haiku-20241022",
		displayName: "Claude 3.5 Haiku",
		contextLimit: 200_000,
	},
	{
		id: "anthropic/claude-opus-4",
		provider: "anthropic",
		model: "claude-opus-4-20250514",
		displayName: "Claude Opus 4",
		contextLimit: 1_000_000,
	},
];

const googleModels: ModelSelectorOption[] = [
	{
		id: "google/gemini-2.5-pro",
		provider: "google",
		model: "gemini-2.5-pro",
		displayName: "Gemini 2.5 Pro",
		contextLimit: 1_000_000,
	},
	{
		id: "google/gemini-2.5-flash",
		provider: "google",
		model: "gemini-2.5-flash",
		displayName: "Gemini 2.5 Flash",
		contextLimit: 1_000_000,
	},
];

const allModels: ModelSelectorOption[] = [
	...openAIModels,
	...anthropicModels,
	...googleModels,
];

const meta: Meta<typeof ModelSelector> = {
	title: "pages/AgentsPage/ChatElements/ModelSelector",
	component: ModelSelector,
	decorators: [
		(Story) => (
			<div className="w-72 rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		options: openAIModels,
		value: "",
		onValueChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ModelSelector>;

// ---------------------------------------------------------------------------
// Single provider stories
// ---------------------------------------------------------------------------

export const Default: Story = {};

export const WithSelectedValue: Story = {
	args: {
		value: "openai/gpt-4o",
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
		value: "openai/gpt-4o",
	},
};

// ---------------------------------------------------------------------------
// Multiple providers (grouped)
// ---------------------------------------------------------------------------

export const MultipleProviders: Story = {
	args: {
		options: allModels,
		value: "anthropic/claude-sonnet-4",
	},
};

export const MultipleProvidersWithCustomLabel: Story = {
	args: {
		options: allModels,
		value: "",
		formatProviderLabel: (provider: string) => {
			const labels: Record<string, string> = {
				openai: "OpenAI",
				anthropic: "Anthropic",
				google: "Google AI",
			};
			return labels[provider] ?? provider;
		},
	},
};

// ---------------------------------------------------------------------------
// Open dropdown state
// ---------------------------------------------------------------------------

export const OpenDropdown: Story = {
	args: {
		options: allModels,
		value: "anthropic/claude-sonnet-4",
		open: true,
		dropdownSide: "bottom",
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
// Play function - selection interaction
// ---------------------------------------------------------------------------

export const SelectsModel: Story = {
	args: {
		options: openAIModels,
		value: "",
		onValueChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// Open the popover by clicking the trigger.
		const trigger = canvas.getByRole("button");
		await userEvent.click(trigger);

		// The dropdown should appear with model options.
		const option = await within(document.body).findByText("GPT-4o Mini");
		await userEvent.click(option);

		expect(args.onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	},
};

// ---------------------------------------------------------------------------
// Play function - search interaction
// ---------------------------------------------------------------------------

export const SearchFiltersModels: Story = {
	args: {
		options: allModels,
		value: "",
		onValueChange: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Open the popover.
		const trigger = canvas.getByRole("button");
		await userEvent.click(trigger);

		// Type in the search input.
		const searchInput = within(document.body).getByPlaceholderText("Search...");
		await userEvent.type(searchInput, "claude");

		// Anthropic models should be visible, OpenAI should not.
		const body = within(document.body);
		expect(body.getByText("Claude Sonnet 4")).toBeVisible();
		expect(body.queryByText("GPT-4o")).not.toBeInTheDocument();
	},
};
