import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { AgentSettingsBehaviorPageView } from "./AgentSettingsBehaviorPageView";

const mockDefaultSystemPrompt = "You are Coder, an AI coding assistant...";

// Baseline props shared across stories. Only primitives and simple
// objects here to avoid the composeStory deep-merge hang (see vault
// entry storybook-composestory-hang).
const baseProps = {
	canSetSystemPrompt: true as boolean,
	systemPromptData: {
		system_prompt: "",
		include_default_system_prompt: true,
		default_system_prompt: mockDefaultSystemPrompt,
	} as TypesGen.ChatSystemPromptResponse,
	planModeInstructionsData: {
		plan_mode_instructions: "",
	} as TypesGen.ChatPlanModeInstructionsResponse,
	userPromptData: { custom_prompt: "" } as TypesGen.UserChatCustomPrompt,
	desktopEnabledData: {
		enable_desktop: false,
	} as TypesGen.ChatDesktopEnabledResponse,
	workspaceTTLData: {
		workspace_ttl_ms: 0,
	} as TypesGen.ChatWorkspaceTTLResponse,
	isWorkspaceTTLLoading: false,
	isWorkspaceTTLLoadError: false,
	retentionDaysData: {
		retention_days: 30,
	} as TypesGen.ChatRetentionDaysResponse,
	isRetentionDaysLoading: false,
	isRetentionDaysLoadError: false,
	modelConfigsData: [] as TypesGen.ChatModelConfig[],
	modelConfigsError: undefined as unknown,
	isLoadingModelConfigs: false,
	thresholds: [] as readonly TypesGen.UserChatCompactionThreshold[],
	isThresholdsLoading: false,
	thresholdsError: undefined as unknown,
	isSavingSystemPrompt: false,
	isSaveSystemPromptError: false,
	isSavingPlanModeInstructions: false,
	isSavePlanModeInstructionsError: false,
	isSavingUserPrompt: false,
	isSaveUserPromptError: false,
	isSavingDesktopEnabled: false,
	isSaveDesktopEnabledError: false,
	isSavingWorkspaceTTL: false,
	isSaveWorkspaceTTLError: false,
	isSavingRetentionDays: false,
	isSaveRetentionDaysError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsBehaviorPageView",
	component: AgentSettingsBehaviorPageView,
	args: {
		...baseProps,
		onSaveSystemPrompt: fn(),
		onSavePlanModeInstructions: fn(),
		onSaveUserPrompt: fn(),
		onSaveDesktopEnabled: fn(),
		onSaveWorkspaceTTL: fn(),
		onSaveRetentionDays: fn(),
		onSaveThreshold: fn(),
		onResetThreshold: fn(),
	},
} satisfies Meta<typeof AgentSettingsBehaviorPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsBehaviorPageView>;

// ── Desktop ────────────────────────────────────────────────────

export const DesktopSetting: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Virtual Desktop");
		await canvas.findByText(
			/Allow agents to use a virtual, graphical desktop/i,
		);
		await canvas.findByRole("switch", { name: "Enable" });
	},
};

export const TogglesDesktop: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", { name: "Enable" });

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(args.onSaveDesktopEnabled).toHaveBeenCalledWith({
				enable_desktop: true,
			});
		});
	},
};

// ── System prompt ──────────────────────────────────────────────

export const AdminWithDefaultToggleOn: Story = {
	args: {
		systemPromptData: {
			system_prompt: "Always use TypeScript for code examples.",
			include_default_system_prompt: true,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		const toggle = await canvas.findByRole("switch", {
			name: "Include Coder Agents default system prompt",
		});
		expect(toggle).toBeChecked();
		expect(
			await canvas.findByDisplayValue(
				"Always use TypeScript for code examples.",
			),
		).toBeInTheDocument();
		expect(
			canvas.getByText(/built-in Coder Agents prompt is prepended/i),
		).toBeInTheDocument();

		// Preview dialog opens and closes.
		await userEvent.click(canvas.getByRole("button", { name: "Preview" }));
		expect(await body.findByText("Default System Prompt")).toBeInTheDocument();
		expect(body.getByText(mockDefaultSystemPrompt)).toBeInTheDocument();
		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(body.queryByText("Default System Prompt")).not.toBeInTheDocument();
		});

		// Toggle off include_default and save.
		await userEvent.click(toggle);
		const promptForm = canvas
			.getByDisplayValue("Always use TypeScript for code examples.")
			.closest("form")!;
		const saveButton = within(promptForm).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
	},
};

export const AdminWithDefaultToggleOff: Story = {
	args: {
		systemPromptData: {
			system_prompt: "You are a custom assistant.",
			include_default_system_prompt: false,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Include Coder Agents default system prompt",
		});
		expect(toggle).not.toBeChecked();
		expect(
			await canvas.findByDisplayValue("You are a custom assistant."),
		).toBeInTheDocument();
		expect(
			canvas.getByText(/only the additional instructions below are used/i),
		).toBeInTheDocument();
	},
};

// ── Autostop ───────────────────────────────────────────────────

export const DefaultAutostopDefault: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Workspace Autostop Fallback");
		await canvas.findByText(
			/set a default autostop for agent-created workspaces/i,
		);

		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).not.toBeChecked();
		expect(canvas.queryByLabelText("Autostop Fallback")).toBeNull();
	},
};

