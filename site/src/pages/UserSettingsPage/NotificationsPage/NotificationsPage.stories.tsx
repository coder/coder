import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	customNotificationTemplatesKey,
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
	userNotificationPreferencesKey,
} from "#/api/queries/notifications";
import type { NotificationPreference } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockCustomNotificationTemplates,
	MockNotificationMethodsResponse,
	MockNotificationPreferences,
	MockSystemNotificationTemplates,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import NotificationsPage from "./NotificationsPage";

const meta = {
	title: "pages/UserSettingsPage/NotificationsPage",
	component: NotificationsPage,
	beforeEach: () => {
		const preferences = new Map(
			MockNotificationPreferences.map((pref) => [pref.id, pref]),
		);
		spyOn(API, "getUserNotificationPreferences").mockImplementation(async () =>
			Array.from(preferences.values()),
		);
		spyOn(API, "putUserNotificationPreferences").mockImplementation(
			async (_userId, { template_disabled_map }) => {
				for (const [id, disabled] of Object.entries(template_disabled_map)) {
					preferences.set(id, {
						id,
						disabled,
						updated_at: MockNotificationPreferences[0].updated_at,
					});
				}
				return Array.from(preferences.values());
			},
		);
	},
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
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
} satisfies Meta<typeof NotificationsPage>;

export default meta;
type Story = StoryObj<typeof NotificationsPage>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await Promise.all([
			// System notification templates
			canvas.findByRole("switch", { name: "Chat Events" }),
			canvas.findByRole("switch", { name: "Template Events" }),
			canvas.findByRole("switch", { name: "User Events" }),
			canvas.findByRole("switch", { name: "Workspace Events" }),

			// Custom notification template
			canvas.findByRole("switch", { name: "Custom Events" }),
		]);
	},
};

export const ToggleGroup: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const groupLabel = await canvas.findByLabelText("Workspace Events");
		await user.click(groupLabel);
	},
};

export const ToggleNotification: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const notificationLabel = await canvas.findByLabelText(
			"Workspace Marked as Dormant",
		);
		await user.click(notificationLabel);
	},
};

export const ToggleNotificationsWhileSaving: Story = {
	beforeEach: () => {
		const pendingSave = createDeferred<NotificationPreference[]>();
		spyOn(API, "putUserNotificationPreferences").mockReturnValue(
			pendingSave.promise,
		);
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await user.click(
			await canvas.findByRole("switch", { name: "Workspace Deleted" }),
		);
		await user.click(
			canvas.getByRole("switch", { name: "Workspace Marked as Dormant" }),
		);
	},
};

export const FailedSave: Story = {
	beforeEach: () => {
		spyOn(API, "putUserNotificationPreferences").mockRejectedValue(
			new Error("Unable to save preferences"),
		);
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await user.click(
			await canvas.findByRole("switch", { name: "Workspace Deleted" }),
		);
		await screen.findByText("Error updating notification preferences.");
		await canvas.findByRole("switch", {
			name: "Workspace Deleted",
			checked: true,
		});
	},
};

export const FailedRefetch: Story = {
	beforeEach: () => {
		spyOn(API, "getUserNotificationPreferences").mockRejectedValue(
			mockApiError({ message: "Unable to refresh notification preferences." }),
		);
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await user.click(
			await canvas.findByRole("switch", { name: "Workspace Deleted" }),
		);
		await canvas.findByRole("alert");
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
	play: async ({ canvasElement }) => {
		await screen.findByText("Notification has been disabled.");
		await within(canvasElement).findByRole("switch", {
			name: templateToDisable.name,
			checked: false,
		});
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
	beforeEach: () => {
		spyOn(API, "putUserNotificationPreferences").mockRejectedValue({});
	},
	play: async () => {
		await screen.findByText("Error disabling notification.");
	},
};
