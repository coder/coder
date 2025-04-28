import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { userEvent } from "@storybook/test";
import { within } from "@testing-library/react";
import { DeleteDialog } from "./DeleteDialog";

const meta: Meta<typeof DeleteDialog> = {
	title: "components/Dialogs/DeleteDialog",
	component: DeleteDialog,
	args: {
		onCancel: action("onClose"),
		onConfirm: action("onConfirm"),
		isOpen: true,
		entity: "foo",
		name: "MyFoo",
		info: "Here's some info about the foo so you know you're deleting the right one.",
	},
};

export default meta;

type Story = StoryObj<typeof DeleteDialog>;

export const Idle: Story = {};

export const FilledSuccessfully: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByLabelText("Name of the foo to delete");
		await user.type(input, "MyFoo");
	},
};

export const FilledWrong: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const input = await body.findByLabelText("Name of the foo to delete");
		await user.type(input, "InvalidFooName");
	},
};

export const Loading: Story = {
	args: {
		confirmLoading: true,
	},
};
