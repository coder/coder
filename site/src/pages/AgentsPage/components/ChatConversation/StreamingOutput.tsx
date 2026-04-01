import type { FC } from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import { ConversationItem, Message, MessageContent } from "../ChatElements";
import { ChatStatusCallout } from "./ChatStatusCallout";
import { BlockList } from "./ConversationTimeline";
import type { LiveStatusModel } from "./liveStatusModel";
import type { MergedTool, RenderBlock, StreamState } from "./types";

const hasTransientLiveStatus = (liveStatus: LiveStatusModel): boolean =>
	liveStatus.phase === "starting" ||
	liveStatus.phase === "retrying" ||
	liveStatus.phase === "reconnecting";

/**
 * True when the block list contains at least one text or reasoning
 * block. Tool-call and other non-text blocks don't count because
 * they don't replace the "Thinking..." placeholder visually.
 */
const hasTextOrReasoningBlock = (blocks: readonly RenderBlock[]): boolean =>
	blocks.some((b) => b.type === "response" || b.type === "thinking");

export const StreamingOutput: FC<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	computerUseSubagentIds?: Set<string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	liveStatus: LiveStatusModel;
	startingResetKey?: string;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}> = ({
	streamState,
	streamTools,
	subagentTitles,
	computerUseSubagentIds,
	subagentStatusOverrides,
	liveStatus,
	startingResetKey,
	urlTransform,
	mcpServers,
}) => {
	if (liveStatus.phase === "idle") {
		return null;
	}

	const isStreaming = liveStatus.phase === "streaming";
	const shouldShowBlocks =
		liveStatus.phase === "streaming" || liveStatus.hasAccumulatedOutput;
	const blocks = shouldShowBlocks ? (streamState?.blocks ?? []) : [];

	// During streaming, keep showing the "Thinking..." indicator
	// until text or reasoning blocks arrive. This bridges the
	// visual gap between the "starting" phase placeholder and the
	// first visible content, preventing the indicator from
	// flickering away when only tool-call parts (or whitespace-
	// only text deltas) have been received so far.
	const needsStreamingThinking =
		isStreaming && !hasTextOrReasoningBlock(blocks);

	const shouldShowStatusCallout =
		hasTransientLiveStatus(liveStatus) || needsStreamingThinking;

	if (!shouldShowBlocks && !shouldShowStatusCallout) {
		return null;
	}

	// When we need the thinking indicator during streaming, present
	// a synthetic "starting" status so ChatStatusCallout renders
	// the shimmer placeholder rather than returning null.
	const calloutStatus: LiveStatusModel = needsStreamingThinking
		? {
				phase: "starting",
				hasAccumulatedOutput: liveStatus.hasAccumulatedOutput,
			}
		: liveStatus;

	const conversationItemProps = { role: "assistant" as const };

	return (
		<ConversationItem {...conversationItemProps}>
			<Message className="w-full">
				<MessageContent className="whitespace-normal">
					<div className="space-y-3">
						{shouldShowBlocks && (
							<BlockList
								blocks={blocks}
								tools={streamTools}
								keyPrefix="stream"
								isStreaming={isStreaming}
								subagentTitles={subagentTitles}
								computerUseSubagentIds={computerUseSubagentIds}
								subagentStatusOverrides={subagentStatusOverrides}
								urlTransform={urlTransform}
								mcpServers={mcpServers}
							/>
						)}
						{shouldShowStatusCallout && (
							<ChatStatusCallout
								status={calloutStatus}
								startingResetKey={startingResetKey}
							/>
						)}
					</div>
				</MessageContent>
			</Message>
		</ConversationItem>
	);
};
