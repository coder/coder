import { ListChecksIcon, TriangleAlertIcon } from "lucide-react";
import { Component, type FC, type ReactNode } from "react";
import { useTime } from "#/hooks/useTime";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import type { WorkingBlock } from "./workingBlockGrouping";

export const formatWorkingDuration = (milliseconds: number): string => {
	const totalSeconds = Math.max(0, Math.floor(milliseconds / 1000));
	const hours = Math.floor(totalSeconds / 3600);
	const minutes = Math.floor((totalSeconds % 3600) / 60);
	const seconds = totalSeconds % 60;
	if (hours > 0) {
		return `${hours}h ${minutes}m`;
	}
	if (minutes > 0) {
		return `${minutes}m ${seconds}s`;
	}
	return `${seconds}s`;
};

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

type LiveLabelProps = { block: WorkingBlock };

const LiveLabel: FC<LiveLabelProps> = ({ block }) => {
	// Only the live block subscribes to a clock; completed blocks render a
	// fixed label, so long transcripts never tick.
	const now = useTime(() => Date.now());
	if (block.startedAt === undefined) {
		return <ToolCall.Label>Working</ToolCall.Label>;
	}
	const elapsed = formatWorkingDuration(now - block.startedAt);
	return (
		<ToolCall.Label>{`Working for ${atLeast(block)}${elapsed}`}</ToolCall.Label>
	);
};

const getCompletedWorkingLabel = (block: WorkingBlock): string => {
	const steps = countLabel(block, block.stepCount, "step");
	if (block.startedAt === undefined || block.endedAt === undefined) {
		return `Completed ${steps}`;
	}
	const duration = formatWorkingDuration(block.endedAt - block.startedAt);
	return `Worked for ${atLeast(block)}${duration} (${steps})`;
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
	children: ReactNode;
};

/**
 * Older pages prepend rows inside an expanded partial block rather than as
 * new scroller items, so the scroller cannot hold the reading position and
 * browsers skip scroll anchoring at the top. Scroll by the growth instead.
 *
 * A class component because getSnapshotBeforeUpdate is the only React API
 * that measures the DOM right before a commit; nested rows resize on their
 * own state, so a height cached any earlier can be stale.
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
				className="mt-1.5 flex flex-col gap-2 border-0 border-l border-solid border-border-default pl-3"
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
						{/* Separates the badge from the label in the button's accessible name. */}
						<span className="sr-only">, </span>
						<TriangleAlertIcon aria-hidden className="size-3.5 shrink-0" />
						{countLabel(block, block.failedCount, "failed step")}
					</span>
				)}
				<ToolCall.Chevron />
			</ToolCall.HeaderButton>
			<ToolCall.Content>
				<WorkingBlockContent memberIds={block.memberIds}>
					{children}
				</WorkingBlockContent>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
