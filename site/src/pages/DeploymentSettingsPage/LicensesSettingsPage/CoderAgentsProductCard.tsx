import { TriangleAlertIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { cn } from "#/utils/cn";
import { ProductCardMetricLabel } from "./ProductCardMetricLabel";

// Allocation sentinel for unlimited agent runtime hours
// (AgentRuntimeHoursUnlimitedAllocation in enterprise/coderd/license).
const unlimitedAllocation = -1;

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
	 * Hours used in the current usage period, floored to tenths.
	 * Undefined when usage does not apply to this license.
	 */
	actual?: number;
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

const CardContainer: FC<{
	className?: string;
	headerEnd?: ReactNode;
	children: ReactNode;
}> = ({ className, headerEnd, children }) => (
	<div
		className={cn(
			"min-w-[320px] flex-1 rounded-sm border px-6 py-4",
			className,
		)}
	>
		<div className="flex items-center justify-between gap-3">
			<div className="text-sm font-medium text-content-primary">
				Coder Agents
			</div>
			{headerEnd}
		</div>
		{children}
	</div>
);

// TODO: placeholder tooltip copy pending product review.
const totalAgentHoursTooltip =
	"Total agent runtime hours used out of the hours included in this license.";
const concurrentAgentsTooltip =
	"Number of Coder Agents that can run at the same time.";
const concurrentAgentsHardLimitTooltip = `${concurrentAgentsTooltip} You've reached your limit: concurrent agents are now capped at ${maxConcurrentChatsOverHardLimit} (down from unlimited).`;

// The value is already floored to tenths, so no rounding happens here.
const formatHoursUsed = (hours: number) =>
	hours.toLocaleString("en-US", {
		minimumFractionDigits: 1,
		maximumFractionDigits: 1,
	});

export const CoderAgentsProductCard: FC<CoderAgentsProductCardProps> = ({
	allocation,
	actual,
	isSoftLimitReached,
	isExceeded,
	isHardLimitExceeded,
}) => {
	const isUnlimited = allocation === unlimitedAllocation;
	const grantsAgentHours =
		allocation !== undefined && (allocation > 0 || isUnlimited);

	if (!grantsAgentHours) {
		return (
			<CardContainer className="border-dashed border-highlight-purple">
				<div className="mt-3 flex flex-wrap gap-x-12 gap-y-3 text-xs">
					<div>
						<ProductCardMetricLabel
							label="Max concurrent agents"
							tooltip={concurrentAgentsTooltip}
						/>
						<div className="mt-0.5 text-sm font-medium text-content-primary">
							{maxConcurrentChatsOverHardLimit}
						</div>
					</div>
					{actual !== undefined && (
						<div>
							<div className="flex items-center gap-1 font-medium text-content-secondary">
								<span>Agent hours used</span>
							</div>
							<div className="mt-0.5 text-sm font-medium text-content-primary">
								{formatHoursUsed(actual)}
							</div>
						</div>
					)}
				</div>
				<Button asChild className="mt-4 w-full">
					<a href="mailto:sales@coder.com">Upgrade</a>
				</Button>
			</CardContainer>
		);
	}

	const isOverage = isExceeded || isHardLimitExceeded;
	const actualLabel = actual === undefined ? "\u2014" : formatHoursUsed(actual);
	const hoursValueClassName = isOverage
		? "text-content-destructive"
		: isSoftLimitReached
			? "text-border-warning"
			: undefined;

	return (
		<CardContainer
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
						Approaching hours limit
					</span>
				) : undefined
			}
		>
			<div className="mt-3 flex flex-wrap gap-x-12 gap-y-3 text-xs">
				<div>
					<ProductCardMetricLabel
						label="Total Agent hours"
						tooltip={totalAgentHoursTooltip}
					/>
					<div className="mt-0.5 text-sm font-medium text-content-primary">
						{isUnlimited ? (
							"Unlimited"
						) : (
							<>
								<span className={hoursValueClassName}>{actualLabel}</span> /{" "}
								{allocation.toLocaleString("en-US")}
							</>
						)}
					</div>
				</div>
				<div>
					<ProductCardMetricLabel
						label="Concurrent agents"
						tooltip={
							isHardLimitExceeded
								? concurrentAgentsHardLimitTooltip
								: concurrentAgentsTooltip
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
			<div className="mt-4 flex items-center gap-2 text-sm">
				<Link asChild size="lg" showExternalIcon={false}>
					<RouterLink to="/deployment/groups">Manage usage</RouterLink>
				</Link>
				<span className="text-content-secondary" aria-hidden>
					|
				</span>
				<Link asChild size="lg" showExternalIcon={false}>
					<RouterLink to="/ai/settings/coder-agents">Agent settings</RouterLink>
				</Link>
			</div>
		</CardContainer>
	);
};
