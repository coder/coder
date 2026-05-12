import type { FC } from "react";
import { useQuery } from "react-query";
import type { UrlTransform } from "streamdown";
import { preferenceSettings } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";
import {
	ConversationItem,
	Message,
	MessageContent,
	Shimmer,
} from "../ChatElements";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
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
 * block. Tool-call blocks don't count; the placeholder should
 * remain visible between tool calls so the user knows the model
 * is still working.
 */
const hasTextOrReasoningBlock = (blocks: readonly RenderBlock[]): boolean =>
	blocks.some((b) => b.type === "response" || b.type === "thinking");

/** True when the stream contains visible response text for the user. */
export const hasResponseBlock = (blocks: readonly RenderBlock[]): boolean =>
	blocks.some((b) => b.type === "response");

/**
 * Placeholder shown during streaming before text or reasoning
 * blocks arrive.
 */
const StreamingThinkingPlaceholder: FC = () => (
	<div className="flex w-full items-center gap-2 py-0.5 text-content-secondary">
		<Shimmer as="span" className="text-[13px] leading-relaxed">
			Thinking...
		</Shimmer>
	</div>
);

/**
 * Persistent thinking indicator rendered by LiveStreamTailContent.
 * Shows at the bottom of the chat while the agent is working
 * and hasn't started writing response text yet.
 */
export const PinnedThinkingIndicator: FC = () => (
	<div className="flex w-full items-center gap-2 py-1 text-content-secondary">
		<Shimmer as="span" className="text-[13px] leading-relaxed">
			Thinking...
		</Shimmer>
	</div>
);

export const StreamingOutput: FC<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	liveStatus: LiveStatusModel;
	startingResetKey?: string;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
}> = ({
	streamState,
	streamTools,
	subagentTitles,
	subagentVariants,
	subagentStatusOverrides,
	liveStatus,
	startingResetKey,
	urlTransform,
	mcpServers,
}) => {
	const prefQuery = useQuery(preferenceSettings());
	const thinkingDisplayMode: ThinkingDisplayMode =
		prefQuery.data?.thinking_display_mode || "auto";

	if (liveStatus.phase === "idle") {
		return null;
	}

	const isStreaming = liveStatus.phase === "streaming";
	const shouldShowBlocks =
		liveStatus.phase === "streaming" || liveStatus.hasAccumulatedOutput;
	const blocks = shouldShowBlocks ? (streamState?.blocks ?? []) : [];

	const needsStreamingThinking =
		isStreaming && !hasTextOrReasoningBlock(blocks);

	const shouldShowStatusCallout =
		hasTransientLiveStatus(liveStatus) || needsStreamingThinking;

	if (!shouldShowBlocks && !shouldShowStatusCallout) {
		return null;
	}

	const conversationItemProps = { role: "assistant" as const };

	// In pinned mode, mute all internal activity (tool calls,
	// thinking) until the agent starts writing response text.
	// The "Thinking..." indicator lives in LiveStreamTailContent.
	const isPinned = thinkingDisplayMode === "pinned";
	const isMuted = isPinned && !hasResponseBlock(blocks);

	return (
		<ConversationItem {...conversationItemProps}>
			<Message className="w-full">
				<MessageContent className="whitespace-normal">
					<div
						className="space-y-3 transition-opacity duration-200"
						style={{ opacity: isMuted ? 0.45 : 1 }}
					>
						{shouldShowBlocks && (
							<BlockList
								blocks={blocks}
								tools={streamTools}
								keyPrefix="stream"
								isStreaming={isStreaming}
								subagentTitles={subagentTitles}
								subagentVariants={subagentVariants}
								subagentStatusOverrides={subagentStatusOverrides}
								urlTransform={urlTransform}
								mcpServers={mcpServers}
							/>
						)}
						{needsStreamingThinking && !isPinned && (
							<StreamingThinkingPlaceholder />
						)}
						{!needsStreamingThinking && hasTransientLiveStatus(liveStatus) && (
							<ChatStatusCallout
								status={liveStatus}
								startingResetKey={startingResetKey}
							/>
						)}
					</div>
				</MessageContent>
			</Message>
		</ConversationItem>
	);
};
