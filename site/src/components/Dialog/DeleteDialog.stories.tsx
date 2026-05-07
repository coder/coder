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
		const confirmButton = await body.findByRole("button", { name: "Delete" });
		await expect(confirmButton).toBeDisabled();
	},
};

export const FilledSuccessfully: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByLabelText("Name of the foo to delete");
		await user.type(input, "MyFoo");
		const confirmButton = await body.findByRole("button", { name: "Delete" });
		await expect(confirmButton).not.toBeDisabled();
	},
};

export const FilledWrong: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByLabelText("Name of the foo to delete");
		await user.type(input, "InvalidFooName");
		const confirmButton = await body.findByRole("button", { name: "Delete" });
		await expect(confirmButton).toBeDisabled();
	},
};

export const Loading: Story = {
	args: {
		confirmLoading: true,
	},
};
