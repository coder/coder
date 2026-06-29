import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ModelSelector, type ModelSelectorOption } from "./ModelSelector";
import { MockModelSelectorOption } from "./modelSelectorFixtures";

const openAIModels: ModelSelectorOption[] = [
	{
		...MockModelSelectorOption,
		id: "openai/gpt-4o",
		model: "gpt-4o",
		displayName: "GPT-4o",
		contextLimit: 128_000,
	},
	{
		...MockModelSelectorOption,
		id: "openai/gpt-4o-mini",
		model: "gpt-4o-mini",
		displayName: "GPT-4o Mini",
		contextLimit: 128_000,
	},
	{
		...MockModelSelectorOption,
		id: "openai/o3-mini",
		model: "o3-mini",
		displayName: "o3-mini",
		contextLimit: 200_000,
	},
];

const anthropicModels: ModelSelectorOption[] = [
	{
		...MockModelSelectorOption,
		id: "anthropic/claude-sonnet-4",
		provider: "anthropic",
		model: "claude-sonnet-4-20250514",
		displayName: "Claude Sonnet 4",
		contextLimit: 200_000,
	},
	{
		...MockModelSelectorOption,
		id: "anthropic/claude-haiku-3.5",
		provider: "anthropic",
		model: "claude-3-5-haiku-20241022",
		displayName: "Claude 3.5 Haiku",
		contextLimit: 200_000,
	},
];

const allModels: ModelSelectorOption[] = [...openAIModels, ...anthropicModels];

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
		options: openAIModels,
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
		value: "openai/gpt-4o",
	},
};

export const CustomPlaceholder: Story = {
	args: {
		placeholder: "Choose a model…",
	},
};

// The default trigger is a borderless pill; a callsite can override
// that via `className` to render a full bordered form field.
export const InputBorderTreatment: Story = {
	args: {
		value: "openai/gpt-4o-mini",
		className:
			"h-10 border border-border border-solid bg-transparent px-3 shadow-sm",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("combobox", { name: /gpt-4o mini/i });
		const styles = getComputedStyle(trigger);

		expect(styles.borderTopStyle).toBe("solid");
		expect(styles.borderTopWidth).not.toBe("0px");
		expect(styles.boxShadow).not.toBe("none");
	},
};

export const Disabled: Story = {
	args: {
		disabled: true,
		value: "openai/gpt-4o",
	},
};

// ---------------------------------------------------------------------------
// Open-by-default stories (visualize the dropdown without an interaction)
// ---------------------------------------------------------------------------

export const OpenSingleProvider: Story = {
	args: {
		options: openAIModels,
		value: "openai/gpt-4o",
		open: true,
	},
};

export const OpenMultipleProviders: Story = {
	args: {
		options: allModels,
		value: "openai/o3-mini",
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
// Play functions: interaction smoke tests
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

		// The popover is portaled, so the option renders in document.body.
		const option = await within(document.body).findByText("GPT-4o Mini");
		await userEvent.click(option);

		expect(args.onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	},
};

export const SearchFiltersOptions: Story = {
	args: {
		options: anthropicModels,
		value: "",
		open: true,
	},
	play: async () => {
		const body = within(document.body);
		const search = await body.findByPlaceholderText("Search...");
		await userEvent.type(search, "haiku");

		expect(await body.findByText("Claude 3.5 Haiku")).toBeInTheDocument();
		expect(body.queryByText("Claude Sonnet 4")).not.toBeInTheDocument();
	},
};
