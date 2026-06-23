import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	AgentSettingsLifecyclePageView,
	type AgentSettingsLifecyclePageViewProps,
} from "./AgentSettingsLifecyclePageView";

const baseArgs: AgentSettingsLifecyclePageViewProps = {
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
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsLifecyclePageView",
	component: AgentSettingsLifecyclePageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsLifecyclePageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsLifecyclePageView>;

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

export const InvalidRetentionDisablesSave: Story = {
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

		const saveButton = within(form).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(input).toBeInvalid();
			expect(saveButton).toBeDisabled();
		});
	},
};
