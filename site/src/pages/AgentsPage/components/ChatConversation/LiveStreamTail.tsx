import { useEffect, useState } from "react";
import { Link } from "react-router";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { LIVE_STREAM_TAIL_ANCHOR_ID } from "../chatViewportUtils";
import { ChatStatusCallout } from "./ChatStatusCallout";
import {
	selectIsAwaitingFirstStreamChunk,
	selectReconnectState,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	type useChatStore,
} from "./chatStore";
import { deriveLiveStatus, type LiveStatusModel } from "./liveStatusModel";
import { StreamingOutput } from "./StreamingOutput";
import { buildStreamTools } from "./streamState";
import type { MergedTool, StreamState } from "./types";

const shouldRenderStreamingSection = (liveStatus: LiveStatusModel): boolean =>
	liveStatus.phase === "streaming" ||
	liveStatus.phase === "starting" ||
	liveStatus.phase === "retrying" ||
	liveStatus.phase === "reconnecting" ||
	liveStatus.hasAccumulatedOutput;

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

type LatchedStreamState = {
	contextKey: string;
	streamState: StreamState;
};

const hasRenderableStreamState = (streamState: StreamState | null): boolean => {
	if (!streamState) {
		return false;
	}
	return (
		streamState.blocks.length > 0 ||
		Object.keys(streamState.toolCalls).length > 0 ||
		Object.keys(streamState.toolResults).length > 0 ||
		streamState.sources.length > 0
	);
};

interface LiveStreamTailContentProps {
	isTranscriptEmpty: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
	startingResetKey?: string;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const LiveStreamTailContent = ({
	isTranscriptEmpty,
	streamState,
	streamTools,
	liveStatus,
	startingResetKey,
	subagentTitles,
	subagentVariants,
	subagentStatusOverrides,
	urlTransform,
	mcpServers,
}: LiveStreamTailContentProps) => {
	const shouldRenderStreamSection = shouldRenderStreamingSection(liveStatus);
	const terminalStatus = liveStatus.phase === "failed" ? liveStatus : null;
	const usageLimitStatus =
		terminalStatus?.kind === "usage_limit" ? terminalStatus : null;
	const shouldRenderEmptyState =
		isTranscriptEmpty && liveStatus.phase === "idle";

	if (
		!shouldRenderEmptyState &&
		!shouldRenderStreamSection &&
		!terminalStatus
	) {
		return null;
	}

	return (
		<div
			data-chat-anchor="true"
			data-chat-anchor-id={LIVE_STREAM_TAIL_ANCHOR_ID}
			className="flex flex-col gap-2"
		>
			{shouldRenderEmptyState && (
				<div className="py-12 text-center text-content-secondary">
					<p className="text-sm">Start a conversation with your agent.</p>
				</div>
			)}
			{shouldRenderStreamSection && (
				<StreamingOutput
					streamState={streamState}
					streamTools={streamTools}
					liveStatus={liveStatus}
					startingResetKey={startingResetKey}
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					subagentStatusOverrides={subagentStatusOverrides}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
				/>
			)}
			{usageLimitStatus ? (
				<Alert
					severity="info"
					actions={
						<Button asChild size="sm">
							<Link to="/agents/analytics">View Usage</Link>
						</Button>
					}
				>
					<AlertDescription>{usageLimitStatus.message}</AlertDescription>
				</Alert>
			) : terminalStatus ? (
				<ChatStatusCallout status={terminalStatus} />
			) : null}
		</div>
	);
};

interface LiveStreamTailProps {
	store: ChatStoreHandle;
	persistedError: ChatDetailError | undefined;
	isTranscriptEmpty: boolean;
	startingResetKey?: string;
	tailContextKey: string;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const LiveStreamTail = ({
	store,
	persistedError,
	isTranscriptEmpty,
	startingResetKey,
	tailContextKey,
	subagentTitles,
	subagentVariants,
	urlTransform,
	mcpServers,
}: LiveStreamTailProps) => {
	const streamState = useChatSelector(store, selectStreamState);
	const streamError = useChatSelector(store, selectStreamError);
	const retryState = useChatSelector(store, selectRetryState);
	const reconnectState = useChatSelector(store, selectReconnectState);
	const isAwaitingFirstStreamChunk = useChatSelector(
		store,
		selectIsAwaitingFirstStreamChunk,
	);
	const subagentStatusOverrides = useChatSelector(
		store,
		selectSubagentStatusOverrides,
	);
	const [latchedStreamState, setLatchedStreamState] =
		useState<LatchedStreamState | null>(null);
	const liveStatus = deriveLiveStatus({
		streamState,
		retryState,
		reconnectState,
		streamError,
		persistedError: persistedError ?? null,
		isAwaitingFirstStreamChunk,
	});

	useEffect(() => {
		if (hasRenderableStreamState(streamState) && streamState) {
			const nextStreamState = streamState;
			setLatchedStreamState((current) => {
				if (
					current?.contextKey === tailContextKey &&
					current.streamState === nextStreamState
				) {
					return current;
				}
				return { contextKey: tailContextKey, streamState: nextStreamState };
			});
			return;
		}
		if (latchedStreamState?.contextKey !== tailContextKey) {
			setLatchedStreamState(null);
		}
	}, [latchedStreamState, streamState, tailContextKey]);

	const effectiveStreamState =
		streamState ??
		(latchedStreamState?.contextKey === tailContextKey
			? latchedStreamState.streamState
			: null);
	const streamTools = buildStreamTools(
		effectiveStreamState?.toolCalls,
		effectiveStreamState?.toolResults,
	);
	const effectiveLiveStatus =
		effectiveStreamState && liveStatus.phase === "starting"
			? { phase: "streaming" as const, hasAccumulatedOutput: true }
			: effectiveStreamState && !liveStatus.hasAccumulatedOutput
				? { ...liveStatus, hasAccumulatedOutput: true }
				: liveStatus;

	return (
		<LiveStreamTailContent
			isTranscriptEmpty={isTranscriptEmpty}
			streamState={effectiveStreamState}
			streamTools={streamTools}
			liveStatus={effectiveLiveStatus}
			startingResetKey={startingResetKey}
			subagentTitles={subagentTitles}
			subagentVariants={subagentVariants}
			subagentStatusOverrides={subagentStatusOverrides}
			urlTransform={urlTransform}
			mcpServers={mcpServers}
		/>
	);
};
