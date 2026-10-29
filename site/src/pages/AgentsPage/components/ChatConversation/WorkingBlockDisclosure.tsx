import { ListTreeIcon } from "lucide-react";
import { AnimatePresence, animate, motion, usePresence } from "motion/react";
import { Component, useEffect, useRef, useState } from "react";
import { useTime } from "#/hooks/useTime";
import { humanDurationShort } from "#/utils/time";
import { useMessageScrollerLayoutSync } from "#/vendor/message-scroller";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import {
	slide,
	WorkingBlockPeek,
	type WorkingBlockPeekProps,
} from "./WorkingBlockPeek";
import type { WorkingBlock } from "./workingBlockGrouping";

/**
 * Whether older history joined the front of a block between two renders. A
 * block that only had its live row has no previous member, so the live row
 * becoming its persisted step is not a prepend.
 */
export const didPrependIntoBlock = (
	previousMemberIds: readonly number[],
	memberIds: readonly number[],
): boolean => {
	const previousFirst = previousMemberIds[0];
	return (
		previousFirst !== undefined &&
		memberIds[0] < previousFirst &&
		memberIds.includes(previousFirst)
	);
};

/**
 * A partial block may be missing earlier rows that are not loaded yet, so its
 * duration and counts are lower bounds.
 */
const atLeast = (block: WorkingBlock) => (block.isPartial ? "at least " : "");

const countLabel = (block: WorkingBlock, count: number, noun: string) =>
	`${count} ${noun}${count === 1 ? "" : "s"}${block.isPartial ? " or more" : ""}`;

const padTwo = (value: number) => value.toString().padStart(2, "0");

/** Fixed-width elapsed time, e.g. "19m 05s", so the label does not jiggle. */
export const formatLiveElapsed = (ms: number): string => {
	const totalSeconds = Math.round(Math.max(0, ms) / 1000);
	const hours = Math.floor(totalSeconds / 3600);
	const minutes = Math.floor((totalSeconds % 3600) / 60);
	const seconds = totalSeconds % 60;
	if (hours > 0) {
		return `${hours}h ${padTwo(minutes)}m`;
	}
	if (minutes > 0) {
		return `${minutes}m ${padTwo(seconds)}s`;
	}
	return `${seconds}s`;
};

type LiveLabelProps = { block: WorkingBlock };

const LiveLabel: React.FC<LiveLabelProps> = ({ block }) => {
	// Only the live block subscribes to a clock; completed blocks render a
	// fixed label, so long transcripts never tick.
	const now = useTime(() => Date.now());

	if (block.startedAt === undefined) {
		return <ToolCall.Label>Working</ToolCall.Label>;
	}

	const elapsed = formatLiveElapsed(now - block.startedAt);
	return (
		<ToolCall.Label className="tabular-nums">
			{`Working for ${atLeast(block)}${elapsed}`}
		</ToolCall.Label>
	);
};

const getCompletedWorkingLabel = (block: WorkingBlock): string => {
	const steps =
		block.stepCount > 0 ? countLabel(block, block.stepCount, "step") : "";
	if (block.startedAt === undefined || block.endedAt === undefined) {
		return steps ? `Completed ${steps}` : "Completed";
	}

	const duration = humanDurationShort(
		Math.max(0, block.endedAt - block.startedAt),
	);
	const worked = `Worked for ${atLeast(block)}${duration}`;
	return steps ? `${worked} (${steps})` : worked;
};

const getScrollParent = (element: HTMLElement): HTMLElement | null => {
	for (let node = element.parentElement; node; node = node.parentElement) {
		const { overflowY } = getComputedStyle(node);
		if (overflowY === "auto" || overflowY === "scroll") {
			return node;
		}
	}

	return null;
};

type WorkingBlockContentProps = {
	memberIds: readonly number[];
	children: React.ReactNode;
};

/**
 * Older pages prepend rows inside the block, where neither MessageScroller nor
 * browser scroll anchoring holds the reading position, so scroll by the growth.
 * A class so getSnapshotBeforeUpdate can measure right before the commit:
 * nested rows resize on their own state, so an earlier height can be stale.
 */
class WorkingBlockContent extends Component<WorkingBlockContentProps> {
	private content: HTMLDivElement | null = null;

	getSnapshotBeforeUpdate(previous: WorkingBlockContentProps): number | null {
		if (!didPrependIntoBlock(previous.memberIds, this.props.memberIds)) {
			return null;
		}

		return this.content?.offsetHeight ?? null;
	}

