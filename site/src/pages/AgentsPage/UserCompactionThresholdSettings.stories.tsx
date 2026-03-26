import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { UserCompactionThresholdSettings } from "./UserCompactionThresholdSettings";

const mockModelConfigs: TypesGen.ChatModelConfig[] = [
	{
		id: "model-1",
		provider: "openai",
		model: "gpt-4o",
		display_name: "GPT-4o",
		enabled: true,
		is_default: true,
		context_limit: 128000,
		compression_threshold: 80,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
	{
		id: "model-2",
		provider: "anthropic",
		model: "claude-sonnet",
		display_name: "Claude Sonnet",
		enabled: true,
		is_default: false,
		context_limit: 200000,
		compression_threshold: 70,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
	{
		id: "model-3",
		provider: "openai",
		model: "gpt-3.5",
		display_name: "GPT-3.5 (Disabled)",
		enabled: false,
		is_default: false,
		context_limit: 16000,
		compression_threshold: 60,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
];

const meta = {
	title: "pages/AgentsPage/UserCompactionThresholdSettings",
	component: UserCompactionThresholdSettings,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		modelConfigs: mockModelConfigs,
	},
	parameters: {
		user: MockUserOwner,
	},
} satisfies Meta<typeof UserCompactionThresholdSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getUserChatCompactionThresholds",
		).mockResolvedValue({
			thresholds: [],
		});
		spyOn(
			API.experimental,
			"updateUserChatCompactionThreshold",
		).mockResolvedValue({
			model_config_id: "model-1",
			threshold_percent: 90,
		});
		spyOn(
			API.experimental,
			"deleteUserChatCompactionThreshold",
		).mockResolvedValue(undefined);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});

		expect(canvas.getByText("GPT-4o")).toBeInTheDocument();
		expect(canvas.getByText("Claude Sonnet")).toBeInTheDocument();
		expect(canvas.queryByText("GPT-3.5 (Disabled)")).not.toBeInTheDocument();

		await userEvent.type(gpt4oInput, "100");
		expect(
			canvas.getByText(
				"⚠ Setting 100% will disable auto-compaction for this model.",
			),
		).toBeInTheDocument();
		await userEvent.clear(gpt4oInput);
		await userEvent.type(gpt4oInput, "95");

		const saveButtons = canvas.getAllByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButtons[0]).toBeEnabled();
		});

		await userEvent.click(saveButtons[0]);
		await waitFor(() => {
			expect(
				API.experimental.updateUserChatCompactionThreshold,
			).toHaveBeenCalledWith("model-1", { threshold_percent: 95 });
		});
	},
};

export const WithOverrides: Story = {
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getUserChatCompactionThresholds",
		).mockResolvedValue({
			thresholds: [
				{ model_config_id: "model-1", threshold_percent: 90 },
				{ model_config_id: "model-2", threshold_percent: 50 },
			],
		});
		spyOn(
			API.experimental,
			"updateUserChatCompactionThreshold",
		).mockResolvedValue({
			model_config_id: "model-1",
			threshold_percent: 90,
		});
		spyOn(
			API.experimental,
			"deleteUserChatCompactionThreshold",
		).mockResolvedValue(undefined);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});
		const claudeInput = await canvas.findByRole("spinbutton", {
			name: /Claude Sonnet compaction threshold/i,
		});

		expect(gpt4oInput).toHaveValue(90);
		expect(claudeInput).toHaveValue(50);

		const resetButtons = canvas.getAllByRole("button", { name: "Reset" });
		await userEvent.click(resetButtons[0]);
		await waitFor(() => {
			expect(
				API.experimental.deleteUserChatCompactionThreshold,
			).toHaveBeenCalledWith("model-1");
		});
	},
};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getUserChatCompactionThresholds").mockReturnValue(
			new Promise(() => {}),
		);
		spyOn(
			API.experimental,
			"updateUserChatCompactionThreshold",
		).mockResolvedValue({
			model_config_id: "model-1",
			threshold_percent: 90,
		});
		spyOn(
			API.experimental,
			"deleteUserChatCompactionThreshold",
		).mockResolvedValue(undefined);
	},
};

export const ErrorState: Story = {
	name: "Error",
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getUserChatCompactionThresholds",
		).mockRejectedValue(new globalThis.Error("Failed to load thresholds"));
		spyOn(
			API.experimental,
			"updateUserChatCompactionThreshold",
		).mockResolvedValue({
			model_config_id: "model-1",
			threshold_percent: 90,
		});
		spyOn(
			API.experimental,
			"deleteUserChatCompactionThreshold",
		).mockResolvedValue(undefined);
	},
};
