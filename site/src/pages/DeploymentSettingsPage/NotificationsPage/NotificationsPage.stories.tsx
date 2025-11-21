import {
	MockCustomNotificationTemplates,
	MockNotificationMethodsResponse,
	MockSystemNotificationTemplates,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	customNotificationTemplatesKey,
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
} from "api/queries/notifications";
import { userEvent, within } from "storybook/test";
import NotificationsPage from "./NotificationsPage";
import { baseMeta } from "./storybookUtils";

const meta: Meta<typeof NotificationsPage> = {
	title: "pages/DeploymentSettingsPage/NotificationsPage",
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
				key: customNotificationTemplatesKey,
				data: undefined,
			},
			{
				key: notificationDispatchMethodsKey,
				data: MockNotificationMethodsResponse,
			},
		],
	},
};

export const LoadingDispatchMethods: Story = {
	parameters: {
		queries: [
			{
				key: systemNotificationTemplatesKey,
				data: MockSystemNotificationTemplates,
			},
			{
				key: customNotificationTemplatesKey,
				data: MockCustomNotificationTemplates,
			},
			{
				key: notificationDispatchMethodsKey,
				data: undefined,
			},
		],
	},
};

export const Events: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// System notification templates
		await canvas.findByText("Template Events");
		await canvas.findByText("User Events");
		await canvas.findByText("Workspace Events");
		await canvas.findByText("Task Events");

		// Custom notification template
		await canvas.findByText("Custom Events");
		await canvas.findByText("Custom Notification");
	},
};

export const Settings: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const settingsTab = await canvas.findByText("Settings");
		await user.click(settingsTab);
	},
};