export const DefaultAutostopCustomValue: Story = {
	args: {
		workspaceTTLData: { workspace_ttl_ms: 7_200_000 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("2");
	},
};

export const DefaultAutostopSave: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// Toggle ON — fires immediate save with 1h default.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		await userEvent.click(toggle);

		await waitFor(() => {
			expect(args.onSaveWorkspaceTTL).toHaveBeenCalledWith(
				{ workspace_ttl_ms: 3_600_000 },
				expect.anything(),
			);
		});

		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("1");

		// Change to 3 hours.
		await userEvent.clear(durationInput);
		await userEvent.type(durationInput, "3");

		const ttlForm = durationInput.closest("form")!;
		const saveButton = within(ttlForm).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});

		// Clearing back to the original value hides Save (pristine form).
		await userEvent.clear(durationInput);
		await waitFor(() => {
			expect(
				within(ttlForm).queryByRole("button", { name: "Save" }),
			).toBeNull();
		});
	},
};

export const DefaultAutostopExceedsMax: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		await userEvent.click(toggle);

		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		const ttlForm = durationInput.closest("form")!;

		// 721 hours exceeds the 30-day / 720h limit.
		await userEvent.clear(durationInput);
		await userEvent.type(durationInput, "721");

		await waitFor(() => {
			expect(canvas.getByText(/must not exceed 30 days/i)).toBeInTheDocument();
		});

		const saveButton = within(ttlForm).getByRole("button", {
			name: "Save",
		});
		expect(saveButton).toBeDisabled();
	},
};

export const DefaultAutostopToggleOff: Story = {
	args: {
		workspaceTTLData: { workspace_ttl_ms: 7_200_000 },
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(args.onSaveWorkspaceTTL).toHaveBeenCalledWith(
				{ workspace_ttl_ms: 0 },
				expect.anything(),
			);
		});
	},
};

export const DefaultAutostopSaveDisabled: Story = {
	args: {
		workspaceTTLData: { workspace_ttl_ms: 7_200_000 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("2");

		const ttlForm = durationInput.closest("form")!;
		expect(within(ttlForm).queryByRole("button", { name: "Save" })).toBeNull();
	},
};

export const DefaultAutostopToggleFailure: Story = {
	args: {
		isSaveWorkspaceTTLError: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).not.toBeChecked();

		await userEvent.click(toggle);

		await waitFor(() => {
			expect(args.onSaveWorkspaceTTL).toHaveBeenCalledWith(
				{ workspace_ttl_ms: 3_600_000 },
				expect.anything(),
			);
		});

		// Error message should be visible.
		expect(
			canvas.getByText("Failed to save autostop setting."),
		).toBeInTheDocument();
	},
};

export const DefaultAutostopToggleOffFailure: Story = {
	args: {
		workspaceTTLData: { workspace_ttl_ms: 7_200_000 },
		isSaveWorkspaceTTLError: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("2");

		await userEvent.click(toggle);

		await waitFor(() => {
			expect(args.onSaveWorkspaceTTL).toHaveBeenCalledWith(
				{ workspace_ttl_ms: 0 },
				expect.anything(),
			);
		});

		// Error message should be visible.
		expect(
			canvas.getByText("Failed to save autostop setting."),
		).toBeInTheDocument();
	},
};

export const DefaultAutostopNotVisibleToNonAdmin: Story = {
	args: {
		canSetSystemPrompt: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Personal Instructions should be visible.
		await canvas.findByText("Personal Instructions");

		// Admin-only sections should not be present.
		expect(canvas.queryByText("Workspace Autostop Fallback")).toBeNull();
		expect(canvas.queryByText("Virtual Desktop")).toBeNull();
		expect(canvas.queryByText("System Instructions")).toBeNull();
	},
};

// ── Invisible Unicode warnings ─────────────────────────────────

export const InvisibleUnicodeWarningSystemPrompt: Story = {
	args: {
		systemPromptData: {
			system_prompt:
				"Normal prompt text\u200b\u200b\u200b\u200bhidden instruction",
			include_default_system_prompt: true,
			default_system_prompt: mockDefaultSystemPrompt,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("System Instructions");

		const alert = await canvas.findByText(/invisible Unicode/);
		expect(alert).toBeInTheDocument();
		expect(alert.textContent).toContain("4");
	},
};

export const InvisibleUnicodeWarningUserPrompt: Story = {
	args: {
		userPromptData: {
			custom_prompt: "My custom prompt\u200b\u200c\u200dhidden",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Personal Instructions");

		const alert = await canvas.findByText(/invisible Unicode/);
		expect(alert).toBeInTheDocument();
		expect(alert.textContent).toContain("2");
	},
};

export const InvisibleUnicodeWarningOnType: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const textarea = await canvas.findByPlaceholderText(
			"Additional behavior, style, and tone preferences",
		);

		// No warning initially.
		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();

		// Type a string containing a ZWS character.
		await userEvent.type(textarea, "hello\u200bworld");

		await waitFor(() => {
			expect(canvas.getByText(/invisible Unicode/)).toBeInTheDocument();
		});
	},
};

export const NoWarningForCleanPrompt: Story = {
	args: {
		systemPromptData: {
			system_prompt: "You are a helpful coding assistant.",
			include_default_system_prompt: true,
			default_system_prompt: mockDefaultSystemPrompt,
		},
		userPromptData: {
			custom_prompt: "Be concise and use TypeScript.",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Personal Instructions");
		await canvas.findByText("System Instructions");

		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();
	},
};
