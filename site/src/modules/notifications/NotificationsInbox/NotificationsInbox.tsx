import { API, watchInboxNotifications } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type {
	ListInboxNotificationsResponse,
	UpdateInboxNotificationReadStatusResponse,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffectEvent } from "hooks/hookPolyfills";
import { type FC, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { InboxPopover } from "./InboxPopover";

const NOTIFICATIONS_QUERY_KEY = ["notifications"];

type NotificationsInboxProps = {
	defaultOpen?: boolean;
	fetchNotifications: () => Promise<ListInboxNotificationsResponse>;
	markAllAsRead: () => Promise<void>;
	markNotificationAsRead: (
		notificationId: string,
	) => Promise<UpdateInboxNotificationReadStatusResponse>;
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

	const updateNotificationsCache = useEffectEvent(
		async (
			callback: (
				res: ListInboxNotificationsResponse,
			) => ListInboxNotificationsResponse,
		) => {
			await queryClient.cancelQueries(NOTIFICATIONS_QUERY_KEY);
			queryClient.setQueryData<ListInboxNotificationsResponse>(
				NOTIFICATIONS_QUERY_KEY,
				(prev) => {
					if (!prev) {
						return { notifications: [], unread_count: 0 };
					}
					return callback(prev);
				},
			);
		},
	);

	useEffect(() => {
		const socket = watchInboxNotifications(
			(res) => {
				updateNotificationsCache((prev) => {
					return {
						unread_count: res.unread_count,
						notifications: [res.notification, ...prev.notifications],
					};
				});
			},
			{ read_status: "unread" },
		);

		return () => {
			socket.close();
		};
	}, [updateNotificationsCache]);

	const markAllAsReadMutation = useMutation({
		mutationFn: markAllAsRead,
		onSuccess: () => {
			updateNotificationsCache((prev) => {
				return {
					unread_count: 0,
					notifications: prev.notifications.map((n) => ({
						...n,
						read_at: new Date().toISOString(),
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
		onSuccess: (res) => {
			updateNotificationsCache((prev) => {
				return {
					unread_count: res.unread_count,
					notifications: prev.notifications.map((n) => {
						if (n.id !== res.notification.id) {
							return n;
						}
						return res.notification;
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
