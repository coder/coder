import { Link } from "react-router";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ChatStatusCallout } from "./ChatStatusCallout";
import {
	resolveOverlayStreamState,
	selectFinalizingStreamState,
	selectReconnectState,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	type useChatStore,
} from "./chatStore";
import { useIsAwaitingFirstStreamChunk } from "./durableChat";
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

interface LiveStreamTailContentProps {
	isTranscriptEmpty: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
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
			className={
				isTranscriptEmpty
					? "flex flex-col gap-2"
					: "mt-2 flex flex-col gap-2 empty:mt-0"
			}
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
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					subagentStatusOverrides={subagentStatusOverrides}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
				/>
			)}
			{usageLimitStatus && !usageLimitStatus.provider ? (
				<Alert
					severity="info"
					actions={
						<Button asChild size="sm">
							<Link to="/agents/analytics">View usage</Link>
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
	chatId?: string;
	persistedError: ChatDetailError | undefined;
	isTranscriptEmpty: boolean;
	// Decided by the durable-reading parent, which reads the finalizing ID and
	// the cache-backed message list in the same render. True once the finalized
	// tail is readable from the durable list, so the overlay can drop it.
	suppressFinalizedOverlay?: boolean;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const LiveStreamTail = ({
	store,
	chatId,
	persistedError,
	isTranscriptEmpty,
	suppressFinalizedOverlay = false,
	subagentTitles,
	subagentVariants,
	urlTransform,
	mcpServers,
}: LiveStreamTailProps) => {
	const streamState = useChatSelector(store, selectStreamState);
	const finalizingStreamState = useChatSelector(
		store,
		selectFinalizingStreamState,
	);
	// The finalizing snapshot bridges the handoff: the store notifies
	// synchronously and the query cache a macrotask later, so dropping the
	// overlay on the store notification alone would blank the tail for a frame.
	const overlayStreamState = resolveOverlayStreamState(
		streamState,
		finalizingStreamState,
		suppressFinalizedOverlay,
	);
	const streamError = useChatSelector(store, selectStreamError);
	const retryState = useChatSelector(store, selectRetryState);
	const reconnectState = useChatSelector(store, selectReconnectState);
	const isAwaitingFirstStreamChunk = useIsAwaitingFirstStreamChunk({
		store,
		chatId,
	});
	const subagentStatusOverrides = useChatSelector(
		store,
		selectSubagentStatusOverrides,
	);
	const streamTools = buildStreamTools(
		overlayStreamState?.toolCalls,
		overlayStreamState?.toolResults,
	);
	const liveStatus = deriveLiveStatus({
		streamState: overlayStreamState,
		retryState,
		reconnectState,
		streamError,
		persistedError: persistedError ?? null,
		isAwaitingFirstStreamChunk,
	});

	return (
		<LiveStreamTailContent
			isTranscriptEmpty={isTranscriptEmpty}
			streamState={overlayStreamState}
			streamTools={streamTools}
			liveStatus={liveStatus}
			subagentTitles={subagentTitles}
			subagentVariants={subagentVariants}
			subagentStatusOverrides={subagentStatusOverrides}
			urlTransform={urlTransform}
			mcpServers={mcpServers}
		/>
	);
};
