import { useEffect, useSyncExternalStore } from "react";
import { matchPath, useNavigate } from "react-router";
import { isStandaloneDisplayMode } from "#/utils/platform";

// The chat that a window opened for a push notification shows once the chat
// list behind it has painted.
let pendingChatPath: string | null = null;
const pendingChatListeners = new Set<() => void>();

const setPendingChatPath = (chatPath: string | null) => {
	pendingChatPath = chatPath;
	for (const listener of pendingChatListeners) {
		listener();
	}
};

const subscribePendingChatPath = (listener: () => void) => {
	pendingChatListeners.add(listener);
	return () => {
		pendingChatListeners.delete(listener);
	};
};

const getPendingChatPath = () => pendingChatPath;

const chatPathFor = (pathname: string): string | null => {
	const chatMatch = matchPath("/agents/:agentId", pathname);
	if (!chatMatch || chatMatch.params.agentId === "settings") {
		return null;
	}
	return pathname;
};

/**
 * Opens a chat on top of the chat list once the list has painted. Used when
 * the chat to open becomes known after boot.
 */
export function requestNotificationChat(chatPath: string): void {
	if (chatPathFor(chatPath) && window.location.pathname === "/agents") {
		setPendingChatPath(chatPath);
	}
}

/**
 * Makes the chat list the first history entry of a window opened for a push
 * notification, so the back swipe (the iOS edge swipe) returns to the list.
 * Handles two launches:
 *
 * - `/agents?chat=<id>`: the list, with the chat to open in the query.
 * - `/agents/<id>` as a standalone window's only entry: replaced by the list.
 *
 * useOpenNotificationChat then opens the chat on top of the list. Call before
 * createBrowserRouter reads the location.
 */
export function openChatListFirst(): void {
	const { pathname, search, hash } = window.location;
	if (pathname === "/agents") {
		const params = new URLSearchParams(search);
		const chatId = params.get("chat");
		const chatPath = chatId ? chatPathFor(`/agents/${chatId}`) : null;
		if (!chatPath) {
			return;
		}
		params.delete("chat");
		const rest = params.toString();
		setPendingChatPath(chatPath);
		window.history.replaceState(
			null,
			"",
			`/agents${rest ? `?${rest}` : ""}${hash}`,
		);
		return;
	}
	if (window.history.length !== 1 || !isStandaloneDisplayMode()) {
		return;
	}
	const chatPath = chatPathFor(pathname);
	if (!chatPath) {
		return;
	}
	setPendingChatPath(`${chatPath}${search}${hash}`);
	window.history.replaceState(null, "", `/agents${search}`);
}

/**
 * Opens the chat set aside by openChatListFirst or requestNotificationChat
 * once the chat list has loaded and painted, or after 1.5 s. WebKit takes
 * the back swipe's picture of the list when the chat entry is pushed;
 * pushing before the list paints shows a blank page during the swipe. Drops
 * the chat if the user has already left the list.
 */
export function useOpenNotificationChat(isListLoading: boolean): void {
	const navigate = useNavigate();
	const chatPath = useSyncExternalStore(
		subscribePendingChatPath,
		getPendingChatPath,
	);
	useEffect(() => {
		if (!chatPath) {
			return;
		}
		let frame = 0;
		const open = () => {
			frame = requestAnimationFrame(() => {
				frame = requestAnimationFrame(() => {
					setPendingChatPath(null);
					if (window.location.pathname === "/agents") {
						void navigate(chatPath);
					}
				});
			});
		};
		const timer = isListLoading ? setTimeout(open, 1500) : undefined;
		if (!isListLoading) {
			open();
		}
		return () => {
			clearTimeout(timer);
			cancelAnimationFrame(frame);
		};
	}, [chatPath, isListLoading, navigate]);
}
