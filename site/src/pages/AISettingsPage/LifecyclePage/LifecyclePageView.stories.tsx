import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	LifecyclePageView,
	type LifecyclePageViewProps,
} from "./LifecyclePageView";

const baseArgs: LifecyclePageViewProps = {
	workspaceTTLData: { workspace_ttl_ms: 7_200_000 },
	isWorkspaceTTLLoading: false,
	isWorkspaceTTLLoadError: false,
	onSaveWorkspaceTTL: fn(),
	isSavingWorkspaceTTL: false,
	isSaveWorkspaceTTLError: false,
	retentionDaysData: { retention_days: 30 },
	isRetentionDaysLoading: false,
	isRetentionDaysLoadError: false,
	onSaveRetentionDays: fn(),
	isSavingRetentionDays: false,
	isSaveRetentionDaysError: false,
	debugRetentionDaysData: { debug_retention_days: 10 },
	isDebugRetentionDaysLoading: false,
	isDebugRetentionDaysLoadError: false,
	onSaveDebugRetentionDays: fn(),
	isSavingDebugRetentionDays: false,
	isSaveDebugRetentionDaysError: false,
	autoArchiveDaysData: { auto_archive_days: 45 },
	isAutoArchiveDaysLoading: false,
	isAutoArchiveDaysLoadError: false,
	onSaveAutoArchiveDays: fn(),
	isSavingAutoArchiveDays: false,
	isSaveAutoArchiveDaysError: false,
	debugLoggingData: undefined,
	isDebugLoggingLoading: false,
	onSaveDebugLogging: fn(),
	isSavingDebugLogging: false,
	isSaveDebugLoggingError: false,
};

const meta = {
	title: "pages/AISettingsPage/LifecyclePage/LifecyclePageView",
	component: LifecyclePageView,
	args: baseArgs,
} satisfies Meta<typeof LifecyclePageView>;

export default meta;
type Story = StoryObj<typeof LifecyclePageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("heading", { name: "Lifecycle" }),
		).toBeVisible();
		expect(
			canvas.getByText(
				"Control workspace lifecycle and conversation retention.",
			),
		).toBeVisible();
		expect(canvas.getByText("Workspace autostop fallback")).toBeVisible();
		expect(
			canvas.getByText("Auto-archive inactive conversations"),
		).toBeVisible();
		expect(canvas.getByText("Conversation retention period")).toBeVisible();
		expect(canvas.getByText("Chat debug data retention")).toBeVisible();
		expect(canvas.queryByRole("button", { name: "Save" })).toBeNull();
	},
};

export const DirtyAutostopInput: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText("Autostop fallback");
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected autostop input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "3");

		const saveButton = within(form).getByRole("button", { name: "Save" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveWorkspaceTTL).toHaveBeenCalledWith(
				{ workspace_ttl_ms: 10_800_000 },
				expect.anything(),
			);
		});
	},
};

export const DirtyAutostopToggle: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		const form = toggle.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected autostop toggle to live inside a form.");
		}

		await userEvent.click(toggle);
		expect(args.onSaveWorkspaceTTL).not.toHaveBeenCalled();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveWorkspaceTTL).toHaveBeenCalledWith(
				{ workspace_ttl_ms: 0 },
				expect.anything(),
			);
		});
	},
};

export const DirtyAutoArchiveToggle: Story = {
	args: {
		autoArchiveDaysData: { auto_archive_days: 0 },
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable auto-archive",
		});
		const form = toggle.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected auto-archive toggle to live inside a form.");
		}

		await userEvent.click(toggle);
		expect(args.onSaveAutoArchiveDays).not.toHaveBeenCalled();
		expect(
			await within(form).findByLabelText("Auto-archive period in days"),
		).toHaveValue(90);

		await userEvent.click(within(form).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(args.onSaveAutoArchiveDays).toHaveBeenCalledWith(
				{ auto_archive_days: 90 },
				expect.anything(),
			);
		});
	},
};

export const DirtyRetentionInput: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Conversation retention period in days",
		);
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected retention input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "60");
		await userEvent.click(within(form).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(args.onSaveRetentionDays).toHaveBeenCalledWith(
				{ retention_days: 60 },
				expect.anything(),
			);
		});
	},
};

