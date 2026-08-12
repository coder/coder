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
	// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
	parameters: { pixel: { exclude: true } },
	args: {
		open: true,
		onClose: fn(),
		onCreate: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof CreateGatewayKeyDialog>;

export const Form: Story = {};

export const CreateAndReveal: Story = {
	render: (args) => {
		const [createdKey, setCreatedKey] = useState<
			CreateAIGatewayKeyResponse | undefined
		>(undefined);
		return (
			<CreateGatewayKeyDialog
				{...args}
				createdKey={createdKey}
				onCreate={(name) => {
					args.onCreate(name);
					setCreatedKey(MockCreateAIGatewayKeyResponse);
				}}
			/>
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
	},
};

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const nameField = await body.findByLabelText("Name");
		await userEvent.type(nameField, "UPPER_CASE");

		const createButton = await body.findByRole("button", { name: "Create" });
		await userEvent.click(createButton);

		await expect(
			body.findByText(
				"Use lowercase letters and numbers with optional single hyphens between words.",
			),
		).resolves.toBeVisible();
	},
};

export const CreateError: Story = {
	render: (args) => {
		const [submitError, setSubmitError] = useState<unknown>(undefined);
		const error = mockApiError({
			message: "Key name must be unique.",
			validations: [
				{
					field: "name",
					detail: "A key with this name already exists.",
				},
			],
		});
		return (
			<CreateGatewayKeyDialog
				{...args}
				submitError={submitError}
				onCreate={(name) => {
					args.onCreate(name);
					setSubmitError(error);
				}}
			/>
		);
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
	render: (args) => {
		const [submitError, setSubmitError] = useState<unknown>(undefined);
		const error = mockApiError({ message: "Failed to create key." });
		return (
			<CreateGatewayKeyDialog
				{...args}
				submitError={submitError}
				onCreate={(name) => {
					args.onCreate(name);
					setSubmitError(error);
				}}
			/>
		);
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

export const EscapeDoesNotDismissCreatedKey: Story = {
	render: (args) => {
		const [createdKey, setCreatedKey] = useState<
			CreateAIGatewayKeyResponse | undefined
		>(undefined);
		return (
			<CreateGatewayKeyDialog
				{...args}
				createdKey={createdKey}
				onCreate={(name) => {
					args.onCreate(name);
					setCreatedKey(MockCreateAIGatewayKeyResponse);
				}}
			/>
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

		await userEvent.keyboard("{Escape}");

		await expect(
			body.findByText(MockCreateAIGatewayKeyResponse.key),
		).resolves.toBeVisible();
	},
};

export const ReopenAfterCreate: Story = {
	render: (args) => {
		const [open, setOpen] = useState(args.open);
		const [createdKey, setCreatedKey] = useState<
			CreateAIGatewayKeyResponse | undefined
		>(undefined);
		return (
			<div>
				<button type="button" onClick={() => setOpen(true)}>
					Open dialog
				</button>
				<CreateGatewayKeyDialog
					{...args}
					open={open}
					createdKey={createdKey}
					onCreate={(name) => {
						args.onCreate(name);
						setCreatedKey(MockCreateAIGatewayKeyResponse);
					}}
					onClose={() => {
						args.onClose();
						setCreatedKey(undefined);
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
