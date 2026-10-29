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
 * at rest stay at full contrast. The top band sits in the gap above and the
 * empty bottom of the line above it, short of that line's glyphs.
 */
const EDGE = 12;
const edgeMask = `linear-gradient(to bottom, transparent, #000 ${EDGE}px, #000 calc(100% - ${EDGE}px), transparent)`;

/**
 * Every moving part shares one curve that lands, so rows, the window, and
 * the count arrive together without a creeping tail.
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

/** Rolls the old number up and out as the new one rolls in. */
const RollingCount: React.FC<{ value: number }> = ({ value }) => (
	<span className="relative inline-flex overflow-hidden tabular-nums">
		<AnimatePresence initial={false} mode="popLayout">
			<motion.span
				key={value}
				initial={{ y: "80%", opacity: 0 }}
				animate={{ y: 0, opacity: 1 }}
				exit={{ y: "-80%", opacity: 0 }}
				transition={slide}
			>
				{value}
			</motion.span>
		</AnimatePresence>
	</span>
);

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
	const hidden = items.slice(0, items.length - shown.length);
	const hiddenSteps = hidden.filter((item) => item.kind === "tool");
	const hiddenRunning = hiddenSteps.filter(
		(item) => item.kind === "tool" && item.tool.status === "running",
	).length;

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
			{/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events -- A mouse shortcut; the summary and earlier-steps buttons expand the block from the keyboard. */}
			<div
				onClick={(event) => {
					userLayoutIntent(event.currentTarget, { revealEnd: true });
					onExpand();
				}}
				className="ml-2 cursor-pointer border-0 border-l border-solid border-border pt-1.5 pl-4 hover:[&_.text-content-secondary]:text-content-primary"
			>
				<AnimatePresence initial={false}>
					{hiddenSteps.length > 0 && (
						<motion.div
							key="earlier"
							className="overflow-hidden"
							initial={{ height: 0, marginBottom: 0, opacity: 0 }}
							animate={{
								height: ROW_HEIGHT,
								marginBottom: ROW_GAP,
								opacity: 1,
							}}
							exit={{ height: 0, marginBottom: 0, opacity: 0 }}
							transition={slide}
						>
							<button
								type="button"
								aria-label={`Show ${hiddenSteps.length} earlier ${hiddenSteps.length === 1 ? "step" : "steps"}`}
								className="m-0 flex h-6 cursor-pointer items-center gap-1 border-0 bg-transparent p-0 font-[inherit] text-xs text-content-secondary transition-colors"
							>
								<RollingCount value={hiddenSteps.length} />
								<span>
									earlier {hiddenSteps.length === 1 ? "step" : "steps"}
									{hiddenRunning > 0 && `, ${hiddenRunning} still running`}
								</span>
							</button>
						</motion.div>
					)}
				</AnimatePresence>
				<motion.div
					data-peek-window=""
					className="relative overflow-hidden"
					style={{
						marginTop: -EDGE,
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
