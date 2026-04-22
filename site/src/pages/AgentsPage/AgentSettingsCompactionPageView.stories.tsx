import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import {
	AgentSettingsCompactionPageView,
	type AgentSettingsCompactionPageViewProps,
} from "./AgentSettingsCompactionPageView";

const baseArgs: AgentSettingsCompactionPageViewProps = {
	modelConfigsData: [
		{
			id: "model-config-1",
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
	modelConfigsError: undefined,
	isLoadingModelConfigs: false,
	thresholds: [
		{
			model_config_id: "model-config-1",
			threshold_percent: 60,
		},
	],
	isThresholdsLoading: false,
	thresholdsError: undefined,
	onSaveThreshold: fn(async () => undefined),
	onResetThreshold: fn(async () => undefined),
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsCompactionPageView",
	component: AgentSettingsCompactionPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsCompactionPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsCompactionPageView>;

export const Default: Story = {};

export const SavesThreshold: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const thresholdInput = await canvas.findByLabelText(
			"GPT 4.1 Mini compaction threshold",
		);

		await userEvent.clear(thresholdInput);
		await userEvent.type(thresholdInput, "80");

		const saveButton = await canvas.findByRole("button", {
			name: "Save 1 change",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveThreshold).toHaveBeenCalledWith("model-config-1", 80);
		});
	},
};

export const ResetsThreshold: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const resetButton = await canvas.findByLabelText(
			"Reset GPT 4.1 Mini to default",
		);

		await waitFor(() => {
			expect(resetButton).toBeEnabled();
		});
		await userEvent.click(resetButton);

		await waitFor(() => {
			expect(args.onResetThreshold).toHaveBeenCalledWith("model-config-1");
		});
	},
};

export const InvalidThresholdIsRejected: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const thresholdInput = await canvas.findByLabelText(
			"GPT 4.1 Mini compaction threshold",
		);

		await userEvent.clear(thresholdInput);
		await userEvent.type(thresholdInput, "150");

		await waitFor(() => {
			expect(thresholdInput).toHaveAttribute("aria-invalid", "true");
			expect(
				canvas.queryByRole("button", { name: /save \d+ changes?/i }),
			).toBeNull();
		});
	},
};
