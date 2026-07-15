import { useSyncExternalStore } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
} from "../../utils/usageLimitMessage";
import type { ChatPreviewPart } from "./chatExecutionStream";
import { applyMessagePartToStreamState } from "./streamState";
import type { ReconnectState, RetryState, StreamState } from "./types";

const retryStatesEqual = (
	left: RetryState | null,
	right: RetryState | null,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return false;
	}
	return (
		left.attempt === right.attempt &&
		left.error === right.error &&
		left.kind === right.kind &&
		left.provider === right.provider &&
		left.delayMs === right.delayMs &&
		left.retryingAt === right.retryingAt
	);
};

const reconnectStatesEqual = (
	left: ReconnectState | null,
	right: ReconnectState | null,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return false;
	}
	return (
		left.attempt === right.attempt &&
		left.delayMs === right.delayMs &&
		left.retryingAt === right.retryingAt
	);
};

export const isActiveChatStatus = (
	status: TypesGen.ChatStatus | null,
): boolean => status === "running" || status === "interrupting";

export type ChatStreamStoreState = {
	streamState: StreamState | null;
	previewConnectionEpoch: number;
	previewHistoryVersion?: number;
	previewGenerationAttempt?: number;
	previewLastSeq?: number;
	transientError: ChatDetailError | null;
	retryState: RetryState | null;
	reconnectState: ReconnectState | null;
};

export type ChatStreamStore = {
	getSnapshot: () => ChatStreamStoreState;
	subscribe: (listener: () => void) => () => void;
	batch: (fn: () => void) => void;
	applyMessagePart: (part: TypesGen.ChatMessagePart) => void;
	applyMessageParts: (parts: readonly TypesGen.ChatMessagePart[]) => void;
	applyPreviewParts: (parts: readonly ChatPreviewPart[]) => boolean;
	setStreamState: (streamState: StreamState | null) => void;
	setTransientError: (reason: ChatDetailError | null) => void;
	clearTransientError: () => void;
	setRetryState: (state: RetryState | null) => void;
	clearRetryState: () => void;
	setReconnectState: (state: ReconnectState | null) => void;
	clearReconnectState: () => void;
	clearStreamState: () => void;
	resetTransportReplayState: () => void;
	resetTransientState: () => void;
};

const createInitialState = (): ChatStreamStoreState => ({
	streamState: null,
	previewConnectionEpoch: 0,
	transientError: null,
	retryState: null,
	reconnectState: null,
});

