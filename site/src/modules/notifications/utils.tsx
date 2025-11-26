import type {
	NotificationPreference,
	NotificationTemplate,
} from "api/typesGenerated";
import { MailIcon, WebhookIcon } from "lucide-react";

// TODO: This should be provided by the auto generated types from codersdk
const notificationMethods = ["smtp", "webhook"] as const;

// localStorage key for tracking whether the user has dismissed the
// task notifications warning alert on the Tasks page
export const TasksNotificationAlertDismissedKey =
	"tasksNotificationAlertDismissed";

export type NotificationMethod = (typeof notificationMethods)[number];

export const methodIcons: Record<NotificationMethod, typeof MailIcon> = {
	smtp: MailIcon,
	webhook: WebhookIcon,
};

export const methodLabels: Record<NotificationMethod, string> = {
	smtp: "SMTP",
	webhook: "Webhook",
};

export const castNotificationMethod = (value: string) => {
	if (notificationMethods.includes(value as NotificationMethod)) {
		return value as NotificationMethod;
	}

	throw new Error(
		`Invalid notification method: ${value}. Accepted values: ${notificationMethods.join(
			", ",
		)}`,
	);
};

export function isTaskNotification(tmpl: NotificationTemplate): boolean {
	return tmpl.group === "Task Events";
}

// Determines if a notification is disabled based on user preferences and system defaults
// A notification is considered disabled if:
// 1. It's NOT enabled by default AND the user hasn't set any preference (undefined), OR
// 2. The user has explicitly disabled it in their preferences
// Returns true if disabled, false if enabled
export function notificationIsDisabled(
	disabledPreferences: Record<string, boolean>,
	tmpl: NotificationTemplate,
): boolean {
	return Boolean(
		(!tmpl.enabled_by_default && disabledPreferences[tmpl.id] === undefined) ||
			disabledPreferences[tmpl.id],
	);
}

// Transforms an array of NotificationPreference objects into a map
// where the key is the template ID and the value is whether it's disabled
// Example: [{ id: "abc", disabled: true }, { id: "def", disabled: false }]
export function selectDisabledPreferences(data: NotificationPreference[]) {
	return Object.fromEntries(data.map((pref) => [pref.id, pref.disabled]));
}
