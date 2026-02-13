import { API } from "api/api";
import { useEffect } from "react";
import { useQueryClient } from "react-query";
import { toast } from "sonner";
import { useChangelog } from "./ChangelogProvider";

const CHANGELOG_TOAST_KEY = "changelog-toast-last-seen";

export const useChangelogToast = () => {
	const { openChangelog } = useChangelog();
	const queryClient = useQueryClient();

	useEffect(() => {
		if (typeof localStorage === "undefined") {
			return;
		}

		let cancelled = false;

		const checkForUnread = async () => {
			try {
				const { notification } = await API.getUnreadChangelogNotification();
				if (cancelled || !notification) {
					return;
				}

				const version = notification.title;
				const lastSeen = localStorage.getItem(CHANGELOG_TOAST_KEY);
				if (lastSeen === version) {
					return;
				}

				// Mark as seen immediately so it only shows once.
				localStorage.setItem(CHANGELOG_TOAST_KEY, version);

				toast(`What's new in Coder ${version}`, {
					description: notification.content,
					duration: 10000,
					action: {
						label: "View changelog",
						onClick: () => {
							openChangelog(version);
							void API.updateInboxNotificationReadStatus(notification.id, {
								is_read: true,
							}).then(() => {
								void queryClient.invalidateQueries({
									queryKey: ["notifications"],
								});
							});
						},
					},
				});
			} catch {
				// Silently ignore — not critical.
			}
		};

		void checkForUnread();

		return () => {
			cancelled = true;
		};
	}, [openChangelog, queryClient]);
};
