import { ListChecksIcon, ListTodoIcon, TriangleAlertIcon } from "lucide-react";
import {
	type FC,
	type ReactNode,
	type RefObject,
	useLayoutEffect,
	useRef,
} from "react";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useTime } from "#/hooks/useTime";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import { WorkingBlockContext } from "./workingBlockContext";
import {
	formatWorkingDuration,
	type WorkingBlock,
} from "./workingBlockGrouping";

const getLiveWorkingLabel = (block: WorkingBlock, now: number): string => {
	const activity = block.activity ? ` (${block.activity})` : "";
	if (block.startedAt === undefined) {
		return `Working${activity}`;
	}
	const elapsed = formatWorkingDuration(now - block.startedAt);
	return block.isPartial
		? `Working for at least ${elapsed}${activity}`
		: `Working for ${elapsed}${activity}`;
};

const LiveLabel: FC<{ block: WorkingBlock; now?: number }> = ({
	block,
	now,
}) => {
	// Only the live block subscribes to a clock; completed blocks render a
	// fixed label, so long transcripts never tick.
	const clock = useTime(() => Date.now(), { disabled: now !== undefined });
	return (
		<ToolCall.Label className="tabular-nums">
			{getLiveWorkingLabel(block, now ?? clock)}
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

/**
 * The message scroller re-pins to the end on every content resize while the
 * viewport is near the bottom, and only leaves that mode on wheel, touch, or
 * keyboard input. Without this, expanding a block near the bottom scrolls
 * the opened rows straight past. A synthetic wheel event is the only signal
 * the scroller accepts as user intent.
 */
const useHoldViewportOnToggle = (
	expanded: boolean,
	rootRef: RefObject<HTMLDivElement | null>,
) => {
	const previousRef = useRef(expanded);
	useLayoutEffect(() => {
		if (previousRef.current === expanded) {
			return;
		}
		previousRef.current = expanded;
		const root = rootRef.current;
		const viewport = root ? getScrollParent(root) : null;
		viewport?.dispatchEvent(
			new WheelEvent("wheel", { bubbles: true, deltaY: 0 }),
		);
	}, [expanded, rootRef]);
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
	const failedLabel = block.isPartial
		? `${pluralize(block.failedCount, "failed step")} or more`
		: pluralize(block.failedCount, "failed step");
	const rootRef = useRef<HTMLDivElement>(null);
	useHoldViewportOnToggle(expanded, rootRef);
	return (
		<ToolCall.Root
			ref={rootRef}
			status={block.isLive ? "running" : "completed"}
			expanded={expanded}
			onExpandedChange={onExpandedChange}
			data-testid="working-block"
		>
			<ToolCall.HeaderButton>
				<ToolCall.LeadingIcon>
					{block.isLive ? (
						<ListTodoIcon className="size-4 shrink-0 stroke-[1.5] text-current" />
					) : (
						<ListChecksIcon className="size-4 shrink-0 stroke-[1.5] text-current" />
					)}
				</ToolCall.LeadingIcon>
				{block.isLive ? (
					<LiveLabel block={block} now={now} />
				) : (
					<ToolCall.Label>{getCompletedWorkingLabel(block)}</ToolCall.Label>
				)}
				{block.failedCount > 0 && (
					<Tooltip>
						<TooltipTrigger asChild>
							<span
								role="img"
								aria-label={failedLabel}
								className="flex shrink-0 items-center gap-1 text-[13px] leading-6 text-content-destructive"
							>
								<TriangleAlertIcon aria-hidden className="size-3.5 shrink-0" />
								{block.failedCount}
							</span>
						</TooltipTrigger>
						<TooltipContent>{failedLabel}</TooltipContent>
					</Tooltip>
				)}
				<ToolCall.Chevron />
			</ToolCall.HeaderButton>
			<ToolCall.Content>
				<div
					ref={contentRef}
					className="ml-2 mt-2 flex min-w-0 flex-col gap-2 border-0 border-l border-solid border-border-default pl-4"
				>
					<WorkingBlockContext.Provider value={true}>
						{children}
					</WorkingBlockContext.Provider>
					{block.outcome && <div aria-hidden className="h-2" />}
				</div>
				{block.outcome && (
					<div
						data-testid="working-block-outcome"
						className="relative mb-2 flex h-6 items-center pl-[25px] text-[13px] leading-6 text-content-secondary"
					>
						<StatusIndicatorDot
							variant="inactive"
							size="sm"
							className="absolute left-[8.5px] -translate-x-1/2"
						/>
						{block.outcome === "stopped" ? "Stopped" : "Completed"}
					</div>
				)}
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
