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

// The server leaves the user metadata empty for a signed-out visitor
// (site/site.go). The development server leaves it unrendered.
const isRenderedSignedOut = () =>
	document
		.querySelector<HTMLMetaElement>('meta[property="user"]')
		?.getAttribute("content") === "";

/**
 * When a standalone window opens on a chat as its only history entry, as a
 * push notification click does, makes the chat list the window's first
 * entry. useOpenNotificationChat then opens the chat on top of it, so the
 * back swipe (the iOS edge swipe) returns to the list. Pushing the list
 * instead would not: WebKit's back swipe skips entries that JavaScript
 * pushed without user activation, down to the first entry
 * (WebBackForwardList::goBackItemSkippingItemsWithoutUserGesture), which is
 * the chat. Does nothing in a browser tab, in a window with more than one
 * history entry, on a path other than /agents/:agentId (agent settings
 * excluded), or for a signed-out visitor, whose sign-in redirect must keep
 * the chat. Call before createBrowserRouter reads the location.
 */
export function openChatListFirst(): void {
	if (
		window.history.length !== 1 ||
		!isStandaloneDisplayMode() ||
		isRenderedSignedOut()
	) {
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
