import { ListChecksIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, type ReactNode, useLayoutEffect, useRef } from "react";
import { useTime } from "#/hooks/useTime";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import {
	didPrependIntoBlock,
	formatWorkingDuration,
	type WorkingBlock,
} from "./workingBlockGrouping";

const LiveLabel: FC<{ block: WorkingBlock }> = ({ block }) => {
	// Only the live block subscribes to a clock; completed blocks render a
	// fixed label, so long transcripts never tick.
	const now = useTime(() => Date.now());
	if (block.startedAt === undefined) {
		return <ToolCall.Label>Working</ToolCall.Label>;
	}
	const elapsed = formatWorkingDuration(now - block.startedAt);
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
 */
const useKeepReadingPositionAcrossPrepend = (memberIds: readonly number[]) => {
	const contentRef = useRef<HTMLDivElement>(null);
	const previousRef = useRef<{
		memberIds: readonly number[];
		height: number;
	}>(null);
	// Nested rows expand and collapse on their own state, which resizes the
	// content without rendering this component. Keep the cached height current
	// so the next prepend is measured against the size just before it.
	const observeContent = (content: HTMLDivElement | null) => {
		contentRef.current = content;
		if (!content) {
			return;
		}
		const observer = new ResizeObserver(() => {
			const previous = previousRef.current;
			if (previous) {
				previousRef.current = { ...previous, height: content.offsetHeight };
			}
		});
		observer.observe(content);
		return () => {
			observer.disconnect();
			contentRef.current = null;
		};
	};
	useLayoutEffect(() => {
		const content = contentRef.current;
		const previous = previousRef.current;
		previousRef.current = content
			? { memberIds, height: content.offsetHeight }
			: null;
		if (
			!content ||
			!previous ||
			!didPrependIntoBlock(previous.memberIds, memberIds)
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
		queueMicrotask(() => {
			viewport.scrollTop += delta;
		});
	});
	return observeContent;
};

type WorkingBlockDisclosureProps = {
	block: WorkingBlock;
	expanded: boolean;
	onExpandedChange: (expanded: boolean) => void;
	children: ReactNode;
};

/**
 * Folds a block's step rows behind a summary row that reads like a tool row.
 * Failed steps stay inside the block but are counted on the summary so a
 * failure is never hidden without a trace.
 */
export const WorkingBlockDisclosure: FC<WorkingBlockDisclosureProps> = ({
	block,
	expanded,
	onExpandedChange,
	children,
}) => {
	const contentRef = useKeepReadingPositionAcrossPrepend(block.memberIds);
	return (
		<ToolCall.Root
			status={block.isLive ? "running" : "completed"}
			expanded={expanded}
			onExpandedChange={onExpandedChange}
		>
			<ToolCall.HeaderButton>
				<ToolCall.LeadingIcon>
					<ListChecksIcon className="size-4 shrink-0 stroke-[1.5] text-current" />
				</ToolCall.LeadingIcon>
				{block.isLive ? (
					<LiveLabel block={block} />
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
					className="mt-1.5 flex flex-col gap-2 border-0 border-l border-solid border-border-default pl-3"
				>
					{children}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
