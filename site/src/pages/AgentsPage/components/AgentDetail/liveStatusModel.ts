import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { getErrorTitle } from "./chatStatusHelpers";
import type { ReconnectState, RetryState, StreamState } from "./types";

type LiveStatusBase = {
	hasAccumulatedOutput: boolean;
};

const RECONNECTING_TITLE = "Reconnecting";
const RECONNECTING_MESSAGE = "Chat stream disconnected. Reconnecting…";

export type LiveStatusModel =
	| ({ phase: "idle" } & LiveStatusBase)
	| ({ phase: "starting" } & LiveStatusBase)
	| ({ phase: "streaming" } & LiveStatusBase)
	| ({
			phase: "retrying";
			title: string;
			kind: string;
			message: string;
			attempt: number;
			provider?: string;
			delayMs?: number;
			retryingAt?: string;
	  } & LiveStatusBase)
	| ({
			phase: "reconnecting";
			title: string;
			message: string;
			attempt: number;
			delayMs: number;
			retryingAt: string;
	  } & LiveStatusBase)
	| ({
			phase: "failed";
			title: string;
			kind: string;
			message: string;
			provider?: string;
			retryable?: boolean;
			statusCode?: number;
	  } & LiveStatusBase);

export type DeriveLiveStatusParams = {
	streamState: StreamState | null;
	retryState: RetryState | null;
	reconnectState: ReconnectState | null;
	streamError: ChatDetailError | null;
	persistedError: ChatDetailError | null;
	isAwaitingFirstStreamChunk: boolean;
};

const getHasAccumulatedOutput = (streamState: StreamState | null): boolean =>
	Boolean(streamState && streamState.blocks.length > 0);

const toReconnectingLiveStatus = (
	reconnectState: ReconnectState,
	options: { hasAccumulatedOutput?: boolean } = {},
): Extract<LiveStatusModel, { phase: "reconnecting" }> => ({
	phase: "reconnecting",
	hasAccumulatedOutput: options.hasAccumulatedOutput ?? false,
	title: RECONNECTING_TITLE,
	message: RECONNECTING_MESSAGE,
	...reconnectState,
});

const toFailedLiveStatus = (
	error: ChatDetailError,
	options: { hasAccumulatedOutput?: boolean } = {},
): Extract<LiveStatusModel, { phase: "failed" }> => ({
	phase: "failed",
	hasAccumulatedOutput: options.hasAccumulatedOutput ?? false,
	title: getErrorTitle(error.kind, "error"),
	kind: error.kind,
	message: error.message,
	provider: error.provider,
	retryable: error.retryable,
	statusCode: error.statusCode,
});

export const deriveLiveStatus = ({
	streamState,
	retryState,
	reconnectState,
	streamError,
	persistedError,
	isAwaitingFirstStreamChunk,
}: DeriveLiveStatusParams): LiveStatusModel => {
	const hasAccumulatedOutput = getHasAccumulatedOutput(streamState);

	if (retryState) {
		return {
			phase: "retrying",
			hasAccumulatedOutput,
			title: getErrorTitle(retryState.kind, "retry"),
			kind: retryState.kind,
			message: retryState.error,
			attempt: retryState.attempt,
			provider: retryState.provider,
			delayMs: retryState.delayMs,
			retryingAt: retryState.retryingAt,
		};
	}

	if (streamError) {
		return toFailedLiveStatus(streamError, { hasAccumulatedOutput });
	}

	if (reconnectState) {
		return toReconnectingLiveStatus(reconnectState, { hasAccumulatedOutput });
	}

	if (isAwaitingFirstStreamChunk) {
		return { phase: "starting", hasAccumulatedOutput };
	}

	if (streamState !== null) {
		return { phase: "streaming", hasAccumulatedOutput };
	}

	if (persistedError) {
		return toFailedLiveStatus(persistedError, { hasAccumulatedOutput });
	}

	return { phase: "idle", hasAccumulatedOutput };
};
