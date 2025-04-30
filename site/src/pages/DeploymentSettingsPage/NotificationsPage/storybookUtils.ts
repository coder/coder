import type { Meta } from "@storybook/react";
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
	withGlobalSnackbar,
	withOrganizationSettingsProvider,
	withTimeSyncProvider,
} from "testHelpers/storybook";
import type { NotificationsPage } from "./NotificationsPage";

// Extracted from a real API response
export const mockNotificationsDeploymentOptions: SerpentOption[] = [
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

export const baseMeta = {
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
		permissions: { viewDeploymentConfig: true },
		deploymentOptions: mockNotificationsDeploymentOptions,
		deploymentValues: {
			notifications: {
				webhook: {
					endpoint: "https://example.com",
				},
				email: {
					smarthost: "smtp.example.com",
					from: "bob@localhost",
					hello: "localhost",
				},
			},
		} as DeploymentValues,
	},
	decorators: [
		withGlobalSnackbar,
		withAuthProvider,
		withDashboardProvider,
		withOrganizationSettingsProvider,
		withTimeSyncProvider,
	],
} satisfies Meta<typeof NotificationsPage>;