export const DirtyDebugRetentionInput: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Chat debug data retention period in days",
		);
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected debug retention input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "14");
		await userEvent.click(within(form).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(args.onSaveDebugRetentionDays).toHaveBeenCalledWith(
				{ debug_retention_days: 14 },
				expect.anything(),
			);
		});
	},
};

export const InvalidRetentionMinDisablesSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Conversation retention period in days",
		);
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected retention input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "0");
		await userEvent.tab();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
			expect(
				canvas.getByText("Retention period must be at least 1 day."),
			).toBeInTheDocument();
		});
	},
};

export const InvalidRetentionMaxDisablesSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Conversation retention period in days",
		);
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected retention input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "9999");
		await userEvent.tab();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
			expect(
				canvas.getByText("Must not exceed 3650 days (~10 years)."),
			).toBeInTheDocument();
		});
	},
};

export const InvalidAutostopMaxDisablesSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText("Autostop fallback");
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected autostop input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "721");

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
			expect(canvas.getByText(/must not exceed 30 days/i)).toBeInTheDocument();
		});
	},
};

export const AutostopInvalidThenToggleOffSubmitsZero: Story = {
	args: {
		workspaceTTLData: { workspace_ttl_ms: 0 },
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText("Autostop fallback");
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected autostop input to live inside a form.");
		}

		const toggle = canvas.getByRole("switch", {
			name: "Enable default autostop",
		});
		await userEvent.click(toggle);
		await userEvent.clear(input);
		await userEvent.type(input, "721");

		await waitFor(() => {
			expect(input).toBeInvalid();
		});

		await userEvent.click(toggle);
		expect(input).toBeDisabled();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveWorkspaceTTL).toHaveBeenCalledWith(
				{ workspace_ttl_ms: 0 },
				expect.anything(),
			);
		});
	},
};

export const AutostopInputDisabledWhenToggleOff: Story = {
	args: {
		workspaceTTLData: { workspace_ttl_ms: 0 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText("Autostop fallback");
		expect(input).toBeDisabled();
	},
};

export const AutostopSaveError: Story = {
	args: {
		isSaveWorkspaceTTLError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to save autostop setting."),
		).toBeInTheDocument();
	},
};

export const AutostopLoadError: Story = {
	args: {
		workspaceTTLData: undefined,
		isWorkspaceTTLLoadError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to load autostop setting."),
		).toBeInTheDocument();
	},
};

export const AutoArchiveSaveError: Story = {
	args: {
		isSaveAutoArchiveDaysError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to save auto-archive setting."),
		).toBeInTheDocument();
	},
};

export const AutoArchiveLoadError: Story = {
	args: {
		autoArchiveDaysData: undefined,
		isAutoArchiveDaysLoadError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to load auto-archive setting."),
		).toBeInTheDocument();
	},
};

export const RetentionSaveError: Story = {
	args: {
		isSaveRetentionDaysError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to save conversation retention setting."),
		).toBeInTheDocument();
	},
};

export const RetentionLoadError: Story = {
	args: {
		retentionDaysData: undefined,
		isRetentionDaysLoadError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to load conversation retention setting."),
		).toBeInTheDocument();
	},
};

export const DebugRetentionSaveError: Story = {
	args: {
		isSaveDebugRetentionDaysError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to save chat debug retention setting."),
		).toBeInTheDocument();
	},
};

export const DebugRetentionLoadError: Story = {
	args: {
		debugRetentionDaysData: undefined,
		isDebugRetentionDaysLoadError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to load chat debug retention setting."),
		).toBeInTheDocument();
	},
};

export const DirtyAutoArchiveToggleOff: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable auto-archive",
		});
		const form = toggle.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected auto-archive toggle to live inside a form.");
		}

		await userEvent.click(toggle);
		expect(args.onSaveAutoArchiveDays).not.toHaveBeenCalled();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveAutoArchiveDays).toHaveBeenCalledWith(
				{ auto_archive_days: 0 },
				expect.anything(),
			);
		});
	},
};

