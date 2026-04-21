import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	AgentSettingsLifecyclePageView,
	type AgentSettingsLifecyclePageViewProps,
} from "./AgentSettingsLifecyclePageView";

const baseArgs: AgentSettingsLifecyclePageViewProps = {
	workspaceTTLData: { workspace_ttl_ms: 0 },
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
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsLifecyclePageView",
	component: AgentSettingsLifecyclePageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsLifecyclePageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsLifecyclePageView>;

export const Default: Story = {};

export const DefaultAutostopDefault: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Workspace Autostop Fallback");
		await canvas.findByText(
			/Set a default autostop for agent-created workspaces/i,
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

		await userEvent.clear(durationInput);
		await userEvent.type(durationInput, "3");

		const ttlForm = durationInput.closest("form");
		if (!(ttlForm instanceof HTMLFormElement)) {
			throw new Error(
				"Expected autostop duration input to live inside a form.",
			);
		}
		const saveButton = within(ttlForm).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});

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
		const ttlForm = durationInput.closest("form");
		if (!(ttlForm instanceof HTMLFormElement)) {
			throw new Error(
				"Expected autostop duration input to live inside a form.",
			);
		}

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

		const ttlForm = durationInput.closest("form");
		if (!(ttlForm instanceof HTMLFormElement)) {
			throw new Error(
				"Expected autostop duration input to live inside a form.",
			);
		}
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
		expect(
			canvas.getByText("Failed to save autostop setting."),
		).toBeInTheDocument();
	},
};

// DefaultAutostopNotVisibleToNonAdmin is intentionally not ported because the
// split Lifecycle page is already gated by RequirePermission.
