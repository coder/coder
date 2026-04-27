import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ConfirmDialog } from "./ConfirmDialog";

const meta: Meta<typeof ConfirmDialog> = {
	title: "components/Dialogs/ConfirmDialog",
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
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);

		expect(
			body.getByRole("heading", { name: "Confirm Dialog" }),
		).toBeInTheDocument();
		expect(
			body.getByText("Do you really want to delete me?"),
		).toBeInTheDocument();

		await user.click(body.getByRole("button", { name: "Cancel" }));
		expect(args.onClose).toHaveBeenCalled();

		await user.click(body.getByRole("button", { name: "Delete" }));
		expect(args.onConfirm).toHaveBeenCalled();
	},
};

export const InfoDialog: Story = {
	args: {
		description: "Information is cool!",
		hideCancel: true,
		type: "info",
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		expect(body.getByRole("button", { name: "OK" })).toBeInTheDocument();
		expect(
			body.queryByRole("button", { name: "Cancel" }),
		).not.toBeInTheDocument();
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
		hideCancel: false,
		type: "success",
		confirmLoading: true,
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		expect(body.getByRole("button", { name: "Cancel" })).toBeDisabled();
		expect(body.getByRole("button", { name: /OK/ })).toBeDisabled();
		expect(body.getByTitle("Loading spinner")).toBeInTheDocument();
	},
};
