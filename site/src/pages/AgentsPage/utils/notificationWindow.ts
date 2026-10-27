import { useEffect } from "react";
import { matchPath, useNavigate } from "react-router";
import { isStandaloneDisplayMode } from "#/utils/platform";

let pendingChatPath: string | null = null;

// Read through functions: in a hook, the React Compiler replaced a local
// copy of this variable with a later read of the variable itself.
const hasPendingChatPath = () => pendingChatPath !== null;
const takePendingChatPath = () => {
	const chatPath = pendingChatPath;
	pendingChatPath = null;
	return chatPath;
};

/**
 * When a standalone window opens on a chat as its only history entry, as a
 * push notification click does, makes the chat list the window's first
 * entry. useOpenNotificationChat then opens the chat on top of it, so the
 * back swipe (the iOS edge swipe) returns to the list. Does nothing in a
 * browser tab, in a window with more than one history entry, or on a path
 * other than /agents/:agentId (agent settings excluded). Call before
 * createBrowserRouter reads the location.
 */
export function openChatListFirst(): void {
	if (window.history.length !== 1 || !isStandaloneDisplayMode()) {
		return;
	}
	const { pathname, search, hash } = window.location;
	const chatMatch = matchPath("/agents/:agentId", pathname);
	if (!chatMatch || chatMatch.params.agentId === "settings") {
		return;
	}
	pendingChatPath = `${pathname}${search}${hash}`;
	window.history.replaceState(null, "", `/agents${search}`);
}

/**
 * Opens the chat set aside by openChatListFirst once the chat list has
 * loaded and painted, or after 1.5 s. WebKit takes the back swipe's picture
 * of the list when the chat entry is pushed; pushing before the list paints
 * shows a blank page during the swipe. Drops the chat if the user has
 * already left the list.
 */
export function useOpenNotificationChat(isListLoading: boolean): void {
	const navigate = useNavigate();
	useEffect(() => {
		if (!hasPendingChatPath()) {
			return;
		}
		let frame = 0;
		const open = () => {
			frame = requestAnimationFrame(() => {
				frame = requestAnimationFrame(() => {
					const chatPath = takePendingChatPath();
					if (chatPath && window.location.pathname === "/agents") {
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
	}, [isListLoading, navigate]);
}
