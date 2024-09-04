import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { NotificationsPage } from "./NotificationsPage";
import { baseMeta } from "./storybookUtils";

const meta: Meta<typeof NotificationsPage> = {
	title: "pages/DeploymentSettings/NotificationsPage",
	component: NotificationsPage,
	...baseMeta,
};

export default meta;

type Story = StoryObj<typeof NotificationsPage>;

export const Events: Story = {};

export const Settings: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const settingsTab = await canvas.findByText("Settings");
		await user.click(settingsTab);
	},
};
