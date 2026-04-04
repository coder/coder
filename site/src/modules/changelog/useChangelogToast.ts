import { useEffect } from "react";
import { useQueryClient } from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import { useChangelog } from "./ChangelogProvider";

const CHANGELOG_TOAST_KEY = "changelog-toast-last-seen";

const changelogToastStorageKey = (userID: string) =>
	`${CHANGELOG_TOAST_KEY}:${userID}`;

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

		const checkForUnread = async () => {
			if (cancelled || settled) {
				return;
			}

			try {
				const { notification } = await API.getUnreadChangelogNotification();
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
								}
							})();
						},
					},
				});
			} catch {
				// Silently ignore — not critical.
			}
		};

		// BroadcastChangelog runs asynchronously at startup; retry a few times
		// so users who open the dashboard immediately after an upgrade still
		// get the toast without manually refreshing.
		for (const delayMs of [0, 15000, 60000] as const) {
			timers.push(
				window.setTimeout(() => {
					void checkForUnread();
				}, delayMs),
			);
		}

		return () => {
			cancelled = true;
			for (const timer of timers) {
				window.clearTimeout(timer);
			}
		};
	}, [openChangelog, queryClient]);
};
