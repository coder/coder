import { useEffect } from "react";
import { matchPath, useNavigate } from "react-router";
import { isStandaloneDisplayMode } from "#/utils/platform";

/**
 * Query parameter that marks a page Safari opened for a notification's
 * navigate URL (Declarative Web Push), instead of the service worker opening
 * it with clients.openWindow().
 */
export const notificationLaunchParam = "notification";

let pendingChatPath: string | null = null;

// Read through functions: in a hook, the React Compiler replaced a local
// copy of this variable with a later read of the variable itself.
const hasPendingChatPath = () => pendingChatPath !== null;
const takePendingChatPath = () => {
	const chatPath = pendingChatPath;
	pendingChatPath = null;
	return chatPath;
};

// The server leaves the user metadata empty for a signed-out visitor
// (site/site.go). The development server leaves it unrendered.
const isRenderedSignedOut = () =>
	document
		.querySelector<HTMLMetaElement>('meta[property="user"]')
		?.getAttribute("content") === "";

/**
 * When Safari opens a standalone window on a chat as its only history entry
 * for a notification's navigate URL (marked with notificationLaunchParam),
 * makes the chat list the window's first entry. useOpenNotificationChat then
 * opens the chat on top of it, so the back swipe (the iOS edge swipe)
 * returns to the list. Pushing the list instead would not: WebKit's back
 * swipe skips entries that JavaScript pushed without user activation, down
 * to the first entry, which is the chat
 * (WebBackForwardList::goBackItemSkippingItemsWithoutUserGesture).
 *
 * Unmarked windows are left alone. In a window from clients.openWindow(),
 * iOS closes the window once a back swipe reaches its first entry and leaves
 * a blank screen, so giving that swipe a target makes it worse. In a browser
 * tab, in a window with more than one history entry, on a path other than
 * /agents/:agentId (agent settings excluded), or for a signed-out visitor,
 * whose sign-in redirect must keep the chat, a marked window only loses the
 * marker. Call before createBrowserRouter reads the location.
 */
export function openChatListFirst(): void {
	const url = new URL(window.location.href);
	if (!url.searchParams.has(notificationLaunchParam)) {
		return;
	}
	url.searchParams.delete(notificationLaunchParam);
	const { pathname, search, hash } = url;
	const chatMatch = matchPath("/agents/:agentId", pathname);
	if (
		window.history.length !== 1 ||
		!isStandaloneDisplayMode() ||
		isRenderedSignedOut() ||
		!chatMatch ||
		chatMatch.params.agentId === "settings"
	) {
		window.history.replaceState(
			window.history.state,
			"",
			`${pathname}${search}${hash}`,
		);
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
