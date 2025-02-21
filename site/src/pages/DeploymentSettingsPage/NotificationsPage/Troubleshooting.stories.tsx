import type { Meta, StoryObj } from "@storybook/react";
import { spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import { Troubleshooting } from "./Troubleshooting";
import { baseMeta } from "./storybookUtils";

const meta: Meta<typeof Troubleshooting> = {
	title: "pages/DeploymentSettingsPage/NotificationsPage/Troubleshooting",
	component: Troubleshooting,
	...baseMeta,
};

export default meta;

type Story = StoryObj<typeof Troubleshooting>;

export const TestNotification: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "postTestNotification").mockResolvedValue();
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		const sendButton = canvas.getByRole("button", {
			name: "Send notification",
		});
		await user.click(sendButton);
		await within(document.body).findByText("Test notification sent");
	},
};
