import { useEffect } from "react";
import { useQueryClient } from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import { useChangelog } from "./ChangelogProvider";

const CHANGELOG_TOAST_KEY = "changelog-toast-last-seen";

const changelogToastStorageKey = (userID: string) =>
	`${CHANGELOG_TOAST_KEY}:${userID}`;

const unreadChangelogNotificationQueryKey = [
	"changelog",
	"unreadNotification",
] as const;

export const useChangelogToast = () => {
	const { openChangelog } = useChangelog();
	const queryClient = useQueryClient();

	useEffect(() => {
		if (typeof localStorage === "undefined") {
			return;
		}

		let cancelled = false;
		let settled = false;
		const timers: number[] = [];
		const pollDelaysMs = [0, 15000, 60000] as const;
		let pollAttempt = 0;

		const checkForUnread = async () => {
			if (cancelled || settled) {
				return;
			}

			try {
				const { notification } = await queryClient.fetchQuery({
					queryKey: unreadChangelogNotificationQueryKey,
					queryFn: API.getUnreadChangelogNotification,
					staleTime: 0,
				});
				if (cancelled || settled || !notification) {
					return;
				}

				const version = notification.title;
				const toastStorageKey = changelogToastStorageKey(notification.user_id);
				const lastSeen = localStorage.getItem(toastStorageKey);
				if (lastSeen === version) {
					return;
				}

				// Mark as seen immediately so it only shows once.
				localStorage.setItem(toastStorageKey, version);
				settled = true;

				toast(`What's new in Coder ${version}`, {
					description: notification.content,
					duration: 10000,
					action: {
						label: "View changelog",
						onClick: () => {
							openChangelog(version);
							void (async () => {
								try {
									await API.updateInboxNotificationReadStatus(notification.id, {
										is_read: true,
									});
								} catch {
									// Silently ignore — not critical.
								} finally {
									void queryClient.invalidateQueries({
										queryKey: ["notifications"],
									});
									void queryClient.invalidateQueries({
										queryKey: unreadChangelogNotificationQueryKey,
									});
								}
							})();
						},
					},
				});
			} catch {
				// Silently ignore — not critical.
			}
		};

		// BroadcastChangelog runs asynchronously at startup and may take longer
		// on larger deployments, so keep polling until a changelog notification
		// appears or this component unmounts.
		const scheduleNextPoll = () => {
			if (cancelled || settled) {
				return;
			}

			const delayMs = pollDelaysMs[Math.min(pollAttempt, pollDelaysMs.length - 1)];
			pollAttempt++;
			timers.push(
				window.setTimeout(() => {
					void checkForUnread().finally(() => {
						scheduleNextPoll();
					});
				}, delayMs),
			);
		};

		scheduleNextPoll();

		return () => {
			cancelled = true;
			for (const timer of timers) {
				window.clearTimeout(timer);
			}
		};
	}, [openChangelog, queryClient]);
};
