import { MessageScroller } from "@shadcn/react/message-scroller";
import { Link } from "react-router";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
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

// Why this gate matters: the scroller looks for newly appended anchor rows only
// at the tail of its child list. A permanently mounted live-tail row would sit
// after every committed turn, so each new user message lands before it and is
// never detected as the new anchor, which stops it from scrolling to the top.
// Returning false here unmounts the row while idle so appended turns stay at the
// tail where the scroller can anchor them.
const hasLiveStreamTailContent = (
	liveStatus: LiveStatusModel,
	isTranscriptEmpty: boolean,
): boolean =>
	(isTranscriptEmpty && liveStatus.phase === "idle") ||
	shouldRenderStreamingSection(liveStatus) ||
	liveStatus.phase === "failed";

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
	if (!hasLiveStreamTailContent(liveStatus, isTranscriptEmpty)) {
		return null;
	}

	const shouldRenderStreamSection = shouldRenderStreamingSection(liveStatus);
	const terminalStatus = liveStatus.phase === "failed" ? liveStatus : null;
	const usageLimitStatus =
		terminalStatus?.kind === "usage_limit" ? terminalStatus : null;
	const shouldRenderEmptyState =
		isTranscriptEmpty && liveStatus.phase === "idle";

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
	persistedError: ChatDetailError | undefined;
	isTranscriptEmpty: boolean;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const LiveStreamTail = ({
	store,
	persistedError,
	isTranscriptEmpty,
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
	const streamTools = buildStreamTools(
		streamState?.toolCalls,
		streamState?.toolResults,
	);
	const liveStatus = deriveLiveStatus({
		streamState,
		retryState,
		reconnectState,
		streamError,
		persistedError: persistedError ?? null,
		isAwaitingFirstStreamChunk,
	});

	// The row owns its MessageScroller.Item so an idle tail leaves no trailing
	// placeholder. It carries no scrollAnchor, so the preceding user turn stays
	// anchored while the reply streams in below it.
	if (!hasLiveStreamTailContent(liveStatus, isTranscriptEmpty)) {
		return null;
	}

	return (
		<MessageScroller.Item messageId="__live_stream__">
			<LiveStreamTailContent
				isTranscriptEmpty={isTranscriptEmpty}
				streamState={streamState}
				streamTools={streamTools}
				liveStatus={liveStatus}
				subagentTitles={subagentTitles}
				subagentVariants={subagentVariants}
				subagentStatusOverrides={subagentStatusOverrides}
				urlTransform={urlTransform}
				mcpServers={mcpServers}
			/>
		</MessageScroller.Item>
	);
};
