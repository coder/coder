import type { ComposerSendResult } from "../context/ComposerContext";

/**
 * A right-panel tab can be shown in a window of its own, at
 * `/agents/{chatId}/tabs/{tabId}`. That window and the chat page keep in
 * touch over a BroadcastChannel named for the tab. Only documents on the
 * dashboard's origin can join it, so everything on the channel comes from
 * dashboard code, never from the previewed app.
 */
export type TabPopoutMessage =
	// The window is showing the tab; the chat page steps aside.
	| { type: "popout-opened" }
	| { type: "popout-closed" }
	// The chat page asks a window that may already be open to announce
	// itself, after a reload for example.
	| { type: "probe" }
	// The chat page asks the window to close.
	| { type: "bring-back" }
	// Chat state the tab renders, sent whenever it changes.
	| { type: "chat-state"; isAgentWorking: boolean }
	// The window asks the chat page to send a message through its
	// composer, and hears back how that went.
	| { type: "send"; id: string; message: string }
	| {
			type: "send-result";
			id: string;
			result: ComposerSendResult | { error: string };
	  };

export function tabPopoutChannelName(tabId: string): string {
	return `coder-right-panel-tab-${tabId}`;
}

export function tabPopoutPath(chatId: string, tabId: string): string {
	return `/agents/${chatId}/tabs/${tabId}`;
}

// Posts one message and closes. Delivery is queued when the message is
// posted, so closing straight after does not lose it.
export function postToTabPopout(
	tabId: string,
	message: TabPopoutMessage,
): void {
	const channel = new BroadcastChannel(tabPopoutChannelName(tabId));
	channel.postMessage(message);
	channel.close();
}

export function isTabPopoutMessage(value: unknown): value is TabPopoutMessage {
	return (
		typeof value === "object" &&
		value !== null &&
		typeof (value as { type?: unknown }).type === "string"
	);
}
