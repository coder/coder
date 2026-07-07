import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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
		contextLimit: 1_000_000,
	},
];

const allModels: ModelSelectorOption[] = [...openAIModels, ...anthropicModels];

const meta: Meta<typeof ModelSelector> = {
	title: "pages/AgentsPage/ChatElements/ModelSelector",
	component: ModelSelector,
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

export const MultipleProviderInstances: Story = {
	args: {
		options: [
			...openAIModels,
			{
				...MockModelSelectorOption,
				id: "anthropic-primary/claude-sonnet-4",
				provider: "anthropic",
				providerId: "provider-anthropic-primary",
				providerLabel: "Anthropic",
				model: "claude-sonnet-4-20250514",
				displayName: "Claude Sonnet 4",
				contextLimit: 200_000,
			},
			{
				...MockModelSelectorOption,
				id: "anthropic-hyper/claude-opus-4",
				provider: "anthropic",
				providerId: "provider-anthropic-hyper",
				providerLabel: "Hyper",
				providerIcon: "/icon/coder.svg",
				model: "claude-opus-4-20250514",
				displayName: "Claude Opus 4",
				contextLimit: 200_000,
			},
		],
		value: "anthropic-primary/claude-sonnet-4",
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
// Play function, selection interaction
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

		const listbox = await within(document.body).findByRole("listbox");
		await userEvent.click(within(listbox).getByText("GPT-4o Mini"));

		expect(args.onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	},
};

export const FiltersModels: Story = {
	args: {
		options: allModels,
		value: "openai/gpt-4o",
		onValueChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const trigger = canvas.getByRole("combobox", { name: "GPT-4o" });

		const openListbox = async () => {
			await userEvent.click(trigger);
			return body.findByRole("listbox");
		};

		const searchFor = async (
			listbox: HTMLElement,
			query: string,
			expected: RegExp,
		) => {
			const input = body.getByPlaceholderText("Search...");
			await userEvent.clear(input);
			await userEvent.type(input, query);
			await waitFor(() => {
				expect(
					within(listbox).getByRole("option", { name: expected }),
				).toBeInTheDocument();
				expect(
					within(listbox).queryByRole("option", { name: /GPT-4o Mini/ }),
				).not.toBeInTheDocument();
			});
		};

		let listbox = await openListbox();
		await searchFor(listbox, "anthropic", /Claude Sonnet 4/);
		expect(
			within(listbox).getByRole("option", { name: /Claude 3.5 Haiku/ }),
		).toBeInTheDocument();

		await searchFor(listbox, "claude-3-5-haiku-20241022", /Claude 3.5 Haiku/);

		await searchFor(listbox, "1M", /Claude 3.5 Haiku/);

		await userEvent.click(trigger);
		await waitFor(() =>
			expect(body.queryByRole("listbox")).not.toBeInTheDocument(),
		);

		listbox = await openListbox();
		expect(
			within(listbox).getByRole("option", { name: /GPT-4o Mini/ }),
		).toBeInTheDocument();

		await userEvent.click(
			within(listbox).getByRole("option", { name: /Claude 3.5 Haiku/ }),
		);
		expect(args.onValueChange).toHaveBeenCalledWith(
			"anthropic/claude-haiku-3.5",
		);
	},
};
