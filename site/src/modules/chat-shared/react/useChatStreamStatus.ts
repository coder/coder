import type { ChatStoreState } from "../core";
import { useChatStoreSnapshot } from "./ChatRuntimeProvider";

/** @public Stream status derived from the shared chat store. */
export type UseChatStreamStatusResult = Pick<
	ChatStoreState,
	| "chatStatus"
	| "streamState"
	| "streamError"
	| "retryState"
	| "subagentStatusOverrides"
> & {
	isStreaming: boolean;
	isRetrying: boolean;
	hasError: boolean;
	isIdle: boolean;
};

/** @public Reads stream status for the active shared chat. */
export const useChatStreamStatus = (): UseChatStreamStatusResult => {
	const snapshot = useChatStoreSnapshot();
	const isStreaming =
		snapshot.chatStatus === "running" || snapshot.streamState !== null;
	const isRetrying = snapshot.retryState !== null;
	const hasError =
		snapshot.chatStatus === "error" || snapshot.streamError !== null;

	return {
		chatStatus: snapshot.chatStatus,
		streamState: snapshot.streamState,
		streamError: snapshot.streamError,
		retryState: snapshot.retryState,
		subagentStatusOverrides: snapshot.subagentStatusOverrides,
		isStreaming,
		isRetrying,
		hasError,
		isIdle: !isStreaming && !isRetrying && !hasError,
	};
};
