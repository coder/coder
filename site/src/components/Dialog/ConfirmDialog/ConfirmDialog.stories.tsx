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
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		await expect(dialog).toBeInTheDocument();

		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await expect(args.onClose).toHaveBeenCalled();
	},
};

export const InfoDialog: Story = {
	args: {
		description: "Information is cool!",
		hideCancel: true,
		type: "info",
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			body.queryByRole("button", { name: "Cancel" }),
		).not.toBeInTheDocument();

		await userEvent.click(body.getByRole("button", { name: "OK" }));
		await expect(args.onConfirm).toHaveBeenCalled();
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
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		// Spinner prefixes the accessible name with "Loading spinner".
		await expect(body.getByRole("button", { name: /ok/i })).toBeDisabled();
	},
};

export const ConfirmAction: Story = {
	args: {
		description: "Do you really want to delete me?",
		hideCancel: false,
		type: "delete",
		confirmText: "CONFIRM",
		cancelText: "CANCEL",
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByRole("button", { name: "CONFIRM" }));
		await expect(args.onConfirm).toHaveBeenCalled();
		await expect(args.onClose).not.toHaveBeenCalled();
	},
};
