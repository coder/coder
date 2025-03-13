import { getErrorDetail, getErrorMessage } from "api/errors";
import { displayError } from "components/GlobalSnackbar/utils";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { InboxPopover } from "./InboxPopover";
import type { Notification } from "./types";

const NOTIFICATIONS_QUERY_KEY = ["notifications"];

type NotificationsResponse = {
	notifications: Notification[];
	unread_count: number;
};

type NotificationsInboxProps = {
	defaultOpen?: boolean;
	fetchNotifications: () => Promise<NotificationsResponse>;
	markAllAsRead: () => Promise<void>;
	markNotificationAsRead: (notificationId: string) => Promise<void>;
};

export const NotificationsInbox: FC<NotificationsInboxProps> = ({
	defaultOpen,
	fetchNotifications,
	markAllAsRead,
	markNotificationAsRead,
}) => {
	const queryClient = useQueryClient();

	const {
		data: res,
		error,
		refetch,
	} = useQuery({
		queryKey: NOTIFICATIONS_QUERY_KEY,
		queryFn: fetchNotifications,
	});

	const markAllAsReadMutation = useMutation({
		mutationFn: markAllAsRead,
		onSuccess: () => {
			safeUpdateNotificationsCache((prev) => {
				return {
					unread_count: 0,
					notifications: prev.notifications.map((n) => ({
						...n,
						read_status: "read",
					})),
				};
			});
		},
		onError: (error) => {
			displayError(
				getErrorMessage(error, "Error on marking all notifications as read"),
				getErrorDetail(error),
			);
		},
	});

	const markNotificationAsReadMutation = useMutation({
		mutationFn: markNotificationAsRead,
		onSuccess: (_, notificationId) => {
			safeUpdateNotificationsCache((prev) => {
				return {
					unread_count: prev.unread_count - 1,
					notifications: prev.notifications.map((n) => {
						if (n.id !== notificationId) {
							return n;
						}
						return { ...n, read_status: "read" };
					}),
				};
			});
		},
		onError: (error) => {
			displayError(
				getErrorMessage(error, "Error on marking notification as read"),
				getErrorDetail(error),
			);
		},
	});

	async function safeUpdateNotificationsCache(
		callback: (res: NotificationsResponse) => NotificationsResponse,
	) {
		await queryClient.cancelQueries(NOTIFICATIONS_QUERY_KEY);
		queryClient.setQueryData<NotificationsResponse>(
			NOTIFICATIONS_QUERY_KEY,
			(prev) => {
				if (!prev) {
					return { notifications: [], unread_count: 0 };
				}
				return callback(prev);
			},
		);
	}

	return (
		<InboxPopover
			defaultOpen={defaultOpen}
			notifications={res?.notifications}
			unreadCount={res?.unread_count ?? 0}
			error={error}
			onRetry={refetch}
			onMarkAllAsRead={markAllAsReadMutation.mutate}
			onMarkNotificationAsRead={markNotificationAsReadMutation.mutate}
		/>
	);
};
