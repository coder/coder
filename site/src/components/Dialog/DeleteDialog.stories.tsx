import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { DeleteDialog } from "./DeleteDialog";
import dayjs from "dayjs";

const meta: Meta<typeof DeleteDialog> = {
	title: "components/Dialog/DeleteDialog",
	component: DeleteDialog,
	args: {
		open: true,
		description: (
			<>
				Deleting this workspace will permanently destroy all of its Terraform
				resources.
			</>
		),

		resourceKind: "workspace",
		resourceName: "coffee-tahr-38",

		onDelete: fn(),

		onCancel: fn(),
	},
};

export default meta;

type Story = StoryObj<typeof DeleteDialog>;

export const Example: Story = {};

export const WithTimeAndOwner: Story = {
	args: {
		resourceLastUsed: dayjs().subtract(5, "minutes"),
		resourceOwnedBy: "Pumpkaboo",
	},
};

export const Deleting: Story = {
	args: {
		deleteLoading: true,
	},
};

export const AdditionalInfo: Story = {
	args: {
		additionalInfo: "Oh hell yeah",
	},
};

export const WithNameInput: Story = {
	args: {
		requireAcknowledgingName: true,
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const confirmButton = await body.findByRole("button", { name: "Delete" });
		await expect(confirmButton).toBeDisabled();
		const input = await body.findByLabelText(
			`Confirm the name of the workspace`,
		);
		await user.type(input, "coffee-tahr-38");
		await expect(confirmButton).toBeEnabled();
	},
};

export const AdditionalInfoWithNameInput: Story = {
	args: {
		requireAcknowledgingName: true,
		additionalInfo: "Oh hell yeah",
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const confirmButton = await body.findByRole("button", { name: "Delete" });
		await expect(confirmButton).toBeDisabled();
		const input = await body.findByLabelText(
			`Confirm the name of the workspace`,
		);
		await user.type(input, "coffee-tahr-38");
		await expect(confirmButton).toBeEnabled();
	},
};

export const WithNameInputFilledWrong: Story = {
	args: {
		requireAcknowledgingName: true,
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const confirmButton = await body.findByRole("button", { name: "Delete" });
		await expect(confirmButton).toBeDisabled();
		const input = await body.findByLabelText(
			`Confirm the name of the workspace`,
		);
		await user.type(input, "tahr-coffee-83");
		await expect(confirmButton).toBeEnabled();
	},
};
