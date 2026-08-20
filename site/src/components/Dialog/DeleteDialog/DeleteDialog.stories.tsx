import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { DeleteDialog } from "./DeleteDialog";

const meta: Meta<typeof DeleteDialog> = {
	title: "components/Dialog/DeleteDialog",
	component: DeleteDialog,
	args: {
		onCancel: fn(),
		onConfirm: fn(),
		isOpen: true,
		entity: "foo",
		name: "MyFoo",
		info: "Here's some info about the foo so you know you're deleting the right one.",
	},
};

export default meta;

type Story = StoryObj<typeof DeleteDialog>;

export const Idle: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await expect(body.getByRole("button", { name: "Delete" })).toBeDisabled();
	},
};

export const FilledSuccessfully: Story = {
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByLabelText("Name of the foo to delete");
		await user.type(input, "MyFoo");

		const confirmButton = body.getByRole("button", { name: "Delete" });
		await expect(confirmButton).toBeEnabled();
		await user.click(confirmButton);
		await expect(args.onConfirm).toHaveBeenCalled();
	},
};

export const FilledWrong: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByLabelText("Name of the foo to delete");
		await user.type(input, "InvalidFooName");
		// Blur so the mismatch error becomes visible.
		await user.tab();

		await expect(body.getByRole("button", { name: "Delete" })).toBeDisabled();
		await expect(
			body.getByText("InvalidFooName does not match the name of this foo"),
		).toBeVisible();
	},
};

export const Loading: Story = {
	args: {
		confirmLoading: true,
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		// Spinner prefixes the accessible name with "Loading spinner".
		await expect(body.getByRole("button", { name: /delete/i })).toBeDisabled();
		await expect(body.getByRole("button", { name: "Cancel" })).toBeDisabled();
	},
};
