import { watchInboxNotifications } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type {
	ListInboxNotificationsResponse,
	UpdateInboxNotificationReadStatusResponse,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { useEffectEvent } from "hooks/hookPolyfills";
import { type FC, useEffect, useMemo } from "react";
import {
	type QueryFunctionContext,
	useInfiniteQuery,
	useMutation,
	useQueryClient,
} from "react-query";
import { InboxPopover } from "./InboxPopover";

const NOTIFICATIONS_QUERY_KEY = ["notifications"];

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
	type CTX = QueryFunctionContext<typeof NOTIFICATIONS_QUERY_KEY, string>;
	const {
		data: infiniteInboxRes,
		error,
		refetch,
		fetchNextPage,
		isFetchingNextPage,
		hasNextPage = false,
	} = useInfiniteQuery({
		queryKey: NOTIFICATIONS_QUERY_KEY,
		queryFn: ({ pageParam }: CTX) => fetchNotifications(pageParam),
		getNextPageParam: (latestPage, allPages) => {
			const notificationsLoaded = allPages.reduce(
				(count, page) => count + page.notifications.length,
				0,
			);
			if (notificationsLoaded >= latestPage.unread_count) {
				return undefined;
			}
			return latestPage.notifications.at(-1)?.id;
		},
	});
	const flatInbox = useMemo<ListInboxNotificationsResponse | undefined>(() => {
		if (infiniteInboxRes === undefined) {
			return undefined;
		}
		return {
			notifications: infiniteInboxRes.pages.flatMap((p) => p.notifications),
			unread_count: infiniteInboxRes.pages.at(0)?.unread_count ?? 0,
		};
	}, [infiniteInboxRes]);

	const queryClient = useQueryClient();
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
			notifications={flatInbox?.notifications}
			unreadCount={flatInbox?.unread_count ?? 0}
			error={error}
			isLoadingMoreNotifications={isFetchingNextPage}
			hasMoreNotifications={hasNextPage}
			onRetry={refetch}
			onMarkAllAsRead={markAllAsReadMutation.mutate}
			onMarkNotificationAsRead={markNotificationAsReadMutation.mutate}
			onLoadMoreNotifications={fetchNextPage}
		/>
	);
};
