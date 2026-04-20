import type { FC } from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
import {
	ConversationItem,
	Message,
	MessageContent,
	Response,
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
 * block. Tool-call and other non-text blocks don't count because
 * they don't replace the "Thinking..." placeholder visually.
 */
const hasTextOrReasoningBlock = (blocks: readonly RenderBlock[]): boolean =>
	blocks.some((b) => b.type === "response" || b.type === "thinking");

/**
 * Stateless "Thinking..." shimmer used during the streaming phase
 * when no text or reasoning blocks have arrived yet. Unlike the
 * `StartingPlaceholder` in `ChatStatusCallout`, this has no
 * delayed-startup timer — the streaming phase is transient and
 * will be replaced as soon as real content arrives.
 */
const StreamingThinkingPlaceholder: FC = () => (
	<div className="relative">
		<Response aria-hidden className="invisible select-none">
			Thinking...
		</Response>
		<div className="pointer-events-none absolute inset-0 flex items-baseline gap-2">
			<Shimmer as="div" className="text-[13px] leading-relaxed">
				Thinking...
			</Shimmer>
		</div>
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
								subagentVariants={subagentVariants}
								subagentStatusOverrides={subagentStatusOverrides}
								urlTransform={urlTransform}
								mcpServers={mcpServers}
							/>
						)}
						{needsStreamingThinking && <StreamingThinkingPlaceholder />}
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
