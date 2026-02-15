import { watchInboxNotifications } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type {
	ListInboxNotificationsResponse,
	UpdateInboxNotificationReadStatusResponse,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffectEvent } from "hooks/hookPolyfills";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { InboxPopover } from "./InboxPopover";

export type ReadStatus = "read" | "unread" | "all";

const NOTIFICATIONS_QUERY_KEY = ["notifications"];
const NOTIFICATIONS_LIMIT = 25; // This is hard set in the API

type NotificationsInboxProps = {
	defaultOpen?: boolean;
	fetchNotifications: (
		startingBeforeId?: string,
		readStatus?: ReadStatus,
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
	const [readStatus, setReadStatus] = useState<ReadStatus>("unread");

	const queryKey = [...NOTIFICATIONS_QUERY_KEY, readStatus];

	const {
		data: inboxRes,
		error,
		refetch,
		isFetching,
	} = useQuery({
		queryKey,
		queryFn: () => fetchNotifications(undefined, readStatus),
	});

	const updateNotificationsCache = useEffectEvent(
		async (
			callback: (
				res: ListInboxNotificationsResponse,
			) => ListInboxNotificationsResponse,
		) => {
			await queryClient.cancelQueries({ queryKey });
			queryClient.setQueryData<ListInboxNotificationsResponse>(
				queryKey,
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
		isPending: isLoadingMoreNotifications,
	} = useMutation({
		mutationFn: async () => {
			if (!inboxRes || inboxRes.notifications.length === 0) {
				return;
			}
			const lastNotification =
				inboxRes.notifications[inboxRes.notifications.length - 1];
			const newRes = await fetchNotifications(lastNotification.id, readStatus);
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
						read_at: n.read_at ?? new Date().toISOString(),
					})),
				};
			});
			// Mark all notification caches as stale so other views refresh
			// when next accessed. We use refetchType: 'none' to avoid an
			// immediate refetch that would replace the paginated cache
			// (which may contain multiple "Load more" pages) with only
			// the first page.
			queryClient.invalidateQueries({
				queryKey: NOTIFICATIONS_QUERY_KEY,
				exact: false,
				refetchType: "none",
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
			// Same as markAllAsRead: mark stale without immediate refetch
			// to preserve paginated cache.
			queryClient.invalidateQueries({
				queryKey: NOTIFICATIONS_QUERY_KEY,
				exact: false,
				refetchType: "none",
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
			readStatus={readStatus}
			notifications={isFetching ? undefined : inboxRes?.notifications}
			unreadCount={inboxRes?.unread_count ?? 0}
			error={isFetching ? undefined : error}
			isLoadingMoreNotifications={isLoadingMoreNotifications}
			hasMoreNotifications={Boolean(
				inboxRes && inboxRes.notifications.length % NOTIFICATIONS_LIMIT === 0,
			)}
			onRetry={refetch}
			onMarkAllAsRead={markAllAsReadMutation.mutate}
			onMarkNotificationAsRead={markNotificationAsReadMutation.mutate}
			onLoadMoreNotifications={loadMoreNotifications}
			onViewAllNotifications={() => setReadStatus("all")}
			onClose={() => setReadStatus("unread")}
		/>
	);
};
