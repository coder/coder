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
 * True when the block list contains at least one response block
 * (final output text). Used in pinned mode to detect the
 * transition from thinking to responding.
 */
const hasResponseBlock = (blocks: readonly RenderBlock[]): boolean =>
	blocks.some((b) => b.type === "response");

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
		<Shimmer as="span" className="text-sm">
			Thinking
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
	isStreaming: boolean;
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
	isStreaming,
	subagentTitles,
	subagentVariants,
	subagentStatusOverrides,
	urlTransform,
	mcpServers,
	liveStatus,
	startingResetKey,
}) => {
	const scrollRef = useRef<HTMLDivElement>(null);
	const hasResponse = hasResponseBlock(blocks);

	// Split blocks: thinking blocks are stripped entirely (the
	// pinned indicator replaces them); response blocks render
	// outside the constrained container; everything else is
	// activity.
	const activityBlocks: RenderBlock[] = [];
	const responseBlocks: RenderBlock[] = [];
	let foundResponse = false;
	for (const block of blocks) {
		// Suppress thinking blocks; the pinned indicator is the
		// only "thinking" signal in this mode.
		if (block.type === "thinking") {
			continue;
		}
		if (block.type === "response") {
			foundResponse = true;
		}
		if (foundResponse && block.type === "response") {
			responseBlocks.push(block);
		} else {
			activityBlocks.push(block);
		}
	}

	// Auto-scroll the activity container to the bottom so the
	// latest thinking/tool content stays visible. useLayoutEffect
	// avoids a visible frame where content has grown but not
	// scrolled.
	const activityBlockCount = activityBlocks.length;
	useLayoutEffect(() => {
		if (activityBlockCount && scrollRef.current) {
			scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
		}
	}, [activityBlockCount]);

	// The pinned indicator shows during every active phase
	// (starting, streaming, retrying, reconnecting), not just
	// while chunks are arriving. It only disappears once response
	// text arrives or the phase goes idle.
	const isAgentWorking =
		liveStatus.phase !== "idle" && liveStatus.phase !== "failed";
	const showPinnedIndicator = isAgentWorking && !hasResponse;
	const showActivityContainer = activityBlocks.length > 0;

	// When thinking: fixed-height box so "Thinking" never moves.
	// Activity scrolls inside the top portion; the indicator is
	// anchored at the bottom of the fixed box.
	if (showPinnedIndicator) {
		return (
			<div className="flex h-48 flex-col">
				{/* Activity area: fills remaining space, scrolls internally */}
				<div
					ref={scrollRef}
					className="min-h-0 flex-1 overflow-y-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden"
					style={PINNED_FADE_MASK}
				>
					{showActivityContainer && (
						<div className="space-y-3">
							<BlockList
								blocks={activityBlocks}
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

				{/* Pinned indicator: always at the bottom of the fixed box */}
				<div className="shrink-0">
					<PinnedThinkingIndicator />
				</div>
			</div>
		);
	}

	// Once the agent starts responding, drop the fixed container.
	// Activity stays in a capped scroll area; response text renders
	// at full brightness below.
	return (
		<div className="space-y-3">
			{showActivityContainer && (
				<div
					ref={scrollRef}
					className="max-h-48 overflow-y-auto [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden"
				>
					<div className="space-y-3">
						<BlockList
							blocks={activityBlocks}
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
				</div>
			)}

			{/* Status callouts for starting/retrying/reconnecting */}
			{hasTransientLiveStatus(liveStatus) && !isStreaming && (
				<ChatStatusCallout
					status={liveStatus}
					startingResetKey={startingResetKey}
				/>
			)}

			{responseBlocks.length > 0 && (
				<BlockList
					blocks={responseBlocks}
					tools={streamTools}
					keyPrefix="stream-response"
					isStreaming={isStreaming}
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					subagentStatusOverrides={subagentStatusOverrides}
					urlTransform={urlTransform}
					mcpServers={mcpServers}
				/>
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
							isStreaming={isStreaming}
							subagentTitles={subagentTitles}
							subagentVariants={subagentVariants}
							subagentStatusOverrides={subagentStatusOverrides}
							urlTransform={urlTransform}
							mcpServers={mcpServers}
							liveStatus={liveStatus}
							startingResetKey={startingResetKey}
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
