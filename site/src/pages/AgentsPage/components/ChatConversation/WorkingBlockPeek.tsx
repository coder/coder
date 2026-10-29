// PROTOTYPE: shows a collapsed live working block's newest steps under its
// summary row, sliding up as steps arrive. Not production code.
import {
	AnimatePresence,
	MotionConfig,
	motion,
	type Transition,
} from "motion/react";
import { useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { useMessageScrollerUserLayoutIntent } from "#/vendor/message-scroller";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { Tool } from "../ChatElements/tools/Tool";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import { ReasoningDisclosure } from "./MessageText";
import { getVisibleContent } from "./messageHelpers";
import type { MergedTool, RenderBlock } from "./types";

type PeekItem =
	| { kind: "thinking"; key: string; text: string; isStreaming: boolean }
	| { kind: "tool"; key: string; tool: MergedTool };

type PeekSource = {
	blocks: readonly RenderBlock[];
	tools: readonly MergedTool[];
	isStreaming: boolean;
};

/**
 * A block's reasoning and tool calls, oldest first. Tools key by call ID and
 * reasoning by the tool before it, so a step keeps its key, and its row, when
 * the live stream persists as a message.
 */
export const collectPeekItems = (
	sources: readonly PeekSource[],
): PeekItem[] => {
	// A step briefly in both the stream and its persisted message renders once,
	// at its newest position.
	const byKey = new Map<string, PeekItem>();
	const put = (item: PeekItem) => {
		byKey.delete(item.key);
		byKey.set(item.key, item);
	};
	let anchor = "start";
	let sinceAnchor = 0;
	for (const source of sources) {
		const { visibleBlocks, visibleTools } = getVisibleContent(
			source.blocks,
			source.tools,
		);
		const toolById = new Map(visibleTools.map((tool) => [tool.id, tool]));
		for (const [index, block] of visibleBlocks.entries()) {
			if (block.type === "thinking") {
				put({
					kind: "thinking",
					key: `thinking:${anchor}:${sinceAnchor++}`,
					text: block.text,
					isStreaming: source.isStreaming && index === visibleBlocks.length - 1,
				});
			} else if (block.type === "tool") {
				const tool = toolById.get(block.id);
				if (tool) {
					put({ kind: "tool", key: `tool:${tool.id}`, tool });
					anchor = tool.id;
					sinceAnchor = 0;
				}
			}
		}
	}
	return [...byKey.values()];
};

const PEEK_ROWS = 2;
const ROW_HEIGHT = 24;
const ROW_GAP = 8;
const ROW_PITCH = ROW_HEIGHT + ROW_GAP;

const rowsHeight = (count: number) =>
	count === 0 ? 0 : count * ROW_HEIGHT + (count - 1) * ROW_GAP;

/**
 * The window reaches this far past its rows at both edges and fades out
 * across that band, so a row dissolves as it slides past an edge while rows
 * at rest stay at full contrast. The top band starts at the summary row's
 * bottom edge, below its glyphs, so a leaving row never shows under them.
 */
const EDGE = 12;
/** The gap between the summary row and the rail, as the expanded block has. */
const RAIL_GAP = 6;
const edgeMask = `linear-gradient(to bottom, transparent, #000 ${EDGE}px, #000 calc(100% - ${EDGE}px), transparent)`;

/**
 * Every moving part shares one curve that lands, so rows and the window
 * arrive together without a creeping tail.
 */
export const slide = {
	duration: 0.3,
	ease: [0.2, 0.8, 0.2, 1],
} satisfies Transition;

type ToolContext = {
	organizationId: string;
	mcpServers: readonly TypesGen.MCPServerConfig[];
	subagentTitles: Map<string, string>;
	subagentVariants: Map<string, SubagentVariant>;
};

const PeekRow: React.FC<{ item: PeekItem } & ToolContext> = ({
	item,
	organizationId,
	mcpServers,
	subagentTitles,
	subagentVariants,
}) => {
	if (item.kind === "thinking") {
		return (
			<ReasoningDisclosure
				id={`peek-${item.key}`}
				text={item.text}
				isStreaming={item.isStreaming}
			/>
		);
	}
	const { tool } = item;
	return (
		<Tool
			organizationId={organizationId}
			name={tool.name}
			args={tool.args}
			result={tool.result}
			status={tool.status}
			isError={tool.isError}
			isMedia={tool.isMedia}
			killedBySignal={tool.killedBySignal}
			shellToolDisplayMode="always_collapsed"
			codeDiffDisplayMode="always_collapsed"
			subagentTitles={subagentTitles}
			subagentVariants={subagentVariants}
			showDesktopPreviews={false}
			mcpServerConfigId={tool.mcpServerConfigId}
			mcpServers={mcpServers}
			modelIntent={tool.modelIntent}
			parsedCommands={tool.parsedCommands}
			startedAt={tool.startedAt}
			providerExecuted={tool.providerExecuted}
			foundPages={tool.foundPages}
		/>
	);
};

export type WorkingBlockPeekProps = ToolContext & {
	items: readonly PeekItem[];
	onExpand: () => void;
};

/**
 * The newest steps of a collapsed live block. Rows sit in fixed slots on one
 * track, and a new step lands below the window while the track slides up by
 * a slot, so arriving, staying, and leaving rows move as one. Rows never fade
 * on a timer: the window's soft edges reveal and hide them by position.
 */
export const WorkingBlockPeek: React.FC<WorkingBlockPeekProps> = ({
	items,
	onExpand,
	...ctx
}) => {
	const shown = items.slice(-PEEK_ROWS);

	// Slots are handed out in arrival order and never reused, so a row keeps
	// its slot from the moment it appears until it has faded out above.
	const [slots, setSlots] = useState<ReadonlyMap<string, number>>(
		() => new Map(shown.map((item, index) => [item.key, index])),
	);
	const unslotted = shown.filter((item) => !slots.has(item.key));
	if (unslotted.length > 0) {
		let next = Math.max(-1, ...slots.values()) + 1;
		const nextSlots = new Map(slots);
		for (const item of unslotted) {
			nextSlots.set(item.key, next++);
		}
		setSlots(nextSlots);
	}
	const slotOf = (item: PeekItem) => slots.get(item.key) ?? 0;
	// The window follows the newest row, so it always shows.
	const newestSlot = Math.max(0, ...shown.map(slotOf));
	const trackOffset = -(newestSlot - (shown.length - 1)) * ROW_PITCH;

	const userLayoutIntent = useMessageScrollerUserLayoutIntent();
	if (shown.length === 0) {
		return null;
	}

	return (
		<MotionConfig reducedMotion="user">
			{/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events -- A mouse shortcut; the summary row expands the block from the keyboard. */}
			<div
				onClick={(event) => {
					userLayoutIntent(event.currentTarget, { revealEnd: true });
					onExpand();
				}}
				className="ml-2 flex cursor-pointer flex-col border-0 border-l border-solid border-border pl-4 hover:[&_.text-content-secondary]:text-content-primary"
				style={{ marginTop: RAIL_GAP }}
			>
				<motion.div
					data-peek-window=""
					className="relative overflow-hidden"
					style={{
						marginTop: -RAIL_GAP,
						maskImage: edgeMask,
						WebkitMaskImage: edgeMask,
					}}
					initial={false}
					animate={{ height: rowsHeight(shown.length) + 2 * EDGE }}
					transition={slide}
				>
					<motion.div
						className="absolute inset-x-0"
						style={{ top: EDGE }}
						initial={false}
						animate={{ y: trackOffset }}
						transition={slide}
					>
						<ToolCall.SummaryProvider>
							<AnimatePresence initial={false}>
								{shown.map((item) => (
									<motion.div
										key={item.key}
										data-peek-row={item.key}
										className="absolute inset-x-0 h-6 overflow-hidden"
										style={{ top: slotOf(item) * ROW_PITCH }}
										// Stays mounted until the track has carried it past
										// the top edge.
										exit={{
											opacity: 0,
											transition: { delay: slide.duration, duration: 0 },
										}}
									>
										<PeekRow item={item} {...ctx} />
									</motion.div>
								))}
							</AnimatePresence>
						</ToolCall.SummaryProvider>
					</motion.div>
				</motion.div>
			</div>
		</MotionConfig>
	);
};
