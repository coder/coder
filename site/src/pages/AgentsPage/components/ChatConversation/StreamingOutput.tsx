import { type FC, useLayoutEffect, useRef } from "react";
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

/**
 * Placeholder shown during streaming before text or reasoning
 * blocks arrive. Uses the same shimmer animation as the
 * collapsible thinking disclosure label.
 */
const StreamingThinkingPlaceholder: FC = () => (
	<div className="flex w-full items-center gap-2 py-0.5 text-content-secondary">
		<Shimmer as="span" className="text-sm">
			Thinking
		</Shimmer>
	</div>
);

/**
 * Bottom-fade gradient mask for the pinned thinking container.
 * Content fades to transparent at the bottom, creating the effect
 * of thoughts materializing as they scroll upward.
 */
const PINNED_FADE_MASK = {
	maskImage:
		"linear-gradient(to bottom, black 0%, black calc(100% - 4em), transparent 100%)",
	WebkitMaskImage:
		"linear-gradient(to bottom, black 0%, black calc(100% - 4em), transparent 100%)",
} as const;

/**
 * Pinned thinking indicator shown at the bottom of the streaming
 * output. Stays fixed while activity streams above it.
 */
const PinnedThinkingIndicator: FC<{ fading?: boolean }> = ({
	fading = false,
}) => (
	<div
		className="flex w-full items-center gap-2 border-t border-border/50 pt-2 text-content-secondary transition-opacity duration-300"
		style={{ opacity: fading ? 0 : 1 }}
	>
		<Shimmer as="span" className="text-[13px] leading-relaxed">
			Thinking...
		</Shimmer>
	</div>
);

/**
 * Pinned mode wrapper: renders streaming blocks in a
 * height-constrained, bottom-scrolling container with a fade-out
 * gradient at the bottom. A persistent "Thinking" indicator sits
 * below the content area, staying in place while activity streams
 * above it.
 *
 * When the agent produces response text, the thinking indicator
 * fades out and the response block renders outside the constrained
 * container so it appears at full brightness.
 */
const PinnedStreamingContent: FC<{
	blocks: readonly RenderBlock[];
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	liveStatus: LiveStatusModel;
	startingResetKey?: string;
}> = ({
	blocks,
	streamTools,
	subagentTitles,
	subagentVariants,
	subagentStatusOverrides,
	urlTransform,
	mcpServers,
	liveStatus,
}) => {
	const scrollRef = useRef<HTMLDivElement>(null);
	const isStreaming = liveStatus.phase === "streaming";

	// Strip thinking blocks; the pinned indicator is the only
	// "thinking" signal in this mode. Everything else (response
	// text, tool calls, files) stays inside the fixed-height
	// scrolling area so nothing can push the indicator around.
	const visibleBlocks = blocks.filter((b) => b.type !== "thinking");

	// Auto-scroll the container to the bottom so the latest
	// content stays visible. useLayoutEffect avoids a frame
	// where content has grown but not scrolled.
	const blockCount = visibleBlocks.length;
	useLayoutEffect(() => {
		if (blockCount && scrollRef.current) {
			scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
		}
	}, [blockCount]);

	const isAgentWorking =
		liveStatus.phase !== "idle" && liveStatus.phase !== "failed";

	return (
		<div className="space-y-3">
			{/* Fixed-height box: all content scrolls inside, indicator
			    anchored at bottom. Height never changes so
			    "Thinking..." never moves. */}
			{isAgentWorking && (
				<div className="flex h-48 flex-col">
					<div
						ref={scrollRef}
						className="min-h-0 flex-1 overflow-y-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden"
						style={PINNED_FADE_MASK}
					>
						{visibleBlocks.length > 0 && (
							<div className="space-y-3">
								<BlockList
									blocks={visibleBlocks}
									tools={streamTools}
									keyPrefix="stream"
									isStreaming={isStreaming}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									subagentStatusOverrides={subagentStatusOverrides}
									urlTransform={urlTransform}
									mcpServers={mcpServers}
								/>
							</div>
						)}
					</div>
					<div className="shrink-0">
						<PinnedThinkingIndicator />
					</div>
				</div>
			)}
		</div>
	);
};

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

	// Pinned mode: use the dedicated pinned container layout.
	// Always use the fixed-height box so "Thinking" appears at
	// the same position from the very first frame.
	if (thinkingDisplayMode === "pinned") {
		return (
			<ConversationItem {...conversationItemProps}>
				<Message className="w-full">
					<MessageContent className="whitespace-normal">
						<PinnedStreamingContent
							blocks={blocks}
							streamTools={streamTools}
							subagentTitles={subagentTitles}
							subagentVariants={subagentVariants}
							subagentStatusOverrides={subagentStatusOverrides}
							urlTransform={urlTransform}
							mcpServers={mcpServers}
							liveStatus={liveStatus}
						/>
					</MessageContent>
				</Message>
			</ConversationItem>
		);
	}

	// Default mode: original layout
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
