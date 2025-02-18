import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import type { Response } from "api/typesGenerated";
import { MockProvisionerJob } from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import { CancelJobConfirmationDialog } from "./CancelJobConfirmationDialog";

const meta: Meta<typeof CancelJobConfirmationDialog> = {
	title:
		"pages/OrganizationSettingsPage/ProvisionersPage/CancelJobConfirmationDialog",
	component: CancelJobConfirmationDialog,
	args: {
		open: true,
		onClose: fn(),
		cancelProvisionerJob: fn(),
		job: {
			...MockProvisionerJob,
			status: "running",
		},
	},
};

export default meta;
type Story = StoryObj<typeof CancelJobConfirmationDialog>;

export const Idle: Story = {};

export const OnCancel: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const cancelButton = body.getByRole("button", { name: "Discard" });
		user.click(cancelButton);
		await waitFor(() => {
			expect(args.onClose).toHaveBeenCalledTimes(1);
		});
	},
};

export const onConfirmSuccess: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	decorators: [withGlobalSnackbar],
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const confirmButton = body.getByRole("button", { name: "Confirm" });

		user.click(confirmButton);
		await waitFor(() => {
			body.getByText("Provisioner job canceled successfully");
		});
		expect(args.cancelProvisionerJob).toHaveBeenCalledTimes(1);
		expect(args.cancelProvisionerJob).toHaveBeenCalledWith(args.job);
		expect(args.onClose).toHaveBeenCalledTimes(1);
	},
};

export const onConfirmFailure: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	decorators: [withGlobalSnackbar],
	args: {
		cancelProvisionerJob: fn(() => {
			throw new Error("API Error");
		}),
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const confirmButton = body.getByRole("button", { name: "Confirm" });

		user.click(confirmButton);
		await waitFor(() => {
			body.getByText("Failed to cancel provisioner job");
		});
		expect(args.cancelProvisionerJob).toHaveBeenCalledTimes(1);
		expect(args.cancelProvisionerJob).toHaveBeenCalledWith(args.job);
		expect(args.onClose).toHaveBeenCalledTimes(0);
	},
};

export const Confirming: Story = {
	args: {
		cancelProvisionerJob: fn(() => new Promise<Response>(() => {})),
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const confirmButton = body.getByRole("button", { name: "Confirm" });
		user.click(confirmButton);
	},
};
