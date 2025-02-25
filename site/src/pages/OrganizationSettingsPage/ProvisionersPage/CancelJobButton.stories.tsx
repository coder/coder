import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, waitFor, within } from "@storybook/test";
import { MockProvisionerJob } from "testHelpers/entities";
import { CancelJobButton } from "./CancelJobButton";

const meta: Meta<typeof CancelJobButton> = {
	title: "pages/OrganizationSettingsPage/ProvisionersPage/CancelJobButton",
	component: CancelJobButton,
	args: {
		job: {
			...MockProvisionerJob,
			status: "running",
		},
	},
};

export default meta;
type Story = StoryObj<typeof CancelJobButton>;

export const Cancellable: Story = {};

export const NotCancellable: Story = {
	args: {
		job: {
			...MockProvisionerJob,
			status: "succeeded",
		},
	},
};

export const OnClick: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await user.click(button);

		const body = within(canvasElement.ownerDocument.body);
		await waitFor(() => {
			body.getByText("Cancel provisioner job");
		});
	},
};
