import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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

const meta = {
	title: "pages/AgentsPage/UserCompactionThresholdSettings",
	component: UserCompactionThresholdSettings,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		modelConfigs: mockModelConfigs,
		thresholds: [],
		isThresholdsLoading: false,
		thresholdsError: undefined,
		onSaveThreshold: fn(async () => undefined),
		onResetThreshold: fn(async () => undefined),
	},
	parameters: {
		user: MockUserOwner,
	},
} satisfies Meta<typeof UserCompactionThresholdSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});

		expect(canvas.getByText("GPT-4o")).toBeInTheDocument();
		expect(canvas.getByText("Claude Sonnet")).toBeInTheDocument();
		expect(canvas.queryByText("GPT-3.5 (Disabled)")).not.toBeInTheDocument();

		// No footer visible when nothing is dirty
		expect(
			canvas.queryByRole("button", { name: /Save/i }),
		).not.toBeInTheDocument();

		// Type a value to make the footer appear
		await userEvent.type(gpt4oInput, "95");
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: /Save 1 change/i }),
			).toBeInTheDocument();
		});
	},
};

export const SaveAll: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});
		const claudeInput = await canvas.findByRole("spinbutton", {
			name: /Claude Sonnet compaction threshold/i,
		});

		// Edit both models
		await userEvent.type(gpt4oInput, "95");
		await userEvent.type(claudeInput, "50");

		// Footer should show "Save 2 changes"
		const saveButton = await canvas.findByRole("button", {
			name: /Save 2 changes/i,
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveThreshold).toHaveBeenCalledWith("model-1", 95);
			expect(args.onSaveThreshold).toHaveBeenCalledWith("model-2", 50);
		});
	},
};

export const WithOverrides: Story = {
	args: {
		thresholds: [
			{ model_config_id: "model-1", threshold_percent: 90 },
			{ model_config_id: "model-2", threshold_percent: 50 },
		],
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});
		const claudeInput = await canvas.findByRole("spinbutton", {
			name: /Claude Sonnet compaction threshold/i,
		});

		expect(gpt4oInput).toHaveValue(90);
		expect(claudeInput).toHaveValue(50);

		// Reset buttons should be visible for both overridden models
		const resetButtons = canvas.getAllByRole("button", {
			name: /Reset .+ to default/i,
		});
		expect(resetButtons).toHaveLength(2);

		await userEvent.click(resetButtons[0]);
		await waitFor(() => {
			expect(args.onResetThreshold).toHaveBeenCalledWith("model-1");
		});
	},
};

export const CancelChanges: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});

		await userEvent.type(gpt4oInput, "42");
		const cancelButton = await canvas.findByRole("button", { name: /Cancel/i });
		await userEvent.click(cancelButton);

		// Footer should disappear after cancel
		await waitFor(() => {
			expect(
				canvas.queryByRole("button", { name: /Save/i }),
			).not.toBeInTheDocument();
		});

		// Input should be cleared back to empty (no override)
		expect(gpt4oInput).toHaveValue(null);
	},
};

export const InvalidDraftShowsFooter: Story = {
	name: "Invalid Draft Shows Footer",
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});

		// Type an out-of-range value (number inputs reject non-numeric chars)
		await userEvent.type(gpt4oInput, "150");

		// Input should be marked invalid
		await waitFor(() => {
			expect(gpt4oInput).toHaveAttribute("aria-invalid", "true");
		});

		// Cancel button should be visible so user can discard the edit
		expect(canvas.getByRole("button", { name: /Cancel/i })).toBeInTheDocument();

		// Save button should NOT be visible (nothing valid to save)
		expect(
			canvas.queryByRole("button", { name: /Save/i }),
		).not.toBeInTheDocument();
	},
};

export const DisableCompactionWarning: Story = {
	name: "100% Disable Compaction Warning",
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});

		await userEvent.type(gpt4oInput, "100");

		// sr-only warning should be in the DOM for screen readers
		await waitFor(() => {
			expect(
				canvas.getByText(
					"Setting 100% will disable auto-compaction for this model.",
				),
			).toBeInTheDocument();
		});
	},
};

export const Loading: Story = {
	args: {
		isThresholdsLoading: true,
	},
};

export const PartialSaveFailure: Story = {
	name: "Partial Save Failure",
	args: {
		onSaveThreshold: fn(async (modelConfigId: string) => {
			if (modelConfigId === "model-2") {
				throw new globalThis.Error("Network error");
			}
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("spinbutton", {
			name: /GPT-4o compaction threshold/i,
		});
		const claudeInput = await canvas.findByRole("spinbutton", {
			name: /Claude Sonnet compaction threshold/i,
		});

		await userEvent.type(gpt4oInput, "90");
		await userEvent.type(claudeInput, "55");

		const saveButton = await canvas.findByRole("button", {
			name: /Save 2 changes/i,
		});
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveThreshold).toHaveBeenCalledWith("model-1", 90);
			expect(args.onSaveThreshold).toHaveBeenCalledWith("model-2", 55);
		});

		// model-2 should show an error, footer should still be visible
		// with Save showing "Save 1 change" for the failed row
		await waitFor(() => {
			expect(canvas.getByText("Network error")).toBeInTheDocument();
			expect(
				canvas.getByRole("button", { name: /Save 1 change/i }),
			).toBeInTheDocument();
		});
	},
};

export const ErrorState: Story = {
	name: "Error",
	args: {
		thresholdsError: new globalThis.Error("Failed to load thresholds"),
	},
};
