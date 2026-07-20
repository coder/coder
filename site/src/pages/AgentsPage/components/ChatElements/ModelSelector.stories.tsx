import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
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

const effortModel: ModelSelectorOption = {
	...MockModelSelectorOption,
	id: "openai/gpt-5",
	model: "gpt-5",
	displayName: "GPT-5",
	contextLimit: 400_000,
	reasoningEffortDefault: "medium",
	reasoningEfforts: [
		"none",
		"minimal",
		"low",
		"medium",
		"high",
		"xhigh",
		"max",
	],
};

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

export const CustomTriggerLabel: Story = {
	args: {
		options: openAIModels,
		value: "openai/gpt-4o",
		triggerAriaLabel: "Agent model behavior",
	},
	play: async ({ canvasElement }) => {
		expect(
			within(canvasElement).getByRole("combobox", {
				name: "Agent model behavior, GPT-4o",
			}),
		).toBeInTheDocument();
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
		onReasoningEffortChange: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const trigger = canvas.getByRole("combobox");
		await userEvent.click(trigger);

		const body = within(document.body);
		const listbox = await body.findByRole("listbox");
		const search = body.getByPlaceholderText("Search...");
		await userEvent.type(search, "mini");
		await userEvent.click(within(listbox).getByText("GPT-4o Mini"));

		expect(args.onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
		await waitFor(() => {
			expect(trigger).toHaveAttribute("aria-expanded", "false");
			expect(body.queryByRole("listbox")).not.toBeInTheDocument();
		});

		await userEvent.click(trigger);
		expect(await body.findByPlaceholderText("Search...")).toHaveValue("");
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

// ---------------------------------------------------------------------------
// Reasoning effort row
// ---------------------------------------------------------------------------

export const EffortRowHiddenWithoutConfig: Story = {
	args: {
		options: openAIModels,
		value: "openai/gpt-4o",
		reasoningEffort: "medium",
		onReasoningEffortChange: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(canvas.getByRole("combobox"));
		await body.findByRole("listbox");

		expect(body.queryByRole("slider")).not.toBeInTheDocument();
		expect(body.queryByText("Effort")).not.toBeInTheDocument();
	},
};

const EffortRowStory = ({
	onValueChange,
	onReasoningEffortChange,
}: {
	onValueChange: (value: string) => void;
	onReasoningEffortChange: (value: string) => void;
}) => {
	const [model, setModel] = useState("openai/gpt-4o");
	const [effort, setEffort] = useState("medium");
	return (
		<ModelSelector
			options={[...openAIModels, effortModel]}
			value={model}
			onValueChange={(value) => {
				onValueChange(value);
				setModel(value);
			}}
			reasoningEffort={effort}
			onReasoningEffortChange={(value) => {
				onReasoningEffortChange(value);
				setEffort(value);
			}}
		/>
	);
};

export const EffortRow: Story = {
	args: {
		onReasoningEffortChange: fn(),
	},
	render: (args) => (
		<EffortRowStory
			onValueChange={args.onValueChange}
			onReasoningEffortChange={(value) => args.onReasoningEffortChange?.(value)}
		/>
	),
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		const trigger = canvas.getByRole("combobox", { name: "GPT-4o" });
		await userEvent.click(trigger);
		const listbox = await body.findByRole("listbox");
		const search = body.getByPlaceholderText("Search...");
		await userEvent.type(search, "gpt-5");
		await userEvent.click(
			within(listbox).getByRole("option", { name: /GPT-5/ }),
		);

		expect(args.onValueChange).toHaveBeenCalledWith("openai/gpt-5");
		await waitFor(() => {
			expect(trigger).toHaveAttribute("aria-expanded", "true");
			expect(listbox).toBeVisible();
			expect(search).toHaveValue("");
			expect(body.getByText("Effort")).toBeVisible();
		});
		const slider = await body.findByRole("slider");
		expect(slider).toHaveAttribute("aria-valuemin", "0");
		expect(slider).toHaveAttribute("aria-valuemax", "6");
		// "medium" is the fourth of seven selectable efforts.
		expect(slider).toHaveAttribute("aria-valuenow", "3");
		expect(body.getByText("Medium")).toBeVisible();

		const infoTrigger = body.getByRole("button", {
			name: "About reasoning effort",
		});
		await userEvent.tab();
		expect(infoTrigger).toHaveFocus();
		expect(await screen.findByRole("tooltip")).toHaveTextContent(
			"Controls how much reasoning the model performs before responding.",
		);

		await userEvent.tab();
		expect(slider).toHaveFocus();

		await userEvent.keyboard("{ArrowRight}");
		await waitFor(() => {
			expect(slider).toHaveAttribute("aria-valuenow", "4");
		});
		expect(args.onReasoningEffortChange).toHaveBeenCalledWith("high");
		expect(body.getByText("High")).toBeVisible();

		await userEvent.keyboard("{ArrowRight}{ArrowRight}");
		await waitFor(() => {
			expect(slider).toHaveAttribute("aria-valuenow", "6");
		});
		expect(args.onReasoningEffortChange).toHaveBeenCalledWith("max");
		expect(body.getByText("Max")).toBeVisible();

		await userEvent.keyboard("{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}");
		await waitFor(() => {
			expect(slider).toHaveAttribute("aria-valuenow", "2");
		});
		expect(args.onReasoningEffortChange).toHaveBeenCalledWith("low");
		expect(body.getByText("Low")).toBeVisible();
	},
};

export const EffortRowClampedToMax: Story = {
	args: {
		options: [
			{
				...effortModel,
				reasoningEffortDefault: "low",
				reasoningEfforts: ["none", "minimal", "low", "medium"],
			},
		],
		value: "openai/gpt-5",
		reasoningEffort: "low",
		onReasoningEffortChange: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(canvas.getByRole("combobox"));
		await body.findByRole("listbox");

		// Selectable efforts stop at the configured max.
		const slider = await body.findByRole("slider");
		expect(slider).toHaveAttribute("aria-valuemax", "3");
		expect(slider).toHaveAttribute("aria-valuenow", "2");
		await waitFor(() => {
			expect(body.getByText("Low")).toBeVisible();
		});
	},
};
