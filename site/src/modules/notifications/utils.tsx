import EmailIcon from "@mui/icons-material/EmailOutlined";
import WebhookIcon from "@mui/icons-material/WebhookOutlined";

// TODO: This should be provided by the auto generated types from codersdk
const notificationMethods = ["smtp", "webhook"] as const;

export type NotificationMethod = (typeof notificationMethods)[number];

export const methodIcons: Record<NotificationMethod, typeof EmailIcon> = {
	smtp: EmailIcon,
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
