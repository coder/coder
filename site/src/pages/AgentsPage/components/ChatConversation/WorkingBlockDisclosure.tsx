import { ListChecksIcon, TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { useTime } from "#/hooks/useTime";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import {
	formatWorkingDuration,
	type WorkingBlock,
} from "./workingBlockGrouping";

const LiveLabel: FC<{ startedAt?: number; now?: number }> = ({
	startedAt,
	now,
}) => {
	// Only the live block subscribes to a clock; completed blocks render a
	// fixed label, so long transcripts never tick.
	const clock = useTime(() => Date.now(), { disabled: now !== undefined });
	if (startedAt === undefined) {
		return <ToolCall.Label>Working</ToolCall.Label>;
	}
	return (
		<ToolCall.Label>
			{`Working for ${formatWorkingDuration((now ?? clock) - startedAt)}`}
		</ToolCall.Label>
	);
};

const pluralize = (count: number, noun: string): string =>
	`${count} ${noun}${count === 1 ? "" : "s"}`;

/**
 * A partial block may be missing earlier rows that are not loaded yet, so its
 * duration and step count are lower bounds. Blocks without part timestamps
 * report steps only rather than a guessed duration.
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

type WorkingBlockDisclosureProps = {
	block: WorkingBlock;
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
	expanded,
	onExpandedChange,
	children,
	now,
}) => {
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
					<LiveLabel startedAt={block.startedAt} now={now} />
				) : (
					<ToolCall.Label>{getCompletedWorkingLabel(block)}</ToolCall.Label>
				)}
				{block.failedCount > 0 && (
					<span className="flex shrink-0 items-center gap-1 text-[13px] leading-6 text-content-destructive">
						<TriangleAlertIcon aria-hidden className="size-3.5 shrink-0" />
						{pluralize(block.failedCount, "failed step")}
					</span>
				)}
				<ToolCall.Chevron />
			</ToolCall.HeaderButton>
			<ToolCall.Content>
				<div className="mt-2 flex min-w-0 flex-col gap-2 border-0 border-l border-solid border-border-default pl-3">
					{children}
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};
