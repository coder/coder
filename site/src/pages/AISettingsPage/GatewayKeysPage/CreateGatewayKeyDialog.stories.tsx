import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import type { CreateAIGatewayKeyResponse } from "#/api/typesGenerated";
import {
	MockCreateAIGatewayKeyResponse,
	mockApiError,
} from "#/testHelpers/entities";
import { CreateGatewayKeyDialog } from "./CreateGatewayKeyDialog";

const meta: Meta<typeof CreateGatewayKeyDialog> = {
	title: "pages/AISettingsPage/CreateGatewayKeyDialog",
	component: CreateGatewayKeyDialog,
	args: {
		open: true,
		onClose: fn(),
		onCreate: fn(
			(_name: string): Promise<CreateAIGatewayKeyResponse> =>
				Promise.resolve(MockCreateAIGatewayKeyResponse),
		),
	},
};

export default meta;
type Story = StoryObj<typeof CreateGatewayKeyDialog>;

export const Form: Story = {};

export const CreateAndReveal: Story = {
	play: async ({ args, canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const nameField = await body.findByLabelText("Name");
		await userEvent.type(nameField, "new-gateway");

		const createButton = await body.findByRole("button", { name: "Create" });
		await userEvent.click(createButton);

		await expect(args.onCreate).toHaveBeenCalledWith("new-gateway");
		await body.findByText(MockCreateAIGatewayKeyResponse.key);
	},
};

export const CreateError: Story = {
	args: {
		onCreate: fn(() =>
			Promise.reject(
				mockApiError({
					message: "Key name must be unique.",
					validations: [
						{
							field: "name",
							detail: "A key with this name already exists.",
						},
					],
				}),
			),
		),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const nameField = await body.findByLabelText("Name");
		await userEvent.type(nameField, "dup-key");

		const createButton = await body.findByRole("button", { name: "Create" });
		await userEvent.click(createButton);

		await expect(
			body.findByText("A key with this name already exists."),
		).resolves.toBeVisible();
	},
};

export const GeneralCreateError: Story = {
	args: {
		onCreate: fn(() =>
			Promise.reject(mockApiError({ message: "Failed to create key." })),
		),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const nameField = await body.findByLabelText("Name");
		await userEvent.type(nameField, "new-gateway");

		const createButton = await body.findByRole("button", { name: "Create" });
		await userEvent.click(createButton);

		await expect(
			body.findByText("Failed to create key."),
		).resolves.toBeVisible();
	},
};

export const ReopenAfterCreate: Story = {
	render: (args) => {
		const [open, setOpen] = useState(args.open);
		return (
			<div>
				<button type="button" onClick={() => setOpen(true)}>
					Open dialog
				</button>
				<CreateGatewayKeyDialog
					{...args}
					open={open}
					onClose={() => {
						args.onClose();
						setOpen(false);
					}}
				/>
			</div>
		);
	},
	play: async ({ args, canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const nameField = await body.findByLabelText("Name");
		await userEvent.type(nameField, "new-gateway");

		const createButton = await body.findByRole("button", { name: "Create" });
		await userEvent.click(createButton);

		await expect(args.onCreate).toHaveBeenCalledWith("new-gateway");
		await body.findByText(MockCreateAIGatewayKeyResponse.key);

		const doneButton = await body.findByRole("button", { name: "Done" });
		await userEvent.click(doneButton);

		const canvas = within(canvasElement);
		const openButton = await canvas.findByRole("button", {
			name: "Open dialog",
		});
		await userEvent.click(openButton);

		await expect(body.findByLabelText("Name")).resolves.toHaveValue("");
		await expect(
			body.queryByText(MockCreateAIGatewayKeyResponse.key),
		).not.toBeInTheDocument();
	},
};
