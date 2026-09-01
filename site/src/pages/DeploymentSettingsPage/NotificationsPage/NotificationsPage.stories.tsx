import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
	customNotificationTemplatesKey,
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
} from "#/api/queries/notifications";
import {
	MockCustomNotificationTemplates,
	MockNotificationMethodsResponse,
	MockSystemNotificationTemplates,
} from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
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
		const docsLinks = canvas.getAllByRole("link", { name: /View docs/ });
		await expect(docsLinks).toHaveLength(3);
		await expect(docsLinks[0]).toHaveAttribute(
			"href",
			docs("/admin/monitoring/notifications"),
		);
		await expect(docsLinks[1]).toHaveAttribute(
			"href",
			docs("/admin/monitoring/notifications#webhook"),
		);
		await expect(docsLinks[2]).toHaveAttribute(
			"href",
			docs("/admin/monitoring/notifications#smtp-email"),
		);

		// System notification templates
		await canvas.findByText("Template Events");
		await canvas.findByText("User Events");
		await canvas.findByText("Workspace Events");

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
