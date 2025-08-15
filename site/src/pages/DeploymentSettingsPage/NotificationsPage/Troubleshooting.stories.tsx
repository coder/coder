import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { spyOn, userEvent, within } from "storybook/test";
import { baseMeta } from "./storybookUtils";
import { Troubleshooting } from "./Troubleshooting";

const meta: Meta<typeof Troubleshooting> = {
	title: "pages/DeploymentSettingsPage/NotificationsPage/Troubleshooting",
	component: Troubleshooting,
	...baseMeta,
};

export default meta;

type Story = StoryObj<typeof Troubleshooting>;

export const TestNotification: Story = {
	beforeEach() {
		spyOn(API, "postTestNotification").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		const sendButton = canvas.getByRole("button", {
			name: "Send notification",
		});
		await user.click(sendButton);
		await within(document.body).findByText("Test notification sent");
	},
};
