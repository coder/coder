import type { Meta, StoryObj } from "@storybook/react";
import { spyOn, userEvent, waitFor, within } from "@storybook/test";
import { API } from "api/api";
import {
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
	userNotificationPreferencesKey,
} from "api/queries/notifications";
import { http, HttpResponse } from "msw";
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

export const DisableValidTemplate: Story = {
	parameters: {
		msw: {
			handlers: [
				http.put("/api/v2/users/:userId/notifications/preferences", () => {
					return HttpResponse.json([
						{ id: "valid-template-id", disabled: true },
					]);
				}),
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const validTemplateId = "valid-template-id";
		const validTemplateName = "Valid Template Name";

		window.history.pushState({}, "", `?disabled=${validTemplateId}`);

		await waitFor(
			async () => {
				const successMessage = await canvas.findByText(
					`${validTemplateName} notification has been disabled`,
				);
				expect(successMessage).toBeInTheDocument();
			},
			{ timeout: 10000 },
		);

		await waitFor(
			async () => {
				const templateSwitch = await canvas.findByLabelText(validTemplateName);
				expect(templateSwitch).not.toBeChecked();
			},
			{ timeout: 10000 },
		);
	},
};

export const DisableInvalidTemplate: Story = {
	parameters: {
		msw: {
			handlers: [
				http.put("/api/v2/users/:userId/notifications/preferences", () => {
					// Mock failed API response
					return new HttpResponse(null, { status: 400 });
				}),
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const invalidTemplateId = "invalid-template-id";

		window.history.pushState({}, "", `?disabled=${invalidTemplateId}`);

		await waitFor(
			async () => {
				const errorMessage = await canvas.findByText(
					"An error occurred when attempting to disable the requested notification",
				);
				expect(errorMessage).toBeInTheDocument();
			},
			{ timeout: 10000 },
		);
	},
};
