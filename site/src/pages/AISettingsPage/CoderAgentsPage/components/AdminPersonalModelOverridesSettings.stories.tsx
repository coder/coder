import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { AdminPersonalModelOverridesSettings } from "./AdminPersonalModelOverridesSettings";

const baseArgs = {
	adminSettings: { allow_users: false },
	adminSettingsError: undefined,
	onRetryAdminSettings: fn(),
	isRetryingAdminSettings: false,
	onSaveAdminSetting: fn(),
	isSavingAdminSetting: false,
	isSaveAdminSettingError: false,
};

const meta = {
	title:
		"pages/AISettingsPage/CoderAgentsPage/components/AdminPersonalModelOverridesSettings",
	component: AdminPersonalModelOverridesSettings,
	args: baseArgs,
} satisfies Meta<typeof AdminPersonalModelOverridesSettings>;

export default meta;
type Story = StoryObj<typeof AdminPersonalModelOverridesSettings>;

export const FeatureDisabled: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});

		expect(
			await canvas.findByText("Allow personal model overrides"),
		).toBeInTheDocument();
		expect(toggle).not.toBeChecked();
		expect(
			canvas.queryByRole("button", { name: "Save" }),
		).not.toBeInTheDocument();
	},
};

export const LoadingState: Story = {
	args: {
		adminSettings: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			await canvas.findByText("Loading personal model override settings..."),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("switch", {
				name: "Allow personal model overrides",
			}),
		).toBeDisabled();
		expect(
			canvas.queryByRole("button", { name: "Save" }),
		).not.toBeInTheDocument();
	},
};

export const LoadError: Story = {
	args: {
		adminSettings: undefined,
		adminSettingsError: new Error("Failed to load personal model overrides."),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		expect(
			await canvas.findByText("Failed to load personal model overrides."),
		).toBeInTheDocument();
		expect(
			canvas.queryByText("Loading personal model override settings..."),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Save" }),
		).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.onRetryAdminSettings).toHaveBeenCalled();
	},
};

export const FeatureEnabled: Story = {
	args: {
		adminSettings: { allow_users: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});

		expect(toggle).toBeChecked();
		expect(
			canvas.queryByRole("button", { name: "Save" }),
		).not.toBeInTheDocument();
	},
};

export const Saving: Story = {
	args: {
		isSavingAdminSetting: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});

		expect(toggle).toBeDisabled();
		expect(
			canvas.queryByRole("button", { name: "Save" }),
		).not.toBeInTheDocument();
	},
};

export const SaveError: Story = {
	args: {
		isSaveAdminSettingError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			await canvas.findByText(
				"Failed to save personal model override settings.",
			),
		).toBeInTheDocument();
	},
};

export const SavesChangedSetting: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow personal model overrides",
		});
		await userEvent.click(toggle);

		await waitFor(() => {
			expect(args.onSaveAdminSetting).toHaveBeenCalledWith({
				allow_users: true,
			});
		});
		expect(
			canvas.queryByRole("button", { name: "Save" }),
		).not.toBeInTheDocument();
	},
};
