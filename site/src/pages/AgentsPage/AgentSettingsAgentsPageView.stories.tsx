import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import {
	AgentSettingsAgentsPageView,
	type AgentSettingsAgentsPageViewProps,
} from "./AgentSettingsAgentsPageView";

const baseArgs: AgentSettingsAgentsPageViewProps = {
	exploreModelOverrideData: {
		has_malformed_override: false,
	},
	modelConfigsData: [],
	modelConfigsError: undefined,
	isLoadingModelConfigs: false,
	onSaveExploreModelOverride: fn(),
	isSavingExploreModelOverride: false,
	isSaveExploreModelOverrideError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsAgentsPageView",
	component: AgentSettingsAgentsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsAgentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsAgentsPageView>;

export const ExploreModelOverrideSetting: Story = {
	args: {
		exploreModelOverrideData: {
			model_config_id: "model-explore-1",
			has_malformed_override: false,
		},
		modelConfigsData: [
			{
				id: "model-explore-1",
				provider: "openai",
				model: "gpt-4.1-mini",
				display_name: "GPT 4.1 Mini",
				enabled: true,
				is_default: false,
				context_limit: 1_000_000,
				compression_threshold: 70,
				created_at: "2026-03-12T12:00:00.000Z",
				updated_at: "2026-03-12T12:00:00.000Z",
			},
			{
				id: "model-explore-2",
				provider: "anthropic",
				model: "claude-sonnet-4",
				display_name: "Claude Sonnet 4",
				enabled: true,
				is_default: false,
				context_limit: 200_000,
				compression_threshold: 70,
				created_at: "2026-03-12T12:00:00.000Z",
				updated_at: "2026-03-12T12:00:00.000Z",
			},
		] as TypesGen.ChatModelConfig[],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Agents");
		await canvas.findByText("Explore subagent model");
		const trigger = canvas.getByRole("combobox", {
			name: /gpt 4.1 mini/i,
		});
		await userEvent.click(trigger);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", { name: "Claude Sonnet 4" }),
		);
		const form = trigger.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected Explore model selector to live inside a form.");
		}
		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveExploreModelOverride).toHaveBeenCalledWith(
				{ model_config_id: "model-explore-2" },
				expect.anything(),
			);
		});
	},
};

export const ExploreModelOverrideAllowsExplicitClear: Story = {
	args: {
		exploreModelOverrideData: {
			model_config_id: "model-explore-clear",
			has_malformed_override: false,
		},
		modelConfigsData: [
			{
				id: "model-explore-clear",
				provider: "openai",
				model: "gpt-4.1-mini",
				display_name: "GPT 4.1 Mini",
				enabled: true,
				is_default: false,
				context_limit: 1_000_000,
				compression_threshold: 70,
				created_at: "2026-03-12T12:00:00.000Z",
				updated_at: "2026-03-12T12:00:00.000Z",
			},
		] as TypesGen.ChatModelConfig[],
		onSaveExploreModelOverride: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const clearButton = await canvas.findByRole("button", { name: "Clear" });
		const form = clearButton.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error(
				"Expected Explore model clear button to live inside a form.",
			);
		}

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await userEvent.click(clearButton);
		expect(args.onSaveExploreModelOverride).not.toHaveBeenCalled();
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveExploreModelOverride).toHaveBeenCalledWith(
				{},
				expect.anything(),
			);
		});
	},
};

export const ExploreModelOverrideClearsMalformedSavedValue: Story = {
	args: {
		exploreModelOverrideData: {
			has_malformed_override: true,
		},
		modelConfigsData: [],
		onSaveExploreModelOverride: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(
			"The saved override is malformed and is being treated as unset. Click Save to clear it.",
		);
		const clearButton = await canvas.findByRole("button", { name: "Clear" });
		const form = clearButton.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error(
				"Expected Explore model clear button to live inside a form.",
			);
		}

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(clearButton);
		expect(args.onSaveExploreModelOverride).not.toHaveBeenCalled();
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(args.onSaveExploreModelOverride).toHaveBeenCalledWith(
				{},
				expect.anything(),
			);
		});
	},
};

export const ExploreModelOverrideFallsBackToModelName: Story = {
	args: {
		exploreModelOverrideData: {
			model_config_id: "model-explore-empty-name",
			has_malformed_override: false,
		},
		modelConfigsData: [
			{
				id: "model-explore-empty-name",
				provider: "anthropic",
				model: "claude-sonnet-4-20250514",
				display_name: "",
				enabled: true,
				is_default: false,
				context_limit: 200_000,
				compression_threshold: 70,
				created_at: "2026-03-12T12:00:00.000Z",
				updated_at: "2026-03-12T12:00:00.000Z",
			},
		] as TypesGen.ChatModelConfig[],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = await canvas.findByRole("combobox", {
			name: /claude-sonnet-4-20250514/i,
		});
		expect(trigger).toHaveTextContent("claude-sonnet-4-20250514");
		await userEvent.click(trigger);
		const body = within(canvasElement.ownerDocument.body);
		expect(
			await body.findByRole("option", {
				name: "claude-sonnet-4-20250514",
			}),
		).toBeInTheDocument();
	},
};
