import { InfoIcon, TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";

// Allocation sentinel for unlimited agent runtime hours
// (AgentRuntimeHoursUnlimitedAllocation in enterprise/coderd/license).
const unlimitedAllocation = -1;

const minutesPerHour = 60;

// Concurrent chat cap once the hard limit is reached. Mirrors
// defaultMaxConcurrentRootAgents in coderd/x/chatd; keep in sync.
const maxConcurrentChatsOverHardLimit = 5;

type CoderAgentsProductCardProps = {
	/**
	 * The license's agent_runtime_hours_allocation claim, in hours.
	 * Undefined or non-positive (except -1, unlimited) grants no hours.
	 */
	allocation?: number;
	/**
	 * Whole minutes used in the current usage period.
	 * Undefined when usage does not apply to this license.
	 */
	actualMinutes?: number;
	/** The license is a trial. */
	isTrial: boolean;
	/**
	 * Usage is at or above this license's advisory soft limit, but still
	 * within the purchased allocation.
	 */
	isSoftLimitReached: boolean;
	/** Usage is above this license's allocation. */
	isExceeded: boolean;
	/** Usage is at or above this license's hard limit. */
	isHardLimitExceeded: boolean;
};

const MetricLabel: FC<{ label: string; tooltip: string }> = ({
	label,
	tooltip,
}) => (
	<div className="flex items-center gap-1 font-medium text-content-secondary">
		<span>{label}</span>
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={`${label} information`}
					className="m-0 inline-flex appearance-none border-0 bg-transparent p-0 text-content-secondary"
				>
					<InfoIcon className="size-3" />
				</button>
			</TooltipTrigger>
			<TooltipContent side="top" className="max-w-xs">
				{tooltip}
			</TooltipContent>
		</Tooltip>
	</div>
);

const CardContainer: FC<{
	title: string;
	className?: string;
	headerEnd?: ReactNode;
	children: ReactNode;
}> = ({ title, className, headerEnd, children }) => (
	<div className={cn("rounded-sm border px-6 py-4", className)}>
		<div className="flex items-center justify-between gap-3">
			<div className="text-sm font-medium text-content-primary">{title}</div>
			{headerEnd}
		</div>
		{children}
	</div>
);

// TODO: placeholder tooltip copy pending product review.
const totalAgentMinutesTooltip =
	"Total agent runtime minutes used out of the minutes included in this license.";
const concurrentChatsTooltip =
	"Number of agents that can run at the same time.";
const concurrentChatsHardLimitTooltip = `${concurrentChatsTooltip} You've reached your limit: concurrent chats are now capped at ${maxConcurrentChatsOverHardLimit} (down from unlimited).`;

const formatMinutes = (minutes: number) => minutes.toLocaleString("en-US");

const MinutesUsedMetric: FC<{ actualMinutes?: number }> = ({
	actualMinutes,
}) => (
	<div>
		<div className="flex items-center gap-1 font-medium text-content-secondary">
			<span>Agent minutes used</span>
		</div>
		<div className="mt-0.5 text-sm font-medium text-content-primary">
			{actualMinutes === undefined ? "\u2014" : formatMinutes(actualMinutes)}
		</div>
	</div>
);

export const CoderAgentsProductCard: FC<CoderAgentsProductCardProps> = ({
	allocation,
	actualMinutes,
	isTrial,
	isSoftLimitReached,
	isExceeded,
	isHardLimitExceeded,
}) => {
	const isUnlimited = allocation === unlimitedAllocation;
	const grantsAgentHours =
		allocation !== undefined && (allocation > 0 || isUnlimited);

	if (!grantsAgentHours) {
		return (
			<CardContainer
				title={isTrial ? "Coder Agents Trial" : "Coder Agents"}
				className="border-dashed border-border"
			>
				<div className="mt-3 flex flex-wrap gap-x-12 gap-y-3 text-xs">
					<MinutesUsedMetric actualMinutes={actualMinutes} />
					<div>
						<MetricLabel
							label={isTrial ? "Concurrent agents" : "Max concurrent agents"}
							tooltip={concurrentChatsTooltip}
						/>
						<div className="mt-0.5 text-sm font-medium text-content-primary">
							{isTrial ? "Unlimited" : maxConcurrentChatsOverHardLimit}
						</div>
					</div>
				</div>
				{isTrial ? (
					<Button asChild className="mt-4 w-full">
						<a href="mailto:sales@coder.com">Upgrade</a>
					</Button>
				) : (
					<div className="mt-4 flex items-center gap-3 text-sm">
						<Link asChild showExternalIcon={false} size="lg">
							<RouterLink to="/deployment/premium">
								Try unlimited for 30 days
							</RouterLink>
						</Link>
						<div aria-hidden="true" className="h-4 w-px shrink-0 bg-border" />
						<Link href={docs("/ai-coder/agents/licensing-usage")} size="lg">
							View docs
						</Link>
					</div>
				)}
			</CardContainer>
		);
	}

	const isOverage = isExceeded || isHardLimitExceeded;
	const actualLabel =
		actualMinutes === undefined ? "\u2014" : formatMinutes(actualMinutes);
	const minutesValueClassName = isOverage
		? "text-content-destructive"
		: isSoftLimitReached
			? "text-border-warning"
			: undefined;

	return (
		<CardContainer
			title="Coder Agents"
			className={cn(
				"border-solid",
				isOverage
					? "border-border-destructive"
					: isSoftLimitReached
						? "border-border-warning"
						: "border-border",
			)}
			headerEnd={
				isHardLimitExceeded ? (
					<Badge variant="destructive" size="sm" role="status">
						<TriangleAlertIcon />
						Limit reached
					</Badge>
				) : isSoftLimitReached && !isOverage ? (
					// The soft limit is otherwise only conveyed by the warning
					// colors, so announce it for assistive technology too.
					<span role="status" className="sr-only">
						Approaching minutes limit
					</span>
				) : undefined
			}
		>
			<div className="mt-3 flex flex-wrap gap-x-12 gap-y-3 text-xs">
				<div>
					<MetricLabel
						label="Total Agent minutes"
						tooltip={totalAgentMinutesTooltip}
					/>
					<div className="mt-0.5 text-sm font-medium text-content-primary">
						{isUnlimited ? (
							"Unlimited"
						) : (
							<>
								<span className={minutesValueClassName}>{actualLabel}</span> /{" "}
								{formatMinutes(allocation * minutesPerHour)}
							</>
						)}
					</div>
				</div>
				<div>
					<MetricLabel
						label="Concurrent agents"
						tooltip={
							isHardLimitExceeded
								? concurrentChatsHardLimitTooltip
								: concurrentChatsTooltip
						}
					/>
					<div
						className={cn(
							"mt-0.5 text-sm font-medium",
							isHardLimitExceeded
								? "text-content-destructive"
								: "text-content-primary",
						)}
					>
						{isHardLimitExceeded
							? maxConcurrentChatsOverHardLimit
							: "Unlimited"}
					</div>
				</div>
			</div>
			<div className="mt-4 text-sm">
				<Link href={docs("/ai-coder/agents/licensing-usage")} size="lg">
					View docs
				</Link>
			</div>
		</CardContainer>
	);
};
