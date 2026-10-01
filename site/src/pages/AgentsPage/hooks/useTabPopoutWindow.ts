import { useCallback, useEffect, useRef, useState } from "react";
import { generateUUID } from "#/utils/random";
import type { ComposerSendResult } from "../context/ComposerContext";
import {
	isTabPopoutMessage,
	type TabPopoutMessage,
	tabPopoutChannelName,
} from "../utils/rightPanelTabPopout";

// How long a send may wait for the chat page to answer before the window
// gives up on it. The chat page normally answers within a round trip.
const sendTimeoutMs = 10_000;

type ChatState = {
	isAgentWorking: boolean;
};

type TabPopoutWindow = {
	// Chat state relayed by the chat page, for the tab to render.
	chatState: ChatState;
	// Sends a message through the chat page's composer. Rejects when no
	// chat page answers, so the caller can tell the user.
	send: (message: string) => Promise<ComposerSendResult>;
};

/**
 * The window side of a popped out right-panel tab. Announces the window
 * to the chat page, closes when asked to, and relays composer sends
 * through the chat page, so the tab behaves as it does in the panel.
 */
export function useTabPopoutWindow(tabId: string): TabPopoutWindow {
	const [chatState, setChatState] = useState<ChatState>({
		isAgentWorking: false,
	});
	const channelRef = useRef<BroadcastChannel>(null);
	const pendingSendsRef = useRef(
		new Map<
			string,
			(result: TabPopoutMessage & { type: "send-result" }) => void
		>(),
	);

	useEffect(() => {
		const channel = new BroadcastChannel(tabPopoutChannelName(tabId));
		channelRef.current = channel;
		const announce = () =>
			channel.postMessage({ type: "popout-opened" } satisfies TabPopoutMessage);
		announce();
		// In case the chat page registered its listener after the first one.
		const retry = setTimeout(announce, 300);

		channel.addEventListener("message", (event: MessageEvent<unknown>) => {
			if (!isTabPopoutMessage(event.data)) {
				return;
			}
			const message = event.data;
			switch (message.type) {
				case "probe":
					announce();
					break;
				case "bring-back":
					window.close();
					break;
				case "chat-state":
					setChatState({ isAgentWorking: message.isAgentWorking });
					break;
				case "send-result": {
					const settle = pendingSendsRef.current.get(message.id);
					if (settle) {
						pendingSendsRef.current.delete(message.id);
						settle(message);
					}
					break;
				}
			}
		});

		const closed = () =>
			channel.postMessage({ type: "popout-closed" } satisfies TabPopoutMessage);
		window.addEventListener("beforeunload", closed);
		return () => {
			clearTimeout(retry);
			closed();
			window.removeEventListener("beforeunload", closed);
			channel.close();
			channelRef.current = null;
		};
	}, [tabId]);

	const send = useCallback(
		(message: string) =>
			new Promise<ComposerSendResult>((resolve, reject) => {
				const channel = channelRef.current;
				if (!channel) {
					reject(new Error("The chat is not open, so nothing was sent."));
					return;
				}
				const id = generateUUID();
				const timer = setTimeout(() => {
					pendingSendsRef.current.delete(id);
					reject(new Error("The chat did not answer, so nothing was sent."));
				}, sendTimeoutMs);
				pendingSendsRef.current.set(id, ({ result }) => {
					clearTimeout(timer);
					if (typeof result === "string") {
						resolve(result);
					} else {
						reject(new Error(result.error));
					}
				});
				channel.postMessage({
					type: "send",
					id,
					message,
				} satisfies TabPopoutMessage);
			}),
		[],
	);

	return { chatState, send };
}
