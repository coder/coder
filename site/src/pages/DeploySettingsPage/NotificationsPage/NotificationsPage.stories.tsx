import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { NotificationsPage } from "./NotificationsPage";
import { baseMeta } from "./storybookUtils";
import { notificationDispatchMethodsKey, systemNotificationTemplatesKey } from "api/queries/notifications";
import { MockNotificationMethodsResponse, MockNotificationTemplates } from "testHelpers/entities";

const meta: Meta<typeof NotificationsPage> = {
	title: "pages/DeploymentSettings/NotificationsPage",
	component: NotificationsPage,
	...baseMeta,
};

export default meta;

type Story = StoryObj<typeof NotificationsPage>;

export const LoadingTemplates: Story = {
	parameters: {
		queries: [
			{
				key: systemNotificationTemplatesKey,
				data: undefined,
			},
			{
				key: notificationDispatchMethodsKey,
				data: MockNotificationMethodsResponse,
			},
		]
	}
};

export const LoadingDispatchMethods: Story = {
	parameters: {
		queries: [
			{ key: systemNotificationTemplatesKey, data: MockNotificationTemplates },
			{
				key: notificationDispatchMethodsKey,
				data: undefined,
			},
		]
	}
};

export const Events: Story = {};

export const Settings: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const settingsTab = await canvas.findByText("Settings");
		await user.click(settingsTab);
	},
};