	componentDidUpdate(
		_previous: WorkingBlockContentProps,
		_state: unknown,
		heightBefore: number | null,
	) {
		const content = this.content;
		if (!content || heightBefore === null) {
			return;
		}

		const delta = content.offsetHeight - heightBefore;
		const viewport = getScrollParent(content);
		if (delta === 0 || !viewport) {
			return;
		}

		// When the same page also prepends rows above the block, MessageScroller
		// restores the block's own top edge from a MutationObserver callback,
		// which runs after this update and would cancel a synchronous adjustment.
		queueMicrotask(() => {
			viewport.scrollTop += delta;
		});
	}

	render() {
		return (
			<div
				ref={(content) => {
					this.content = content;
				}}
				className="mt-1.5 flex flex-col gap-2 border-0 border-l border-solid border-border ml-2 pl-4"
			>
				{this.props.children}
			</div>
		);
	}
}

type WorkingBlockDisclosureProps = {
	block: WorkingBlock;
	expanded: boolean;
	onExpandedChange: (expanded: boolean) => void;
	/** PROTOTYPE: previews a collapsed live block's newest steps. */
	peek?: Omit<WorkingBlockPeekProps, "onExpand">;
	children: React.ReactNode;
};

/** A block this young formed in front of the reader, so its peek grows in. */
const FRESH_BLOCK_MS = 5000;

const fold = { duration: 0.5, ease: [0.4, 0, 0.2, 1] } as const;

/**
 * Grows the peek in with the shared slide, and folds it away when the block
 * completes. The fold writes each frame's height itself and reconciles the
 * scroll position in the same frame: the scroller's resize pass waits for the
 * next frame, so a fold it corrects late lets the browser clamp an anchored
 * prompt for a frame.
 */
const PeekPresence: React.FC<{ children: React.ReactNode }> = ({
	children,
}) => {
	const [isPresent, safeToRemove] = usePresence();
	const foldRef = useRef<HTMLDivElement>(null);
	const syncLayout = useMessageScrollerLayoutSync();

	// The fold is an animation driven outside React.
	useEffect(() => {
		const element = foldRef.current;
		if (isPresent) {
			return;
		}
		if (!element) {
			safeToRemove();
			return;
		}
		const from = element.offsetHeight;
		const controls = animate(from, 0, {
			...fold,
			onUpdate: (height) => {
				element.style.height = `${height}px`;
				element.style.opacity = String(from > 0 ? height / from : 0);
				syncLayout();
			},
			onComplete: safeToRemove,
		});
		return () => controls.stop();
	}, [isPresent, safeToRemove, syncLayout]);

	return (
		<div ref={foldRef} className="overflow-hidden">
			<motion.div
				className="overflow-hidden"
				initial={{ height: 0, opacity: 0 }}
				animate={{ height: "auto", opacity: 1 }}
				transition={slide}
			>
				{children}
			</motion.div>
		</div>
	);
};

/**
 * Folds a block's step rows behind a summary row that reads like a tool row.
 */
export const WorkingBlockDisclosure: React.FC<WorkingBlockDisclosureProps> = ({
	block,
	expanded,
	onExpandedChange,
	peek,
	children,
}) => {
	// A page that loads mid-turn, or a reader collapsing the block, shows the
	// peek at once; only a block that just formed grows it in.
	const [peekGrowsIn] = useState(
		() =>
			block.startedAt === undefined ||
			Date.now() - block.startedAt < FRESH_BLOCK_MS,
	);
	const [hasExpanded, setHasExpanded] = useState(expanded);
	if (expanded && !hasExpanded) {
		setHasExpanded(true);
	}

	return (
		<ToolCall.Root
			status={block.isLive ? "running" : "completed"}
			hasContent
			expanded={expanded}
			onExpandedChange={onExpandedChange}
			revealEndOnExpand
		>
			<ToolCall.HeaderButton>
				<ToolCall.LeadingIcon>
					<ListTreeIcon className="size-4 shrink-0 stroke-[1.5] text-current" />
				</ToolCall.LeadingIcon>
				{block.isLive ? (
					<LiveLabel block={block} />
				) : (
					<ToolCall.Label>{getCompletedWorkingLabel(block)}</ToolCall.Label>
				)}
				<ToolCall.Chevron />
			</ToolCall.HeaderButton>
			{/* Unmounted while expanded, so the expanded rows replace the peek in
			    the same frame; it folds away only when the block completes. */}
			{peek && !expanded && (
				<AnimatePresence initial={peekGrowsIn && !hasExpanded}>
					{block.isLive && (
						<PeekPresence key="peek">
							<WorkingBlockPeek
								{...peek}
								onExpand={() => onExpandedChange(true)}
							/>
						</PeekPresence>
					)}
				</AnimatePresence>
			)}
			<ToolCall.Content>
				<WorkingBlockContent memberIds={block.memberIds}>
					{children}
				</WorkingBlockContent>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
