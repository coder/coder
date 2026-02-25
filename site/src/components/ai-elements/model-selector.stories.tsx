import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { fn } from "storybook/test";
import { type ModelSelectorOption, ModelSelector } from "./model-selector";

const openAIModels: ModelSelectorOption[] = [
	{
		id: "openai/gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
	{
		id: "openai/gpt-4o-mini",
		provider: "openai",
		model: "gpt-4o-mini",
		displayName: "GPT-4o Mini",
	},
	{
		id: "openai/o3-mini",
		provider: "openai",
		model: "o3-mini",
		displayName: "o3-mini",
	},
];

const anthropicModels: ModelSelectorOption[] = [
	{
		id: "anthropic/claude-sonnet-4",
		provider: "anthropic",
		model: "claude-sonnet-4-20250514",
		displayName: "Claude Sonnet 4",
	},
	{
		id: "anthropic/claude-haiku-3.5",
		provider: "anthropic",
		model: "claude-3-5-haiku-20241022",
		displayName: "Claude 3.5 Haiku",
	},
];

const allModels: ModelSelectorOption[] = [...openAIModels, ...anthropicModels];

const meta: Meta<typeof ModelSelector> = {
	title: "components/ai-elements/ModelSelector",
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
			};
			return labels[provider] ?? provider;
		},
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
// Play function – selection interaction
// ---------------------------------------------------------------------------

export const SelectsModel: Story = {
	args: {
		options: openAIModels,
		value: "",
		onValueChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// Open the popover by clicking the trigger button.
		const trigger = canvas.getByRole("button", { name: "Select model" });
		await userEvent.click(trigger);

		// The dropdown should appear with model options.
		const listbox = await within(document.body).findByRole("listbox");
		const option = within(listbox).getByText("GPT-4o Mini");
		await userEvent.click(option);

		expect(args.onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	},
};
