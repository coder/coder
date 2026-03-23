import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { getErrorTitle } from "./chatStatusHelpers";
import type { RetryState, StreamState } from "./types";

type LiveStatusBase = {
	hasAccumulatedOutput: boolean;
};

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
	streamError: ChatDetailError | null;
	persistedError: ChatDetailError | null;
	isAwaitingFirstStreamChunk: boolean;
};

const getHasAccumulatedOutput = (streamState: StreamState | null): boolean =>
	Boolean(streamState && streamState.blocks.length > 0);

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
