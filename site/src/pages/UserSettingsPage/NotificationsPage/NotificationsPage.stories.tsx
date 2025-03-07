import type { Meta, StoryObj } from "@storybook/react";
import { expect, spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import {
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
	userNotificationPreferencesKey,
} from "api/queries/notifications";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
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

const meta = {
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
		permissions: { viewDeploymentConfig: true },
	},
	decorators: [withGlobalSnackbar, withAuthProvider, withDashboardProvider],
} satisfies Meta<typeof NotificationsPage>;

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
		permissions: { viewDeploymentConfig: false },
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
const templateToDisable = MockNotificationTemplates.find(
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
