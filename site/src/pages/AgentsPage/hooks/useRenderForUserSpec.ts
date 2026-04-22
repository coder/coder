import type { Spec } from "@json-render/react";
import { useMemo, useSyncExternalStore } from "react";
import type { ChatMessage } from "#/api/typesGenerated";

/**
 * Data extracted from a render_for_user tool call.
 */
export interface RenderForUserData {
	title: string;
	spec: Spec;
}

/**
 * Minimal store interface matching the ChatStoreHandle shape.
 */
interface StoreHandle {
	getSnapshot: () => {
		messagesByID: Map<number, ChatMessage>;
		orderedMessageIDs: readonly number[];
	};
	subscribe: (listener: () => void) => () => void;
}

/**
 * Scans chat messages for the most recent render_for_user
 * tool call and extracts the JSON spec from its args.
 *
 * Returns the extracted spec data or null if no tool call
 * is found.
 */
function extractRenderSpec(
	messagesByID: Map<number, ChatMessage>,
	orderedMessageIDs: readonly number[],
): RenderForUserData | null {
	// Walk messages in reverse to find the most recent one.
	for (let i = orderedMessageIDs.length - 1; i >= 0; i--) {
		const msgId = orderedMessageIDs[i];
		const msg = messagesByID.get(msgId!);
		if (!msg?.content || !Array.isArray(msg.content)) {
			continue;
		}
		for (const part of msg.content) {
			if (
				part.type === "tool-call" &&
				part.tool_name === "render_for_user" &&
				part.args
			) {
				try {
					const args =
						typeof part.args === "string" ? JSON.parse(part.args) : part.args;
					const title = typeof args.title === "string" ? args.title : "AI View";
					const spec =
						typeof args.spec === "string" ? JSON.parse(args.spec) : args.spec;
					if (spec && typeof spec === "object" && spec.root) {
						return { title, spec: spec as Spec };
					}
				} catch {
					// Malformed args; skip.
				}
			}
		}
	}
	return null;
}

/**
 * Hook that subscribes to the chat store and returns the latest
 * render_for_user spec if one exists. Returns null otherwise.
 */
export function useRenderForUserSpec(
	store: StoreHandle | undefined,
): RenderForUserData | null {
	const snapshot = useSyncExternalStore(
		store?.subscribe ?? (() => () => {}),
		store?.getSnapshot ??
			(() => ({ messagesByID: new Map(), orderedMessageIDs: [] })),
	);

	return useMemo(
		() => extractRenderSpec(snapshot.messagesByID, snapshot.orderedMessageIDs),
		[snapshot.messagesByID, snapshot.orderedMessageIDs],
	);
}
