import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
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

// Minimal stub that satisfies UseMutationResult enough for stories.
// The actual mutation behavior is driven by the spied API calls.
const stubMutation = (overrides?: Record<string, unknown>) =>
	({
		mutate: () => {},
		mutateAsync: () => Promise.resolve(),
		reset: () => {},
		isPending: false,
		isIdle: true,
		isSuccess: false,
		isError: false,
		data: undefined,
		error: null,
		variables: undefined,
		status: "idle" as const,
		failureCount: 0,
		failureReason: null,
		context: undefined,
		isPaused: false,
		submittedAt: 0,
		...overrides,
		// biome-ignore lint/suspicious/noExplicitAny: story stubs
	}) as any;

const meta = {
	title: "pages/AgentsPage/UserCompactionThresholdSettings",
	component: UserCompactionThresholdSettings,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		modelConfigs: mockModelConfigs,
		thresholds: [],
		thresholdsError: null,
		isLoadingThresholds: false,
		updateThresholdMutation: stubMutation(),
		deleteThresholdMutation: stubMutation(),
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
	},
};

export const WithOverrides: Story = {
	args: {
		thresholds: [
			{ model_config_id: "model-1", threshold_percent: 90 },
			{ model_config_id: "model-2", threshold_percent: 50 },
		],
	},
	beforeEach: () => {
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
	},
};

export const Loading: Story = {
	args: {
		isLoadingThresholds: true,
	},
};

export const ErrorState: Story = {
	name: "Error",
	args: {
		thresholdsError: new globalThis.Error("Failed to load thresholds"),
	},
};
