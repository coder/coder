import { Link } from "react-router";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import {
	selectIsAwaitingFirstStreamChunk,
	selectReconnectState,
	selectRetryState,
	selectStreamError,
	selectStreamState,
	selectSubagentStatusOverrides,
	useChatSelector,
	type useChatStore,
} from "./ChatContext";
import { ChatStatusCallout } from "./ChatStatusCallout";
import { StreamingOutput } from "./ConversationTimeline";
import { deriveLiveStatus, type LiveStatusModel } from "./liveStatusModel";
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
	startingResetKey?: string;
	subagentTitles: Map<string, string>;
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
	subagentStatusOverrides,
	urlTransform,
	mcpServers,
}: LiveStreamTailContentProps) => {
	const shouldRenderStreamSection = shouldRenderStreamingSection(liveStatus);
	const terminalStatus = liveStatus.phase === "failed" ? liveStatus : null;
	const usageLimitStatus =
		terminalStatus?.kind === "usage-limit" ? terminalStatus : null;
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
		<div className="flex flex-col gap-3">
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
					subagentStatusOverrides={subagentStatusOverrides}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
				/>
			)}
			{usageLimitStatus ? (
				<Alert
					severity="info"
					className="py-2"
					actions={
						<Button asChild variant="subtle" size="sm">
							<Link to="/agents/analytics">View Usage</Link>
						</Button>
					}
				>
					{usageLimitStatus.message}
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
	subagentTitles: Map<string, string>;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const LiveStreamTail = ({
	store,
	persistedError,
	isTranscriptEmpty,
	startingResetKey,
	subagentTitles,
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
	const streamTools = buildStreamTools(streamState);
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
			startingResetKey={startingResetKey}
			subagentTitles={subagentTitles}
			subagentStatusOverrides={subagentStatusOverrides}
			urlTransform={urlTransform}
			mcpServers={mcpServers}
		/>
	);
};
