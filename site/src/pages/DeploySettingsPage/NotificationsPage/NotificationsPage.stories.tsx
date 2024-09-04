import type { Meta, StoryObj } from "@storybook/react";
import { spyOn, userEvent, within } from "@storybook/test";
import { API } from "api/api";
import {
	notificationDispatchMethodsKey,
	systemNotificationTemplatesKey,
} from "api/queries/notifications";
import type { DeploymentValues, SerpentOption } from "api/typesGenerated";
import {
	MockNotificationMethodsResponse,
	MockNotificationTemplates,
	MockUser,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withDeploySettings,
	withGlobalSnackbar,
} from "testHelpers/storybook";
import { NotificationsPage } from "./NotificationsPage";

const meta: Meta<typeof NotificationsPage> = {
	title: "pages/DeploymentSettings/NotificationsPage",
	component: NotificationsPage,
	parameters: {
		experiments: ["notifications"],
		queries: [
			{ key: systemNotificationTemplatesKey, data: MockNotificationTemplates },
			{
				key: notificationDispatchMethodsKey,
				data: MockNotificationMethodsResponse,
			},
		],
		user: MockUser,
		permissions: { viewDeploymentValues: true },
		deploymentOptions: mockNotificationOptions(),
		deploymentValues: {
			notifications: {
				webhook: {
					endpoint: "https://example.com",
				},
				email: {
					smarthost: "smtp.example.com",
				},
			},
		} as DeploymentValues,
	},
	decorators: [
		withGlobalSnackbar,
		withAuthProvider,
		withDashboardProvider,
		withDeploySettings,
	],
};

export default meta;

type Story = StoryObj<typeof NotificationsPage>;

export const Default: Story = {};

export const NoEmailSmarthost: Story = {
	parameters: {
		deploymentValues: {
			notifications: {
				webhook: {
					endpoint: "https://example.com",
				},
				email: {
					smarthost: "",
				},
			},
		} as DeploymentValues,
	},
};

export const NoWebhookEndpoint: Story = {
	parameters: {
		deploymentValues: {
			notifications: {
				webhook: {
					endpoint: "",
				},
				email: {
					smarthost: "smtp.example.com",
				},
			},
		} as DeploymentValues,
	},
};

export const Toggle: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "updateNotificationTemplateMethod").mockResolvedValue();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const option = await canvas.findByText("Workspace Marked as Dormant");
		const li = option.closest("li");
		if(!li) {
			throw new Error("Could not find li");
		}
		const toggleButton = within(li).getByRole("button", {
			name: "Webhook",
		});
		await user.click(toggleButton);
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

function mockNotificationOptions(): SerpentOption[] {
	return [
		{
			name: "Notifications: Dispatch Timeout",
			description:
				"How long to wait while a notification is being sent before giving up.",
			flag: "notifications-dispatch-timeout",
			env: "CODER_NOTIFICATIONS_DISPATCH_TIMEOUT",
			yaml: "dispatchTimeout",
			default: "1m0s",
			value: 60000000000,
			annotations: {
				format_duration: "true",
			},
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			value_source: "default",
		},
		{
			name: "Notifications: Fetch Interval",
			description: "How often to query the database for queued notifications.",
			flag: "notifications-fetch-interval",
			env: "CODER_NOTIFICATIONS_FETCH_INTERVAL",
			yaml: "fetchInterval",
			default: "15s",
			value: 15000000000,
			annotations: {
				format_duration: "true",
			},
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			hidden: true,
			value_source: "default",
		},
		{
			name: "Notifications: Lease Count",
			description:
				"How many notifications a notifier should lease per fetch interval.",
			flag: "notifications-lease-count",
			env: "CODER_NOTIFICATIONS_LEASE_COUNT",
			yaml: "leaseCount",
			default: "20",
			value: 20,
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			hidden: true,
			value_source: "default",
		},
		{
			name: "Notifications: Lease Period",
			description:
				"How long a notifier should lease a message. This is effectively how long a notification is 'owned' by a notifier, and once this period expires it will be available for lease by another notifier. Leasing is important in order for multiple running notifiers to not pick the same messages to deliver concurrently. This lease period will only expire if a notifier shuts down ungracefully; a dispatch of the notification releases the lease.",
			flag: "notifications-lease-period",
			env: "CODER_NOTIFICATIONS_LEASE_PERIOD",
			yaml: "leasePeriod",
			default: "2m0s",
			value: 120000000000,
			annotations: {
				format_duration: "true",
			},
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			hidden: true,
			value_source: "default",
		},
		{
			name: "Notifications: Max Send Attempts",
			description: "The upper limit of attempts to send a notification.",
			flag: "notifications-max-send-attempts",
			env: "CODER_NOTIFICATIONS_MAX_SEND_ATTEMPTS",
			yaml: "maxSendAttempts",
			default: "5",
			value: 5,
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			value_source: "default",
		},
		{
			name: "Notifications: Method",
			description:
				"Which delivery method to use (available options: 'smtp', 'webhook').",
			flag: "notifications-method",
			env: "CODER_NOTIFICATIONS_METHOD",
			yaml: "method",
			default: "smtp",
			value: "smtp",
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			value_source: "env",
		},
		{
			name: "Notifications: Retry Interval",
			description: "The minimum time between retries.",
			flag: "notifications-retry-interval",
			env: "CODER_NOTIFICATIONS_RETRY_INTERVAL",
			yaml: "retryInterval",
			default: "5m0s",
			value: 300000000000,
			annotations: {
				format_duration: "true",
			},
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			hidden: true,
			value_source: "default",
		},
		{
			name: "Notifications: Store Sync Buffer Size",
			description:
				"The notifications system buffers message updates in memory to ease pressure on the database. This option controls how many updates are kept in memory. The lower this value the lower the change of state inconsistency in a non-graceful shutdown - but it also increases load on the database. It is recommended to keep this option at its default value.",
			flag: "notifications-store-sync-buffer-size",
			env: "CODER_NOTIFICATIONS_STORE_SYNC_BUFFER_SIZE",
			yaml: "storeSyncBufferSize",
			default: "50",
			value: 50,
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			hidden: true,
			value_source: "default",
		},
		{
			name: "Notifications: Store Sync Interval",
			description:
				"The notifications system buffers message updates in memory to ease pressure on the database. This option controls how often it synchronizes its state with the database. The shorter this value the lower the change of state inconsistency in a non-graceful shutdown - but it also increases load on the database. It is recommended to keep this option at its default value.",
			flag: "notifications-store-sync-interval",
			env: "CODER_NOTIFICATIONS_STORE_SYNC_INTERVAL",
			yaml: "storeSyncInterval",
			default: "2s",
			value: 2000000000,
			annotations: {
				format_duration: "true",
			},
			group: {
				name: "Notifications",
				yaml: "notifications",
				description: "Configure how notifications are processed and delivered.",
			},
			hidden: true,
			value_source: "default",
		},
	];
}
