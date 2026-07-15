import { Link } from "react-router";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ChatStatusCallout } from "./ChatStatusCallout";
import {
	isAwaitingFirstStreamChunk,
	selectReconnectState,
	selectRetryState,
	selectStreamState,
	selectTransientError,
	useChatSelector,
	type useChatStreamStore,
} from "./chatStreamStore";
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

type ChatStreamStoreHandle = ReturnType<typeof useChatStreamStore>["store"];

interface LiveStreamTailContentProps {
	isTranscriptEmpty: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
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
	store: ChatStreamStoreHandle;
	chatStatus: TypesGen.ChatStatus;
	actionRequired?: TypesGen.ChatStreamActionRequired;
	latestMessage: TypesGen.ChatMessage | undefined;
	persistedError: ChatDetailError | undefined;
	isTranscriptEmpty: boolean;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}

export const LiveStreamTail = ({
	store,
	chatStatus,
	actionRequired,
	latestMessage,
	persistedError,
	isTranscriptEmpty,
	subagentTitles,
	subagentVariants,
	urlTransform,
	mcpServers,
}: LiveStreamTailProps) => {
	const streamState = useChatSelector(store, selectStreamState);
	const transientError = useChatSelector(store, selectTransientError);
	const retryState = useChatSelector(store, selectRetryState);
	const reconnectState = useChatSelector(store, selectReconnectState);
	const awaitingFirstStreamChunk = isAwaitingFirstStreamChunk(
		chatStatus,
		streamState,
		latestMessage,
	);
	const streamTools = buildStreamTools(
		streamState?.toolCalls,
		streamState?.toolResults,
	);
	const unsupportedActionError: ChatDetailError | null = actionRequired
		? {
				kind: "generic",
				message:
					"This agent requested client-side tools that Coder Agents cannot execute yet.",
				detail: `${actionRequired.tool_calls.length} pending tool call${actionRequired.tool_calls.length === 1 ? "" : "s"}.`,
			}
		: null;
	const liveStatus = deriveLiveStatus({
		streamState,
		retryState,
		reconnectState,
		streamError: transientError,
		persistedError: unsupportedActionError ?? persistedError ?? null,
		isAwaitingFirstStreamChunk: awaitingFirstStreamChunk,
	});

	return (
		<LiveStreamTailContent
			isTranscriptEmpty={isTranscriptEmpty}
			streamState={streamState}
			streamTools={streamTools}
			liveStatus={liveStatus}
			subagentTitles={subagentTitles}
			subagentVariants={subagentVariants}
			urlTransform={urlTransform}
			mcpServers={mcpServers}
		/>
	);
};
