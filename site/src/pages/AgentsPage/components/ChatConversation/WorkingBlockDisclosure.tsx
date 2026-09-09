import { ListChecksIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, type ReactNode, useLayoutEffect, useRef } from "react";
import { useTime } from "#/hooks/useTime";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import {
	formatWorkingDuration,
	type WorkingBlock,
} from "./workingBlockGrouping";

const LiveLabel: FC<{ block: WorkingBlock; now?: number }> = ({
	block,
	now,
}) => {
	// Only the live block subscribes to a clock; completed blocks render a
	// fixed label, so long transcripts never tick.
	const clock = useTime(() => Date.now(), { disabled: now !== undefined });
	if (block.startedAt === undefined) {
		return <ToolCall.Label>Working</ToolCall.Label>;
	}
	const elapsed = formatWorkingDuration((now ?? clock) - block.startedAt);
	return (
		<ToolCall.Label>
			{block.isPartial
				? `Working for at least ${elapsed}`
				: `Working for ${elapsed}`}
		</ToolCall.Label>
	);
};

const pluralize = (count: number, noun: string): string =>
	`${count} ${noun}${count === 1 ? "" : "s"}`;

/**
 * A partial block may be missing earlier rows that are not loaded yet, so its
 * duration and step count are lower bounds (the live label does the same).
 * Blocks without part timestamps report steps only rather than a guessed
 * duration.
 */
const getCompletedWorkingLabel = (block: WorkingBlock): string => {
	const steps = pluralize(block.stepCount, "step");
	const stepsLabel = block.isPartial ? `${steps} or more` : steps;
	if (block.startedAt === undefined || block.endedAt === undefined) {
		return `Completed ${stepsLabel}`;
	}
	const duration = formatWorkingDuration(block.endedAt - block.startedAt);
	return block.isPartial
		? `Worked for at least ${duration} (${stepsLabel})`
		: `Worked for ${duration} (${steps})`;
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

/**
 * Older pages prepend rows inside an expanded partial block rather than as
 * new scroller items, so the scroller cannot hold the reading position and
 * browsers skip scroll anchoring at the top. Scroll by the growth instead.
 * Only a prepend qualifies: the previous first row must still be a member.
 * When the live row that opened a block is replaced by its persisted step,
 * the first key changes too, but that content changed in place.
 */
const useKeepReadingPositionAcrossPrepend = (rowKeys: readonly string[]) => {
	const firstRowKey = rowKeys[0];
	const contentRef = useRef<HTMLDivElement>(null);
	const previousRef = useRef<{ firstRowKey: string; height: number }>(null);
	useLayoutEffect(() => {
		const content = contentRef.current;
		const previous = previousRef.current;
		previousRef.current = content
			? { firstRowKey, height: content.offsetHeight }
			: null;
		if (
			!content ||
			!previous ||
			previous.firstRowKey === firstRowKey ||
			!rowKeys.includes(previous.firstRowKey)
		) {
			return;
		}
		const delta = content.offsetHeight - previous.height;
		const viewport = getScrollParent(content);
		if (delta === 0 || !viewport) {
			return;
		}
		// When the same page also prepends rows above the block, MessageScroller
		// restores the block's own top edge from a MutationObserver callback,
		// which runs after this effect and would cancel a synchronous adjustment.
		// The inner growth is applied after it, still before the next paint.
		queueMicrotask(() => {
			viewport.scrollTop += delta;
		});
	});
	return contentRef;
};

type WorkingBlockDisclosureProps = {
	block: WorkingBlock;
	/** Keys of the block's rows, oldest first; older pages join at the front. */
	rowKeys: readonly string[];
	expanded: boolean;
	onExpandedChange: (expanded: boolean) => void;
	children: ReactNode;
	/** Fixed clock for stories and tests. */
	now?: number;
};

/**
 * Folds a block's step rows behind a summary row that reads like a tool row.
 * Failed steps stay inside the block but are counted on the summary so a
 * failure is never hidden without a trace.
 */
export const WorkingBlockDisclosure: FC<WorkingBlockDisclosureProps> = ({
	block,
	rowKeys,
	expanded,
	onExpandedChange,
	children,
	now,
}) => {
	const contentRef = useKeepReadingPositionAcrossPrepend(rowKeys);
	return (
		<ToolCall.Root
			status={block.isLive ? "running" : "completed"}
			expanded={expanded}
			onExpandedChange={onExpandedChange}
			data-testid="working-block"
		>
			<ToolCall.HeaderButton>
				<ToolCall.LeadingIcon>
					<ListChecksIcon className="size-4 shrink-0 stroke-[1.5] text-current" />
				</ToolCall.LeadingIcon>
				{block.isLive ? (
					<LiveLabel block={block} now={now} />
				) : (
					<ToolCall.Label>{getCompletedWorkingLabel(block)}</ToolCall.Label>
				)}
				{block.failedCount > 0 && (
					<span className="flex shrink-0 items-center gap-1 text-[13px] leading-6 text-content-destructive">
						<TriangleAlertIcon aria-hidden className="size-3.5 shrink-0" />
						{block.isPartial
							? `${pluralize(block.failedCount, "failed step")} or more`
							: pluralize(block.failedCount, "failed step")}
					</span>
				)}
				<ToolCall.Chevron />
			</ToolCall.HeaderButton>
			<ToolCall.Content>
				<div
					ref={contentRef}
					className="mt-2 flex min-w-0 flex-col gap-2 border-0 border-l border-solid border-border-default pl-3"
				>
					{children}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
