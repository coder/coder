import type * as TypesGen from "#/api/typesGenerated";
import type { ChatDetailError } from "./chatError";
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
	| ({ phase: "interrupting" } & LiveStatusBase)
	| ({
			phase: "retrying";
			title: string;
			kind: TypesGen.ChatErrorKind;
			message: string;
			attempt: number;
			provider?: string;
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
			kind: TypesGen.ChatErrorKind;
			message: string;
			detail?: string;
			provider?: string;
			statusCode?: number;
	  } & LiveStatusBase);

export const shouldRenderLiveAssistant = (
	liveStatus: LiveStatusModel,
): boolean =>
	liveStatus.phase === "streaming" ||
	liveStatus.phase === "starting" ||
	liveStatus.phase === "interrupting" ||
	liveStatus.phase === "retrying" ||
	liveStatus.phase === "reconnecting" ||
	liveStatus.hasAccumulatedOutput;

export type DeriveLiveStatusParams = {
	streamState: StreamState | null;
	retryState: RetryState | null;
	reconnectState: ReconnectState | null;
	streamError: ChatDetailError | null;
	persistedError: ChatDetailError | null;
	isAwaitingFirstStreamChunk: boolean;
	chatStatus: TypesGen.ChatStatus | null;
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

const toRetryingLiveStatus = (
	retryState: RetryState,
	options: { hasAccumulatedOutput?: boolean } = {},
): Extract<LiveStatusModel, { phase: "retrying" }> => ({
	phase: "retrying",
	hasAccumulatedOutput: options.hasAccumulatedOutput ?? false,
	title: getErrorTitle(retryState.kind, "retry"),
	kind: retryState.kind,
	message: retryState.error,
	attempt: retryState.attempt,
	provider: retryState.provider,
	retryingAt: retryState.retryingAt,
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
	...(error.detail ? { detail: error.detail } : {}),
	provider: error.provider,
	statusCode: error.statusCode,
});

export const deriveLiveStatus = ({
	streamState,
	retryState,
	reconnectState,
	streamError,
	persistedError,
	isAwaitingFirstStreamChunk,
	chatStatus,
}: DeriveLiveStatusParams): LiveStatusModel => {
	const hasAccumulatedOutput = getHasAccumulatedOutput(streamState);

	if (retryState) {
		return toRetryingLiveStatus(retryState, { hasAccumulatedOutput });
	}

	if (streamError) {
		// The error handler clears the stream, so output and an error
		// only coexist when a stale part repopulates the stream after
		// the clear. Suppress that leftover so it does not render as a
		// live row under the callout.
		return toFailedLiveStatus(streamError, { hasAccumulatedOutput: false });
	}

	if (reconnectState) {
		return toReconnectingLiveStatus(reconnectState, { hasAccumulatedOutput });
	}

	// The interrupt outranks stream leftovers: while the worker drains and
	// finalizes an interruption, the transcript must not claim the agent is
	// still producing output.
	if (chatStatus === "interrupting") {
		return { phase: "interrupting", hasAccumulatedOutput };
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
