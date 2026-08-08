import type * as TypesGen from "#/api/typesGenerated";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ChatStatusCallout } from "./ChatStatusCallout";
import type { ChatDetailError } from "./chatError";
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

interface LiveStreamTailContentProps {
	isTranscriptEmpty: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
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
	mcpServers,
}: LiveStreamTailContentProps) => {
	const shouldRenderStreamSection = shouldRenderStreamingSection(liveStatus);
	const terminalStatus = liveStatus.phase === "failed" ? liveStatus : null;
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
					mcpServers={mcpServers}
				/>
			)}
			{terminalStatus && <ChatStatusCallout status={terminalStatus} />}
		</div>
	);
};

interface LiveStreamTailProps {
	store: ChatStoreHandle;
	persistedError: ChatDetailError | undefined;
	isTranscriptEmpty: boolean;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const LiveStreamTail = ({
	store,
	persistedError,
	isTranscriptEmpty,
	subagentTitles,
	subagentVariants,
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

	return (
		<LiveStreamTailContent
			isTranscriptEmpty={isTranscriptEmpty}
			streamState={streamState}
			streamTools={streamTools}
			liveStatus={liveStatus}
			subagentTitles={subagentTitles}
			subagentVariants={subagentVariants}
			subagentStatusOverrides={subagentStatusOverrides}
			mcpServers={mcpServers}
		/>
	);
};
