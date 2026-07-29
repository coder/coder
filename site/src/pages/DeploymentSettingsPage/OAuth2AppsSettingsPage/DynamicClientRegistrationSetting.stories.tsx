import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { DynamicClientRegistrationSetting } from "./DynamicClientRegistrationSetting";

const meta: Meta<typeof DynamicClientRegistrationSetting> = {
	title: "pages/DeploymentSettingsPage/DynamicClientRegistrationSetting",
	component: DynamicClientRegistrationSetting,
	args: {
		enabled: false,
		canEdit: true,
		isUpdating: false,
		onChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof DynamicClientRegistrationSetting>;

export const Disabled: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeEnabled();
		await expect(canvas.queryByText("Enabled")).not.toBeInTheDocument();
	},
};

export const Enabled: Story = {
	args: {
		enabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByText("Enabled")).toBeVisible();
		await expect(canvas.getByRole("button", { name: "Disable" })).toBeVisible();
	},
};

export const ReadOnly: Story = {
	args: {
		canEdit: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeDisabled();
	},
};

export const EnabledReadOnly: Story = {
	args: {
		enabled: true,
		canEdit: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("button", { name: "Disable" }),
		).toBeDisabled();
	},
};

export const EnableShowsConfirmationDialog: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));

		const body = within(canvasElement.ownerDocument.body);
		await body.findByText("Enable Dynamic Client Registration?");
		await expect(args.onChange).not.toHaveBeenCalled();

		// The dialog's confirm button shares its accessible name with the trigger
		// button behind it, so scope the query to the dialog.
		const dialog = within(body.getByTestId("dialog"));
		await userEvent.click(dialog.getByRole("button", { name: "Enable" }));
		await expect(args.onChange).toHaveBeenCalledWith(true);
	},
};

export const CancelEnable: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Enable" }));

		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		await expect(args.onChange).not.toHaveBeenCalled();
	},
};

export const Updating: Story = {
	args: {
		isUpdating: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByRole("button", { name: "Enable" })).toBeDisabled();
	},
};

export const UpdatingWhileEnabled: Story = {
	args: {
		enabled: true,
		isUpdating: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("button", { name: "Disable" }),
		).toBeDisabled();
	},
};

// Disabling skips the confirmation dialog, unlike enabling.
export const Disable: Story = {
	args: {
		enabled: true,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Disable" }));

		await expect(args.onChange).toHaveBeenCalledWith(false);
	},
};