export const DirtyRetentionToggleOff: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable conversation retention",
		});
		const form = toggle.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected retention toggle to live inside a form.");
		}

		await userEvent.click(toggle);
		expect(args.onSaveRetentionDays).not.toHaveBeenCalled();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveRetentionDays).toHaveBeenCalledWith(
				{ retention_days: 0 },
				expect.anything(),
			);
		});
	},
};

export const DirtyDebugRetentionToggleOff: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable chat debug data retention",
		});
		const form = toggle.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected debug retention toggle to live inside a form.");
		}

		await userEvent.click(toggle);
		expect(args.onSaveDebugRetentionDays).not.toHaveBeenCalled();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		expect(saveButton).toBeEnabled();
		await userEvent.click(saveButton);

		await waitFor(() => {
			expect(args.onSaveDebugRetentionDays).toHaveBeenCalledWith(
				{ debug_retention_days: 0 },
				expect.anything(),
			);
		});
	},
};

export const InvalidAutoArchiveMinDisablesSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText("Auto-archive period in days");
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected auto-archive input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "0");
		await userEvent.tab();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
			expect(
				canvas.getByText("Auto-archive period must be at least 1 day."),
			).toBeInTheDocument();
		});
	},
};

export const InvalidAutoArchiveMaxDisablesSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText("Auto-archive period in days");
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected auto-archive input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "9999");
		await userEvent.tab();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
			expect(
				canvas.getByText("Must not exceed 3650 days (~10 years)."),
			).toBeInTheDocument();
		});
	},
};

export const InvalidDebugRetentionMinDisablesSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Chat debug data retention period in days",
		);
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected debug retention input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "0");
		await userEvent.tab();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
			expect(
				canvas.getByText("Debug retention period must be at least 1 day."),
			).toBeInTheDocument();
		});
	},
};

export const InvalidDebugRetentionMaxDisablesSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Chat debug data retention period in days",
		);
		const form = input.closest("form");
		if (!(form instanceof HTMLFormElement)) {
			throw new Error("Expected debug retention input to live inside a form.");
		}

		await userEvent.clear(input);
		await userEvent.type(input, "9999");
		await userEvent.tab();

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
			expect(
				canvas.getByText("Must not exceed 3650 days (~10 years)."),
			).toBeInTheDocument();
		});
	},
};

export const AutoArchiveInputDisabledWhenToggleOff: Story = {
	args: {
		autoArchiveDaysData: { auto_archive_days: 0 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText("Auto-archive period in days");
		expect(input).toBeDisabled();
	},
};

export const RetentionInputDisabledWhenToggleOff: Story = {
	args: {
		retentionDaysData: { retention_days: 0 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Conversation retention period in days",
		);
		expect(input).toBeDisabled();
	},
};

export const DebugRetentionInputDisabledWhenToggleOff: Story = {
	args: {
		debugRetentionDaysData: { debug_retention_days: 0 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = await canvas.findByLabelText(
			"Chat debug data retention period in days",
		);
		expect(input).toBeDisabled();
	},
};

export const DirtyDebugLoggingToggle: Story = {
	args: {
		debugLoggingData: { allow_users: false, forced_by_deployment: false },
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});

		await userEvent.click(toggle);

		await waitFor(() => {
			expect(args.onSaveDebugLogging).toHaveBeenCalledWith({
				allow_users: true,
			});
		});
	},
};

export const DebugLoggingForcedByDeployment: Story = {
	args: {
		debugLoggingData: { allow_users: false, forced_by_deployment: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});
		expect(toggle).toBeDisabled();

		expect(
			canvas.getByText(
				"Debug logging is already enabled deployment-wide, so this per-user setting has no effect right now.",
			),
		).toBeInTheDocument();
	},
};

export const DebugLoggingSaveError: Story = {
	args: {
		debugLoggingData: { allow_users: false, forced_by_deployment: false },
		isSaveDebugLoggingError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText(
				"Failed to save the admin debug logging setting.",
			),
		).toBeInTheDocument();
	},
};
