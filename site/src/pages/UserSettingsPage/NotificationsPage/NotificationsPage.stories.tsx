import {
	MockCustomNotificationTemplates,
	MockNotificationMethodsResponse,
	MockNotificationPreferences,
	MockSystemNotificationTemplates,
	MockUserOwner,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withGlobalSnackbar,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import {
	customNotificationTemplatesKey,
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
	userNotificationPreferencesKey,
} from "api/queries/notifications";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import NotificationsPage from "./NotificationsPage";

const meta = {
	title: "pages/UserSettingsPage/NotificationsPage",
	component: NotificationsPage,
	parameters: {
		experiments: ["notifications"],
		queries: [
			{
				key: userNotificationPreferencesKey(MockUserOwner.id),
				data: MockNotificationPreferences,
			},
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
				data: MockNotificationMethodsResponse,
			},
		],
		user: MockUserOwner,
		permissions: { createTemplates: true, createUser: true },
	},
	decorators: [withGlobalSnackbar, withAuthProvider, withDashboardProvider],
} satisfies Meta<typeof NotificationsPage>;

export default meta;
type Story = StoryObj<typeof NotificationsPage>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await Promise.all([
			// System notification templates
			canvas.findByRole("checkbox", { name: "Task Events" }),
			canvas.findByRole("checkbox", { name: "Template Events" }),
			canvas.findByRole("checkbox", { name: "User Events" }),
			canvas.findByRole("checkbox", { name: "Workspace Events" }),

			// Custom notification template
			canvas.findByRole("checkbox", { name: "Custom Events" }),
		]);
	},
};

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
		permissions: { createTemplates: false, createUser: false },
	},
};

export const TemplateAdmin: Story = {
	parameters: {
		permissions: { createTemplates: true, createUser: false },
	},
};

export const UserAdmin: Story = {
	parameters: {
		permissions: { createTemplates: false, createUser: true },
	},
};

// Ensure the selected notification template is enabled before attempting to
// disable it.
const enabledPreference = MockNotificationPreferences.find(
	(pref) => pref.disabled === false,
);
if (!enabledPreference) {
	throw new Error(
		"No enabled notification preference available to test the disabling action.",
	);
}
const templateToDisable = MockSystemNotificationTemplates.find(
	(tpl) => tpl.id === enabledPreference.id,
);
if (!templateToDisable) {
	throw new Error("	No notification template matches the enabled preference.");
}

export const DisableValidTemplate: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				searchParams: { disabled: templateToDisable.id },
			},
		}),
	},
	decorators: [
		(Story) => {
			// Since the action occurs during the initial render, we need to spy on
			// the API call before the story is rendered. This is done using a
			// decorator to ensure the spy is set up in time.
			spyOn(API, "putUserNotificationPreferences").mockResolvedValue(
				MockNotificationPreferences.map((pref) => {
					if (pref.id === templateToDisable.id) {
						return {
							...pref,
							disabled: true,
						};
					}
					return pref;
				}),
			);
			return <Story />;
		},
	],
	play: async ({ canvasElement }) => {
		await within(document.body).findByText("Notification has been disabled");
		const switchEl = await within(canvasElement).findByLabelText(
			templateToDisable.name,
		);
		expect(switchEl).not.toBeChecked();
	},
};

export const DisableInvalidTemplate: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				searchParams: { disabled: "invalid-template-id" },
			},
		}),
	},
	decorators: [
		(Story) => {
			// Since the action occurs during the initial render, we need to spy on
			// the API call before the story is rendered. This is done using a
			// decorator to ensure the spy is set up in time.
			spyOn(API, "putUserNotificationPreferences").mockRejectedValue({});
			return <Story />;
		},
	],
	play: async () => {
		await within(document.body).findByText("Error disabling notification");
	},
};

export const EnablingTaskNotificationClearsAlertDismissal: Story = {
	parameters: {
		queries: [
			{
				// User notification preferences: empty because user hasn't changed defaults
				// Task notifications are disabled by default (enabled_by_default: false)
				key: ["users", MockUserOwner.id, "notifications", "preferences"],
				data: [],
			},
			{
				// System notification templates: includes task notifications with enabled_by_default: false
				key: ["notifications", "templates", "system"],
				data: MockSystemNotificationTemplates,
			},
			{
				key: customNotificationTemplatesKey,
				data: MockCustomNotificationTemplates,
			},
			{
				key: notificationDispatchMethodsKey,
				data: MockNotificationMethodsResponse,
			},
			{
				// User preferences: alert was previously dismissed
				key: ["me", "preferences"],
				data: { task_notification_alert_dismissed: true },
			},
		],
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		// Mock the API call to update notification preferences
		spyOn(API, "putUserNotificationPreferences").mockResolvedValue([
			{
				id: "d4a6271c-cced-4ed0-84ad-afd02a9c7799", // Task Idle
				disabled: false,
				updated_at: new Date().toISOString(),
			},
		]);

		// Mock the user preferences update to verify the alert dismissal is cleared
		const updatePreferencesSpy = spyOn(
			API,
			"updateUserPreferenceSettings",
		).mockResolvedValue({
			task_notification_alert_dismissed: false,
		});

		await step("Enable Task Idle notification", async () => {
			// Find the Task Idle checkbox by its label text
			const taskIdleToggle = canvas.getByLabelText("Task Idle");

			// Click to enable it
			await userEvent.click(taskIdleToggle);

			// Verify the preferences API was called to clear the alert dismissal
			await waitFor(() => {
				expect(updatePreferencesSpy).toHaveBeenCalledWith({
					task_notification_alert_dismissed: false,
				});
			});
		});
	},
};
