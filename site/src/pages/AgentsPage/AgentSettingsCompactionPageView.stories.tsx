import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import { userCompactionStrategyKey } from "#/api/queries/chats";
import { MockChatModel } from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import {
	AgentSettingsCompactionPageView,
	type AgentSettingsCompactionPageViewProps,
} from "./AgentSettingsCompactionPageView";

const baseArgs: AgentSettingsCompactionPageViewProps = {
	models: [
		{
			...MockChatModel,
			id: "model-config-1",
			organization_id: MockDefaultOrganization.id,
			ai_provider_id: "prov-openai",
			model: "gpt-4.1-mini",
			display_name: "GPT 4.1 Mini",
			is_default: false,
			context_limit: 1_000_000,
		},
	],
	providerTypeByID: new Map<string, string>([["prov-openai", "openai"]]),
	organizations: [MockDefaultOrganization],
	compactionTriggersByOrganizationID: new Map(),
	compactionTriggerLoadErrors: [],
	modelsError: undefined,
	isLoadingModels: false,
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
	compactionStrategy: "summarize",
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsCompactionPageView",
	component: AgentSettingsCompactionPageView,
	decorators: [withDashboardProvider],
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsCompactionPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsCompactionPageView>;

export const Default: Story = {};

export const SavesThreshold: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const thresholdInput = await canvas.findByLabelText(
			`GPT 4.1 Mini compaction threshold for ${MockDefaultOrganization.display_name}`,
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
			`Reset GPT 4.1 Mini for ${MockDefaultOrganization.display_name} to default`,
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
			`GPT 4.1 Mini compaction threshold for ${MockDefaultOrganization.display_name}`,
		);

		await userEvent.clear(thresholdInput);
		await userEvent.type(thresholdInput, "150");

		// Wait for validation to run so the snapshot captures the rejected
		// state.
		await waitFor(() => {
			expect(thresholdInput).toHaveAttribute("aria-invalid", "true");
		});
	},
};

export const SummarizeStrategy: Story = {
	parameters: {
		experiments: ["chat-token-budget"],
		queries: [
			{ key: userCompactionStrategyKey, data: { strategy: "summarize" } },
		],
	},
};

export const CompactionStrategyOptions: Story = {
	parameters: {
		experiments: ["chat-token-budget"],
		queries: [
			{ key: userCompactionStrategyKey, data: { strategy: "summarize" } },
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", {
				name: "Compaction strategy",
			}),
		);
	},
};

export const CompactionStrategyLoadError: Story = {
	args: { compactionStrategy: undefined },
	parameters: { experiments: ["chat-token-budget"] },
	beforeEach: () => {
		spyOn(API.experimental, "getUserChatCompactionStrategy").mockRejectedValue(
			new Error("Failed to load the compaction strategy."),
		);
	},
};

export const CompactionStrategySaveError: Story = {
	parameters: {
		experiments: ["chat-token-budget"],
		queries: [
			{ key: userCompactionStrategyKey, data: { strategy: "summarize" } },
		],
	},
	beforeEach: () => {
		spyOn(
			API.experimental,
			"updateUserChatCompactionStrategy",
		).mockRejectedValue(new Error("Failed to save the compaction strategy."));
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", {
				name: "Compaction strategy",
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Token budget" }),
		);
	},
};

export const CompactionStrategyLoading: Story = {
	args: { compactionStrategy: undefined },
	parameters: { experiments: ["chat-token-budget"] },
	beforeEach: () => {
		spyOn(API.experimental, "getUserChatCompactionStrategy").mockReturnValue(
			new Promise(() => undefined),
		);
	},
};

export const CompactionStrategySaving: Story = {
	parameters: {
		experiments: ["chat-token-budget"],
		queries: [
			{ key: userCompactionStrategyKey, data: { strategy: "summarize" } },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "updateUserChatCompactionStrategy").mockReturnValue(
			new Promise(() => undefined),
		);
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", {
				name: "Compaction strategy",
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Token budget" }),
		);
	},
};
