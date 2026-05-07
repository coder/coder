import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ConfirmDialog } from "./ConfirmDialog";

const meta: Meta<typeof ConfirmDialog> = {
	title: "components/Dialog/ConfirmDialog",
	component: ConfirmDialog,
	args: {
		onClose: fn(),
		onConfirm: fn(),
		open: true,
		title: "Confirm Dialog",
	},
};

export default meta;
type Story = StoryObj<typeof ConfirmDialog>;

export const Example: Story = {
	args: {
		description: "Do you really want to delete me?",
		hideCancel: false,
		type: "delete",
	},
};

export const InfoDialog: Story = {
	args: {
		description: "Information is cool!",
		hideCancel: true,
		type: "info",
	},
};

export const InfoDialogWithCancel: Story = {
	args: {
		description: "Information can be cool!",
		hideCancel: false,
		type: "info",
	},
};

export const SuccessDialog: Story = {
	args: {
		description: "I am successful.",
		hideCancel: true,
		type: "success",
	},
};

export const SuccessDialogWithCancel: Story = {
	args: {
		description: "I may be successful.",
		hideCancel: false,
		type: "success",
	},
};

export const SuccessDialogLoading: Story = {
	args: {
		description: "I am successful.",
		hideCancel: true,
		type: "success",
		confirmLoading: true,
	},
};

export const CallsOnCloseWhenCancelled: Story = {
	args: {
		cancelText: "CANCEL",
		hideCancel: false,
		title: "Test",
	},
	play: async ({ args, canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByRole("button", { name: "CANCEL" }));
		await expect(args.onClose).toHaveBeenCalledTimes(1);
		await expect(args.onConfirm).not.toHaveBeenCalled();
	},
};

export const CallsOnConfirmWhenConfirmed: Story = {
	args: {
		cancelText: "CANCEL",
		confirmText: "CONFIRM",
		hideCancel: false,
		title: "Test",
	},
	play: async ({ args, canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(await body.findByRole("button", { name: "CONFIRM" }));
		await expect(args.onConfirm).toHaveBeenCalledTimes(1);
		await expect(args.onClose).not.toHaveBeenCalled();
	},
};
