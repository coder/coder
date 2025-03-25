import { watchInboxNotifications } from "api/api";
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
const NOTIFICATIONS_LIMIT = 25; // This is hard set in the API

type NotificationsInboxProps = {
	defaultOpen?: boolean;
	fetchNotifications: (
		startingBeforeId?: string,
	) => Promise<ListInboxNotificationsResponse>;
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
		data: inboxRes,
		error,
		refetch,
	} = useQuery({
		queryKey: NOTIFICATIONS_QUERY_KEY,
		queryFn: () => fetchNotifications(),
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
		const socket = watchInboxNotifications({ read_status: "unread" });

		socket.addEventListener("message", (e) => {
			if (e.parseError) {
				console.warn("Error parsing inbox notification: ", e.parseError);
				return;
			}

			const msg = e.parsedMessage;
			updateNotificationsCache((current) => {
				return {
					unread_count: msg.unread_count,
					notifications: [msg.notification, ...current.notifications],
				};
			});
		});

		socket.addEventListener("error", () => {
			displayError(
				"Unable to retrieve latest inbox notifications. Please try refreshing the browser.",
			);
			socket.close();
		});

		return () => socket.close();
	}, [updateNotificationsCache]);

	const {
		mutate: loadMoreNotifications,
		isLoading: isLoadingMoreNotifications,
	} = useMutation({
		mutationFn: async () => {
			if (!inboxRes || inboxRes.notifications.length === 0) {
				return;
			}
			const lastNotification =
				inboxRes.notifications[inboxRes.notifications.length - 1];
			const newRes = await fetchNotifications(lastNotification.id);
			updateNotificationsCache((prev) => {
				return {
					unread_count: newRes.unread_count,
					notifications: [...prev.notifications, ...newRes.notifications],
				};
			});
		},
		onError: (error) => {
			displayError(
				getErrorMessage(error, "Error loading more notifications"),
				getErrorDetail(error),
			);
		},
	});

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
			notifications={inboxRes?.notifications}
			unreadCount={inboxRes?.unread_count ?? 0}
			error={error}
			isLoadingMoreNotifications={isLoadingMoreNotifications}
			hasMoreNotifications={Boolean(
				inboxRes && inboxRes.notifications.length % NOTIFICATIONS_LIMIT === 0,
			)}
			onRetry={refetch}
			onMarkAllAsRead={markAllAsReadMutation.mutate}
			onMarkNotificationAsRead={markNotificationAsReadMutation.mutate}
			onLoadMoreNotifications={loadMoreNotifications}
		/>
	);
};