export const createChatStreamStore = (): ChatStreamStore => {
	let state = createInitialState();
	const listeners = new Set<() => void>();

	const emit = (): void => {
		for (const listener of listeners) {
			listener();
		}
	};

	// Batching: suppress emit() during a batch and fire once
	// at the end. This collapses N store mutations from a
	// single WebSocket message into one subscriber notification.
	let batchDepth = 0;
	let batchDirty = false;

	const batch = (fn: () => void): void => {
		batchDepth += 1;
		try {
			fn();
		} finally {
			batchDepth -= 1;
			if (batchDepth === 0 && batchDirty) {
				batchDirty = false;
				emit();
			}
		}
	};

	const setState = (
		updater: (current: ChatStreamStoreState) => ChatStreamStoreState,
	): void => {
		const next = updater(state);
		if (next === state) {
			return;
		}
		state = next;
		if (batchDepth > 0) {
			batchDirty = true;
		} else {
			emit();
		}
	};

	const applyMessageParts = (parts: readonly TypesGen.ChatMessagePart[]) => {
		if (parts.length === 0) {
			return;
		}

		setState((current) => {
			let nextStreamState: StreamState | null = current.streamState;
			for (const part of parts) {
				nextStreamState = applyMessagePartToStreamState(nextStreamState, part);
			}
			if (nextStreamState === current.streamState) {
				return current;
			}
			return {
				...current,
				streamState: nextStreamState,
			};
		});
	};

	const applyPreviewParts = (parts: readonly ChatPreviewPart[]): boolean => {
		if (parts.length === 0) {
			return false;
		}
		let acceptedAny = false;
		setState((current) => {
			let nextStreamState: StreamState | null = current.streamState;
			let connectionEpoch = current.previewConnectionEpoch;
			let historyVersion = current.previewHistoryVersion;
			let generationAttempt = current.previewGenerationAttempt;
			let lastSeq = current.previewLastSeq;
			let changed = false;

			for (const preview of parts) {
				if (preview.connectionEpoch < connectionEpoch) {
					continue;
				}
				if (preview.connectionEpoch > connectionEpoch) {
					connectionEpoch = preview.connectionEpoch;
					historyVersion = preview.historyVersion;
					generationAttempt = preview.generationAttempt;
					lastSeq = undefined;
					nextStreamState = null;
				}

				const nextHistoryVersion = preview.historyVersion ?? historyVersion;
				const nextGenerationAttempt =
					preview.generationAttempt ?? generationAttempt;
				const episodeIsOlder =
					nextHistoryVersion !== undefined &&
					historyVersion !== undefined &&
					(nextHistoryVersion < historyVersion ||
						(nextHistoryVersion === historyVersion &&
							nextGenerationAttempt !== undefined &&
							generationAttempt !== undefined &&
							nextGenerationAttempt < generationAttempt));
				if (episodeIsOlder) {
					continue;
				}
				const episodeIsNewer =
					nextHistoryVersion !== historyVersion ||
					nextGenerationAttempt !== generationAttempt;
				if (episodeIsNewer) {
					historyVersion = nextHistoryVersion;
					generationAttempt = nextGenerationAttempt;
					lastSeq = undefined;
					nextStreamState = null;
				}
				if (
					preview.seq !== undefined &&
					lastSeq !== undefined &&
					preview.seq <= lastSeq
				) {
					continue;
				}
				acceptedAny = true;
				const applied = applyMessagePartToStreamState(
					nextStreamState,
					preview.part,
				);
				if (applied !== nextStreamState) {
					nextStreamState = applied;
					changed = true;
				}
				if (preview.seq !== undefined) {
					lastSeq = preview.seq;
				}
			}

			if (
				!changed &&
				connectionEpoch === current.previewConnectionEpoch &&
				historyVersion === current.previewHistoryVersion &&
				generationAttempt === current.previewGenerationAttempt &&
				lastSeq === current.previewLastSeq
			) {
				return current;
			}
			return {
				...current,
				streamState: nextStreamState,
				previewConnectionEpoch: connectionEpoch,
				previewHistoryVersion: historyVersion,
				previewGenerationAttempt: generationAttempt,
				previewLastSeq: lastSeq,
			};
		});
		return acceptedAny;
	};

	return {
		getSnapshot: () => state,
		subscribe: (listener) => {
			listeners.add(listener);
			return () => {
				listeners.delete(listener);
			};
		},
		batch,
		applyMessagePart: (part) => applyMessageParts([part]),
		applyMessageParts,
		applyPreviewParts,
		setStreamState: (streamState) => {
			if (state.streamState === streamState) {
				return;
			}
			setState((current) => {
				if (current.streamState === streamState) {
					return current;
				}
				return {
					...current,
					streamState,
				};
			});
		},
		setTransientError: (reason) => {
			setState((current) => {
				if (chatDetailErrorsEqual(current.transientError, reason)) {
					return current;
				}
				return {
					...current,
					transientError: reason,
				};
			});
		},
		clearTransientError: () => {
			if (state.transientError === null) {
				return;
			}
			setState((current) => ({
				...current,
				transientError: null,
			}));
		},
		setRetryState: (retryState) => {
			setState((current) => {
				if (retryStatesEqual(current.retryState, retryState)) {
					return current;
				}
				return {
					...current,
					retryState,
				};
			});
		},
		clearRetryState: () => {
			if (state.retryState === null) {
				return;
			}
			setState((current) => ({
				...current,
				retryState: null,
			}));
		},
		setReconnectState: (reconnectState) => {
			setState((current) => {
				if (reconnectStatesEqual(current.reconnectState, reconnectState)) {
					return current;
				}
				return {
					...current,
					reconnectState,
				};
			});
		},
		clearReconnectState: () => {
			if (state.reconnectState === null) {
				return;
			}
			setState((current) => ({
				...current,
				reconnectState: null,
			}));
		},
		clearStreamState: () => {
			if (state.streamState === null) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
				previewHistoryVersion: undefined,
				previewGenerationAttempt: undefined,
				previewLastSeq: undefined,
			}));
		},
		resetTransportReplayState: () => {
			if (
				state.reconnectState === null &&
				state.streamState === null &&
				state.transientError === null &&
				state.retryState === null
			) {
				return;
			}
			setState((current) => ({
				...current,
				reconnectState: null,
				streamState: null,
				previewConnectionEpoch: current.previewConnectionEpoch + 1,
				previewHistoryVersion: undefined,
				previewGenerationAttempt: undefined,
				previewLastSeq: undefined,
				transientError: null,
				retryState: null,
			}));
		},
		resetTransientState: () => {
			if (
				state.streamState === null &&
				state.transientError === null &&
				state.retryState === null &&
				state.reconnectState === null
			) {
				return;
			}
			setState((current) => ({
				...current,
				streamState: null,
				previewConnectionEpoch: current.previewConnectionEpoch + 1,
				previewHistoryVersion: undefined,
				previewGenerationAttempt: undefined,
				previewLastSeq: undefined,
				transientError: null,
				retryState: null,
				reconnectState: null,
			}));
		},
	};
};

export const selectStreamState = (state: ChatStreamStoreState) =>
	state.streamState;
export const selectHasStreamState = (state: ChatStreamStoreState) =>
	state.streamState !== null;
export const selectTransientError = (state: ChatStreamStoreState) =>
	state.transientError;
export const selectRetryState = (state: ChatStreamStoreState) =>
	state.retryState;
export const selectReconnectState = (state: ChatStreamStoreState) =>
	state.reconnectState;

export const isAwaitingFirstStreamChunk = (
	status: TypesGen.ChatStatus | null,
	streamState: StreamState | null,
	latestMessage: TypesGen.ChatMessage | undefined,
): boolean => {
	const latestMessageNeedsAssistantResponse =
		!latestMessage || latestMessage.role !== "assistant";
	if (streamState !== null || !latestMessageNeedsAssistantResponse) {
		return false;
	}
	return status === "running";
};

export const useChatSelector = <T>(
	store: ChatStreamStore,
	selector: (state: ChatStreamStoreState) => T,
): T => {
	const getSnapshot = () => selector(store.getSnapshot());
	return useSyncExternalStore(store.subscribe, getSnapshot, getSnapshot);
};

export { useChatStreamStore } from "./useChatStreamStore";
