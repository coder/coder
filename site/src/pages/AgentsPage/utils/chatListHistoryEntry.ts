import { matchPath } from "react-router";

/**
 * Inserts the chat list as the history entry before a chat that opened an
 * installed (standalone) app window, as a push notification click does. With
 * the chat as the window's only entry and no browser back button, the system
 * back gesture, such as the iOS edge swipe, has nothing to go back to.
 * Browser tabs keep their own history.
 */
export function insertChatListHistoryEntry(): void {
	if (
		window.history.length !== 1 ||
		!window.matchMedia("(display-mode: standalone)").matches
	) {
		return;
	}
	const { pathname, search, hash } = window.location;
	const chat = matchPath("/agents/:agentId", pathname);
	if (!chat || chat.params.agentId === "settings") {
		return;
	}
	// WebKit's back swipe skips entries that JavaScript pushed without user
	// interaction but stops at the window's first entry, so the chat list takes
	// over that entry instead of being pushed too. The states match what React
	// Router writes for opening the chat list and tapping a chat.
	window.history.replaceState({ idx: 0 }, "", `/agents${search}`);
	window.history.pushState(
		{ usr: null, key: Math.random().toString(36).slice(2, 10), idx: 1 },
		"",
		`${pathname}${search}${hash}`,
	);
}
