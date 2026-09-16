import { type FC, useEffect, useEffectEvent, useRef } from "react";
import {
	type ChatStore,
	type ChatStoreState,
	useChatSelector,
} from "../ChatConversation/chatStore";
import { collectMcpAppToolCalls, type McpAppToolCallRef } from "./mcpAppTab";

const selectMcpAppToolCalls = (state: ChatStoreState): McpAppToolCallRef[] =>
	collectMcpAppToolCalls(
		state.messagesByID,
		state.orderedMessageIDs,
		state.streamState,
	);

interface McpAppTabSyncProps {
	store: ChatStore;
	/** While true the store is still receiving the loaded history. */
	isHydrating: boolean;
	/**
	 * Receives every app tool call in transcript order whenever the list
	 * changes. `initial` is true for the first call after hydration, which
	 * carries history rather than new activity.
	 */
	onAppToolCalls: (
		refs: readonly McpAppToolCallRef[],
		options: { initial: boolean },
	) => void;
}

/**
 * Renders nothing. Subscribes to the chat store and reports the app tool
 * calls in the transcript through `onAppToolCalls` whenever that list
 * changes identity.
 */
export const McpAppTabSync: FC<McpAppTabSyncProps> = ({
	store,
	isHydrating,
	onAppToolCalls,
}) => {
	const refs = useChatSelector(store, selectMcpAppToolCalls);
	const hasSyncedRef = useRef(false);
	const notify = useEffectEvent((current: readonly McpAppToolCallRef[]) => {
		const initial = !hasSyncedRef.current;
		hasSyncedRef.current = true;
		onAppToolCalls(current, { initial });
	});
	useEffect(() => {
		if (isHydrating) {
			return;
		}
		notify(refs);
	}, [refs, isHydrating]);
	return null;
};
