import type { Meta, StoryObj } from "@storybook/react";
import { spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import {
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
	userNotificationPreferencesKey,
} from "api/queries/notifications";
import {
	MockNotificationMethodsResponse,
	MockNotificationPreferences,
	MockNotificationTemplates,
	MockUser,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withGlobalSnackbar,
} from "testHelpers/storybook";
import { NotificationsPage } from "./NotificationsPage";

const meta: Meta<typeof NotificationsPage> = {
	title: "pages/UserSettingsPage/NotificationsPage",
	component: NotificationsPage,
	parameters: {
		experiments: ["notifications"],
		queries: [
			{
				key: userNotificationPreferencesKey(MockUser.id),
				data: MockNotificationPreferences,
			},
			{
				key: systemNotificationTemplatesKey,
				data: MockNotificationTemplates,
			},
			{
				key: notificationDispatchMethodsKey,
				data: MockNotificationMethodsResponse,
			},
		],
		user: MockUser,
		permissions: { viewDeploymentValues: true },
	},
	decorators: [withGlobalSnackbar, withAuthProvider, withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof NotificationsPage>;

export const Default: Story = {};

export const ToggleGroup: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "putUserNotificationPreferences").mockResolvedValue([]);
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const groupLabel = await canvas.findByLabelText("Workspace Events");
		await user.click(groupLabel);
	},
};

export const ToggleNotification: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "putUserNotificationPreferences").mockResolvedValue([]);
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const notificationLabel = await canvas.findByLabelText(
			"Workspace Marked as Dormant",
		);
		await user.click(notificationLabel);
	},
};

export const NonAdmin: Story = {
	parameters: {
		permissions: { viewDeploymentValues: false },
	},
};
